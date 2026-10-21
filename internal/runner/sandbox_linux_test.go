package runner

import (
	"bytes"
	"context"
	"fmt"
	"math/rand/v2"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"golang.org/x/sys/unix"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

// withCode returns the 26-logfmt stub with extra imports (aliased, so they
// cannot clash with the stub's) and extra declarations.
func withCode(t *testing.T, imports []string, decls string) map[string]string {
	t.Helper()
	src := stubFiles(t, "26-logfmt")["logfmt.go"]
	block := "import (\n\t" + strings.Join(imports, "\n\t") + "\n)\n"
	out := strings.Replace(src, "package logfmt\n", "package logfmt\n\n"+block, 1)
	if out == src {
		t.Fatal("the 26-logfmt stub has no package clause line; adjust the test")
	}
	return map[string]string{"logfmt.go": out + "\n" + decls}
}

// probeResults collects the "PROBE name: result" lines of a run's output.
func probeResults(events []runnerapi.Event) map[string]string {
	out := make(map[string]string)
	for _, ev := range events {
		for line := range strings.Lines(ev.Output) {
			rest, ok := strings.CutPrefix(strings.TrimSpace(line), "PROBE ")
			if !ok {
				continue
			}
			if name, result, ok := strings.Cut(rest, ": "); ok {
				out[name] = result
			}
		}
	}
	return out
}

// listenOutsideEphemeral listens on a loopback port below the ephemeral
// range: a fixed port, like the runner's own API in production.
func listenOutsideEphemeral(t *testing.T) net.Listener {
	t.Helper()
	lo, _ := ephemeralPorts()
	for range 100 {
		port := 20000 + rand.IntN(max(lo-20000, 1))
		ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:"+strconv.Itoa(port))
		if err == nil {
			t.Cleanup(func() { _ = ln.Close() })
			return ln
		}
	}
	t.Skip("no free port below the ephemeral range")
	return nil
}

// TestSandboxConfinesTheRun runs learner code that tries what an attacker
// with code execution in a run would try first.
func TestSandboxConfinesTheRun(t *testing.T) {
	slow(t)
	t.Parallel()
	abi := landlockABI()
	if abi == 0 {
		t.Skip("Landlock is not available on this kernel")
	}
	env := newTestEnv(t)

	// Another run's workspace, as a concurrent run would have it.
	other := filepath.Join(env.workDir, workspacePrefix+"other", "mod")
	if err := os.MkdirAll(other, 0o700); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(other, "secret.go")
	if err := os.WriteFile(secret, []byte("package other // another learner's code\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(env.workDir, workspacePrefix+"other")) })
	fixed := listenOutsideEphemeral(t)
	go func() {
		for {
			c, err := fixed.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	runner := os.Getpid() // the test process serves the runner API here

	files := withCode(t, []string{`pfmt "fmt"`, `pnet "net"`, `pos "os"`, `psyscall "syscall"`, `ptime "time"`},
		fmt.Sprintf(`
func report(name string, err error) {
	if err != nil {
		pfmt.Printf("PROBE %%s: denied: %%v\n", name, err)
		return
	}
	pfmt.Printf("PROBE %%s: allowed\n", name)
}

func init() {
	_, err := pos.ReadFile(%q)
	report("other-workspace", err)
	_, err = pos.ReadFile("/proc/%d/environ")
	report("runner-environ", err)
	f, err := pos.Open("/proc/%d/mem")
	if err == nil {
		_ = f.Close()
	}
	report("runner-memory", err)
	report("signal-runner", psyscall.Kill(%d, 0))
	c, err := pnet.DialTimeout("tcp", %q, 5*ptime.Second)
	if err == nil {
		_ = c.Close()
	}
	report("connect-fixed-port", err)
	ln, err := pnet.Listen("tcp", "127.0.0.1:0")
	if err == nil {
		c, err = pnet.Dial("tcp", ln.Addr().String())
		if err == nil {
			_ = c.Close()
		}
		_ = ln.Close()
	}
	report("loopback-ephemeral-port", err)
	_, err = pos.ReadFile("logfmt_test.go")
	report("own-workspace", err)
	err = pos.WriteFile(pos.TempDir()+"/scratch", []byte("x"), 0o600)
	report("own-tmp", err)
}
`, secret, runner, runner, runner, fixed.Addr().String()))

	events := env.runTests(t, runnerapi.TestRequest{Exercise: "26-logfmt", Files: files, Run: "^TestParse$"})
	got := probeResults(events)
	want := map[string]string{
		"other-workspace":         "denied",
		"runner-memory":           "denied",
		"loopback-ephemeral-port": "allowed",
		"own-workspace":           "allowed",
		"own-tmp":                 "allowed",
	}
	// The kernel lets CAP_PERFMON and CAP_SYS_ADMIN read any process's
	// environment before it asks Landlock; the runner has neither in
	// production, but a test run as root does.
	if !hasCapability(capPerfmon) && !hasCapability(capSysAdmin) {
		want["runner-environ"] = "denied"
	}
	if abi >= 4 {
		want["connect-fixed-port"] = "denied"
	}
	if abi >= 6 {
		want["signal-runner"] = "denied"
	}
	for name, w := range want {
		if g := got[name]; !strings.HasPrefix(g, w) {
			t.Errorf("probe %s: %q, want %s", name, g, w)
		}
	}
	if t.Failed() {
		t.Logf("all probes: %v", got)
	}
}

// TestStrayProcessesAreKilled runs learner code that starts processes
// meant to outlive the run: one in a new session, one in a new process
// group of the run's session. Neither the group kill nor the timeout
// reaches them; the sweep must.
func TestStrayProcessesAreKilled(t *testing.T) {
	slow(t)
	// Not parallel: any server's sweep kills any stray, and the parallel
	// tests check that their own runs leave none.
	env := newTestEnv(t)
	markers := []string{
		fmt.Sprintf("%d.%06d", 3000+rand.IntN(900), rand.IntN(1e6)),
		fmt.Sprintf("%d.%06d", 4000+rand.IntN(900), rand.IntN(1e6)),
	}
	files := withCode(t, []string{`pexec "os/exec"`, `psyscall "syscall"`}, fmt.Sprintf(`
func init() {
	session := pexec.Command("/bin/sleep", %q)
	session.SysProcAttr = &psyscall.SysProcAttr{Setsid: true}
	_ = session.Start()
	group := pexec.Command("/bin/sleep", %q)
	group.SysProcAttr = &psyscall.SysProcAttr{Setpgid: true}
	_ = group.Start()
}
`, markers[0], markers[1]))

	env.runTests(t, runnerapi.TestRequest{Exercise: "26-logfmt", Files: files, Run: "^TestParse$"})
	if !waitFor(10*time.Second, func() bool { return len(sleepers(markers)) == 0 }) {
		t.Errorf("processes outlived the run: %v", sleepers(markers))
	}
	assertWorkDirEmpty(t, env.workDir)
	if n := counterValue(t, env.srv.metrics.strays); n < 2 {
		t.Errorf("runner_stray_processes_killed_total = %v, want at least 2", n)
	}
}

func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatal(err)
	}
	return m.GetCounter().GetValue()
}

// sleepers returns the live sleep processes with 1 of the markers as
// their argument.
func sleepers(markers []string) []string {
	var found []string
	for _, p := range listProcs() {
		if p.state == 'Z' {
			continue
		}
		cmdline, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(p.pid), "cmdline"))
		if err != nil {
			continue
		}
		args := strings.Split(strings.TrimRight(string(cmdline), "\x00"), "\x00")
		for _, m := range markers {
			if len(args) == 2 && args[1] == m {
				found = append(found, strconv.Itoa(p.pid)+" "+strings.Join(args, " "))
			}
		}
	}
	return found
}

func TestSweepKillsAProcessInANewSession(t *testing.T) {
	if err := enableSubreaper(); err != nil {
		t.Fatal(err)
	}
	marker := fmt.Sprintf("%d.%06d", 5000+rand.IntN(900), rand.IntN(1e6))
	cmd := exec.CommandContext(t.Context(), "/bin/sleep", marker)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Skipf("start sleep: %v", err)
	}
	// The sweep reaps it; Wait would race with that.
	defer func() { _ = cmd.Process.Release() }()

	if n := procs.sweep(); n < 1 {
		t.Errorf("sweep killed %d processes, want at least 1", n)
	}
	if !waitFor(5*time.Second, func() bool {
		procs.sweep() // reaps the zombie
		_, err := os.Stat(filepath.Join("/proc", strconv.Itoa(cmd.Process.Pid)))
		return err != nil
	}) {
		t.Error("the process in a new session survived the sweep or was not reaped")
	}
}

func TestSweepSparesTheRunnersGroup(t *testing.T) {
	// A process in the runner's own process group was started by the
	// program around the runner, never by a run.
	cmd := exec.CommandContext(t.Context(), "/bin/sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skipf("start sleep: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	procs.sweep()
	if st, err := readProcStat(cmd.Process.Pid); err != nil || st.state == 'Z' || st.state == 'T' {
		t.Errorf("sweep touched a process of the runner's group: %+v, %v", st, err)
	}
}

func TestChildLimits(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var out bytes.Buffer
	// The pause lets the runner set the limits; then cat, a child of the
	// shell like learner code is a child of go, reports what it inherited.
	code, err := run(ctx, command{
		path: "/bin/sh", args: []string{"-c", "sleep 0.3; cat /proc/self/oom_score_adj /proc/self/limits"},
		dir: t.TempDir(), env: []string{"PATH=/usr/bin:/bin"}, stdout: &out, stderr: &out,
	})
	if err != nil || code != 0 {
		t.Fatalf("run: exit %d, %v: %s", code, err, out.String())
	}
	lines := strings.Split(out.String(), "\n")
	if lines[0] != childOOMScoreAdj {
		t.Errorf("oom_score_adj = %q, want %s", lines[0], childOOMScoreAdj)
	}
	limits := map[string][]string{}
	for _, l := range lines[1:] {
		for _, name := range []string{"Max core file size", "Max file size"} {
			if rest, ok := strings.CutPrefix(l, name); ok {
				limits[name] = strings.Fields(rest)
			}
		}
	}
	size := strconv.Itoa(childMaxFileSize)
	for name, want := range map[string][]string{
		"Max core file size": {"0", "0"},
		"Max file size":      {size, size},
	} {
		if got := limits[name]; len(got) < 2 || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("%s = %v, want soft and hard %v", name, got, want)
		}
	}
}

func TestParsePortRange(t *testing.T) {
	tests := []struct {
		in     string
		lo, hi int
	}{
		{"32768\t60999\n", 32768, 60999},
		{"1024 65535", 1024, 65535},
		{"", defaultEphemeralLow, defaultEphemeralHigh},
		{"60999 32768", defaultEphemeralLow, defaultEphemeralHigh},
		{"0 100", defaultEphemeralLow, defaultEphemeralHigh},
		{"1 70000", defaultEphemeralLow, defaultEphemeralHigh},
		{"a b", defaultEphemeralLow, defaultEphemeralHigh},
		{"1 2 3", defaultEphemeralLow, defaultEphemeralHigh},
	}
	for _, tt := range tests {
		if lo, hi := parsePortRange(tt.in); lo != tt.lo || hi != tt.hi {
			t.Errorf("parsePortRange(%q) = %d, %d, want %d, %d", tt.in, lo, hi, tt.lo, tt.hi)
		}
	}
}

func TestParseProcStat(t *testing.T) {
	// Fields after the name: state ppid pgrp session tty tpgid flags
	// minflt cminflt majflt cmajflt utime stime cutime cstime priority nice
	// threads itrealvalue starttime ...
	tail := " 0 0 4194304 100 0 0 0 1 2 0 0 20 0 1 0 987654 1000 200"
	tests := []struct {
		name    string
		line    string
		want    procStat
		wantErr bool
	}{
		{name: "plain", line: "42 (sleep) S 7 42 42" + tail,
			want: procStat{pid: 42, ppid: 7, pgrp: 42, session: 42, state: 'S', start: 987654}},
		{name: "name with spaces and parentheses", line: "42 (a) b (c) Z 1 2 3" + tail,
			want: procStat{pid: 42, ppid: 1, pgrp: 2, session: 3, state: 'Z', start: 987654}},
		{name: "no parenthesis", line: "42 sleep S 7 42 42" + tail, wantErr: true},
		{name: "short", line: "42 (sleep) S 7 42", wantErr: true},
		{name: "bad ppid", line: "42 (sleep) S x 42 42" + tail, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseProcStat(42, tt.line)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNewSandboxGrantsTheToolchainAndCaches(t *testing.T) {
	tc, err := toolchain()
	if err != nil {
		t.Fatal(err)
	}
	cacheDir := t.TempDir()
	sb, err := newSandbox(Options{
		Go: tc.goPath, GoCache: cacheDir, Path: "/usr/bin:relative/bin:" + filepath.Dir(tc.goPath),
		Staticcheck: "/usr/bin/true", StaticcheckCache: filepath.Join(cacheDir, "sc"),
	})
	if err != nil {
		t.Fatal(err)
	}
	goroot := filepath.Dir(binDir(tc.goPath))
	for _, want := range []string{"/usr", "/usr/bin", goroot, filepath.Dir(tc.goPath)} {
		if !slices.Contains(sb.exec, want) {
			t.Errorf("exec %v lacks %s", sb.exec, want)
		}
	}
	if slices.Contains(sb.exec, "relative/bin") {
		t.Errorf("exec %v holds a relative PATH entry", sb.exec)
	}
	if !slices.Contains(sb.write, cacheDir) || !slices.Contains(sb.write, filepath.Join(cacheDir, "sc")) {
		t.Errorf("write %v lacks the caches", sb.write)
	}
	// The rules hold absolute paths.
	course, err := filepath.Abs(courseDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, list := range [][]string{sb.exec, sb.read, sb.dev} {
		if slices.Contains(list, course) {
			t.Errorf("the course checkout (with solutions/ in development) is reachable: %v", list)
		}
	}
}

// Capability numbers from linux/capability.h.
const (
	capSysAdmin = 21
	capPerfmon  = 38
)

// hasCapability reports whether the test process has capability c in its
// effective set.
func hasCapability(c uint) bool {
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return false
	}
	for line := range strings.Lines(string(status)) {
		if rest, ok := strings.CutPrefix(line, "CapEff:"); ok {
			v, err := strconv.ParseUint(strings.TrimSpace(rest), 16, 64)
			return err == nil && v&(1<<c) != 0
		}
	}
	return false
}

// mainThreadProbe is what TestMain saw when it ran runOffMainThread on the
// process's main thread, which a test function cannot get.
var mainThreadProbe struct {
	onMain     bool // TestMain started on the main thread, so the rest means something
	fnThread   int
	err        error
	noNewPrivs string // of the main thread afterwards
}

func probeMainThread() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if unix.Gettid() != unix.Getpid() {
		return
	}
	mainThreadProbe.onMain = true
	_, mainThreadProbe.err = runOffMainThread(func() error {
		mainThreadProbe.fnThread = unix.Gettid()
		return unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0)
	})
	mainThreadProbe.noNewPrivs = threadNoNewPrivs(unix.Getpid())
}

// threadNoNewPrivs returns the NoNewPrivs flag of 1 thread of the test
// process: "0" or "1".
func threadNoNewPrivs(tid int) string {
	status, err := os.ReadFile(filepath.Join("/proc/self/task", strconv.Itoa(tid), "status"))
	if err != nil {
		return err.Error()
	}
	for line := range strings.Lines(string(status)) {
		if rest, ok := strings.CutPrefix(line, "NoNewPrivs:"); ok {
			return strings.TrimSpace(rest)
		}
	}
	return "missing"
}

func TestSandboxNeverRestrictsTheMainThread(t *testing.T) {
	// The kernel checks signals and ptrace against the main thread, so a
	// restricted main thread would let 1 run reach the runner.
	pid := unix.Getpid()
	for range 50 {
		if err := onSpareThread(func() error {
			return unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0)
		}); err != nil {
			t.Fatal(err)
		}
	}
	if got := threadNoNewPrivs(pid); got != "0" {
		t.Errorf("main thread NoNewPrivs = %s after restricting spare threads, want 0", got)
	}

	p := mainThreadProbe
	if !p.onMain {
		t.Skip("TestMain did not start on the main thread")
	}
	if p.err != nil {
		t.Fatalf("runOffMainThread on the main thread: %v", p.err)
	}
	if p.fnThread == pid || p.fnThread == 0 {
		t.Errorf("fn ran on thread %d; the main thread is %d", p.fnThread, pid)
	}
	if p.noNewPrivs != "0" {
		t.Errorf("main thread NoNewPrivs = %s after runOffMainThread, want 0", p.noNewPrivs)
	}
}

// assertNoStrays checks that the server counted no stray processes: the
// processes of a run that was killed died with its process group.
func assertNoStrays(t *testing.T, env *testEnv) {
	t.Helper()
	if n := counterValue(t, env.srv.metrics.strays); n != 0 {
		t.Errorf("runner_stray_processes_killed_total = %v after a run that left nothing, want 0", n)
	}
}
