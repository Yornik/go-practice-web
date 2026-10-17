package runner

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// Limits every child gets. Learner code runs as the runner's user and
// inherits them from the go command.
const (
	// childOOMScoreAdj makes a run's processes the OOM killer's first
	// choice, so that a test that eats memory dies instead of the runner.
	childOOMScoreAdj = "1000"
	// childMaxFileSize bounds 1 file. The largest file a run writes is a
	// test binary built with -race, a few tens of MB.
	childMaxFileSize = 256 << 20
)

// enableSubreaper makes the runner a child subreaper: a process whose
// parent died becomes the runner's child instead of PID 1's. A process that
// left its run's session (setsid) is then still the runner's to find, kill
// and reap.
var enableSubreaper = sync.OnceValue(func() error {
	return unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0)
})

// tracker knows the sessions of the commands that run, so that a sweep can
// tell a stray process from a working one. It is package state because the
// kernel state it mirrors is per process: the subreaper flag and the
// process's children.
type tracker struct {
	mu       sync.Mutex
	sessions map[int]bool // session IDs of running commands: their leader's PID
}

var procs = &tracker{sessions: map[int]bool{}}

// start runs startFn, which must start cmd in a new session, and tracks
// the session. Holding the lock keeps a sweep from seeing the new process
// before it is tracked.
func (t *tracker) start(cmd *exec.Cmd, startFn func() error) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := startFn(); err != nil {
		return err
	}
	pid := cmd.Process.Pid
	t.sessions[pid] = true
	// The go command runs for a while before it starts learner code, which
	// inherits these, so setting them after the start leaves no gap that
	// matters. They are best effort: the run is still bounded by its
	// timeout.
	_ = unix.Prlimit(pid, unix.RLIMIT_CORE, &unix.Rlimit{}, nil)
	_ = unix.Prlimit(pid, unix.RLIMIT_FSIZE, &unix.Rlimit{Cur: childMaxFileSize, Max: childMaxFileSize}, nil)
	_ = os.WriteFile(filepath.Join("/proc", strconv.Itoa(pid), "oom_score_adj"), []byte(childOOMScoreAdj), 0)
	return nil
}

// finished forgets the session of a command whose leader was reaped and
// whose process group was killed, and sweeps the processes it left. It
// returns the number of processes killed.
func (t *tracker) finished(pid int) int {
	t.mu.Lock()
	delete(t.sessions, pid)
	t.mu.Unlock()
	// A killed test binary built with -race can take a moment to unmap its
	// memory. Waiting for the group to die first keeps the count to the
	// processes that escaped it.
	waitGroupDead(pid, groupDeathWait)
	return t.sweep()
}

// groupDeathWait bounds the wait for a killed process group to die. Only a
// process stuck in the kernel (uninterruptible sleep) takes longer.
const groupDeathWait = 2 * time.Second

// waitGroupDead waits until no process of the group pgid runs any more,
// or until max passed.
func waitGroupDead(pgid int, maxWait time.Duration) {
	deadline := time.Now().Add(maxWait)
	for {
		alive := false
		for _, p := range listProcs() {
			if p.pgrp == pgid && p.state != 'Z' && p.state != 'X' {
				alive = true
				break
			}
		}
		if !alive || time.Now().After(deadline) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// maxStopPasses bounds the passes that stop stray processes before they
// are killed. Each pass stops what the last one found; a process can only
// fork while it runs, so a fork bomb runs out of running members.
const maxStopPasses = 64

// sweep kills the runner's descendants that belong to no running command:
// processes that left their run's session with setsid or that outlived it
// in another process group. They are stopped first and killed after, so
// that a process that forks cannot outrun the sweep. Zombies among the
// runner's own children are reaped. It returns the number of processes
// killed.
//
// The runner's own process group is left alone: the runner starts every
// command in a new session, which its processes cannot leave for the
// runner's group, so anything there was started by the program around the
// runner (a test, for example).
func (t *tracker) sweep() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	self, ownGroup := os.Getpid(), unix.Getpgrp()
	for range maxStopPasses {
		running := false
		for _, p := range t.strays(self, ownGroup) {
			if !p.stopped() {
				running = true
				p.signal(self, unix.SIGSTOP)
			}
		}
		if !running {
			break
		}
	}
	killed := 0
	for _, p := range t.strays(self, ownGroup) {
		if p.state != 'Z' {
			p.signal(self, unix.SIGKILL)
			killed++
		}
	}
	for _, p := range t.strays(self, ownGroup) {
		if p.state == 'Z' && p.ppid == self {
			var ws unix.WaitStatus
			_, _ = unix.Wait4(p.pid, &ws, unix.WNOHANG, nil)
		}
	}
	return killed
}

// strays lists the descendants of self outside the tracked sessions and
// outside ownGroup.
func (t *tracker) strays(self, ownGroup int) []procStat {
	all := listProcs()
	children := make(map[int][]procStat)
	for _, p := range all {
		children[p.ppid] = append(children[p.ppid], p)
	}
	var out []procStat
	stack := []int{self}
	for len(stack) > 0 {
		pid := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, c := range children[pid] {
			stack = append(stack, c.pid)
			if !t.sessions[c.session] && c.pgrp != ownGroup {
				out = append(out, c)
			}
		}
	}
	return out
}

// procStat is what a sweep needs from /proc/<pid>/stat.
type procStat struct {
	pid, ppid, pgrp, session int
	state                    byte
	start                    uint64 // in clock ticks after boot; tells a reused PID apart
}

func (p procStat) stopped() bool {
	return p.state == 'T' || p.state == 't' || p.state == 'Z' || p.state == 'X'
}

// signal sends sig to p if p is still the process that was listed. A pidfd
// pins the process, and the start time read after opening it proves that
// the PID was not reused in between. Without pidfd (Linux before 5.3) only
// the runner's direct children are signalled: their PIDs cannot be reused
// until the runner reaps them.
func (p procStat) signal(self int, sig unix.Signal) {
	fd, err := unix.PidfdOpen(p.pid, 0)
	if err != nil {
		if errors.Is(err, unix.ENOSYS) && p.ppid == self {
			_ = unix.Kill(p.pid, sig)
		}
		return
	}
	defer func() { _ = unix.Close(fd) }()
	if now, err := readProcStat(p.pid); err != nil || now.start != p.start {
		return
	}
	_ = unix.PidfdSendSignal(fd, sig, nil, 0)
}

// listProcs reads every process the runner can see. Processes that exit
// while it reads are skipped.
func listProcs() []procStat {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	out := make([]procStat, 0, len(entries))
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if p, err := readProcStat(pid); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func readProcStat(pid int) (procStat, error) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return procStat{}, err
	}
	return parseProcStat(pid, string(data))
}

var errBadStat = errors.New("runner: malformed /proc stat line")

// parseProcStat parses a /proc/<pid>/stat line. The command name is in
// parentheses and may hold spaces and parentheses itself, so the fields
// are counted from the last ')'.
func parseProcStat(pid int, s string) (procStat, error) {
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return procStat{}, errBadStat
	}
	f := strings.Fields(s[i+1:])
	// f[0] is field 3 (state); starttime is field 22.
	if len(f) < 20 || len(f[0]) != 1 {
		return procStat{}, errBadStat
	}
	p := procStat{pid: pid, state: f[0][0]}
	var err error
	if p.ppid, err = strconv.Atoi(f[1]); err != nil {
		return procStat{}, errBadStat
	}
	if p.pgrp, err = strconv.Atoi(f[2]); err != nil {
		return procStat{}, errBadStat
	}
	if p.session, err = strconv.Atoi(f[3]); err != nil {
		return procStat{}, errBadStat
	}
	if p.start, err = strconv.ParseUint(f[19], 10, 64); err != nil {
		return procStat{}, errBadStat
	}
	return p, nil
}
