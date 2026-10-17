package runner

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Landlock access rights by the ABI version that introduced them. A right
// the kernel does not know cannot be handled, so it stays allowed.
const (
	fsRightsABI1 = unix.LANDLOCK_ACCESS_FS_EXECUTE | unix.LANDLOCK_ACCESS_FS_WRITE_FILE |
		unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_READ_DIR |
		unix.LANDLOCK_ACCESS_FS_REMOVE_DIR | unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
		unix.LANDLOCK_ACCESS_FS_MAKE_CHAR | unix.LANDLOCK_ACCESS_FS_MAKE_DIR |
		unix.LANDLOCK_ACCESS_FS_MAKE_REG | unix.LANDLOCK_ACCESS_FS_MAKE_SOCK |
		unix.LANDLOCK_ACCESS_FS_MAKE_FIFO | unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK |
		unix.LANDLOCK_ACCESS_FS_MAKE_SYM

	accessExec = unix.LANDLOCK_ACCESS_FS_EXECUTE | unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_READ_DIR
	accessRead = unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_READ_DIR
	accessDev  = unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_WRITE_FILE |
		unix.LANDLOCK_ACCESS_FS_READ_DIR | unix.LANDLOCK_ACCESS_FS_TRUNCATE | unix.LANDLOCK_ACCESS_FS_IOCTL_DEV
	// accessWrite is everything but creating device nodes, which nothing
	// in a run needs.
	accessWrite = ^uint64(unix.LANDLOCK_ACCESS_FS_MAKE_CHAR | unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK)
	// accessFile are the rights that apply to a file; a rule on a file
	// with directory rights is invalid.
	accessFile = unix.LANDLOCK_ACCESS_FS_EXECUTE | unix.LANDLOCK_ACCESS_FS_WRITE_FILE |
		unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_TRUNCATE | unix.LANDLOCK_ACCESS_FS_IOCTL_DEV

	netRights = unix.LANDLOCK_ACCESS_NET_BIND_TCP | unix.LANDLOCK_ACCESS_NET_CONNECT_TCP
	scopes    = unix.LANDLOCK_SCOPE_SIGNAL | unix.LANDLOCK_SCOPE_ABSTRACT_UNIX_SOCKET

	// landlockRuleNetPort is LANDLOCK_RULE_NET_PORT, which x/sys/unix
	// does not define.
	landlockRuleNetPort = 2
)

// landlockNetPortAttr is struct landlock_net_port_attr.
type landlockNetPortAttr struct {
	allowedAccess uint64
	port          uint64
}

// landlockABI is the kernel's Landlock ABI version, or 0 when Landlock is
// unavailable: an old kernel, Landlock missing from the boot's LSM list,
// or a seccomp profile that blocks its system calls.
var landlockABI = sync.OnceValue(func() int {
	v, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
	if errno != 0 {
		return 0
	}
	return int(v)
})

// handledFS returns the file system rights the kernel can restrict.
func handledFS(abi int) uint64 {
	r := uint64(fsRightsABI1)
	if abi >= 2 {
		r |= unix.LANDLOCK_ACCESS_FS_REFER
	}
	if abi >= 3 {
		r |= unix.LANDLOCK_ACCESS_FS_TRUNCATE
	}
	if abi >= 5 {
		r |= unix.LANDLOCK_ACCESS_FS_IOCTL_DEV
	}
	return r
}

// describe says how much of the sandbox the kernel enforces, for the log
// at start.
func (sb *sandbox) describe() (string, bool) {
	abi := landlockABI()
	if sb == nil || abi == 0 {
		return "none: Landlock is not available, so runs can read each other's files and signal the runner", false
	}
	parts := []string{"files", "ptrace"}
	if abi >= 4 {
		parts = append(parts, "tcp")
	}
	if abi >= 6 {
		parts = append(parts, "signals")
	}
	return fmt.Sprintf("landlock ABI %d: %s", abi, strings.Join(parts, ", ")), true
}

// start starts cmd confined to the sandbox, with full access to the
// workspace. Without Landlock it starts cmd unconfined. A kernel that has
// Landlock but refuses the rules fails the start: running learner code
// unconfined by accident is worse than a failed run.
func (sb *sandbox) start(cmd *exec.Cmd, workspace string) error {
	abi := landlockABI()
	if sb == nil || abi == 0 {
		return cmd.Start()
	}
	netFD, err := netRuleset()
	if err != nil {
		return err
	}
	fsFD, err := sb.ruleset(abi, workspace)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(fsFD) }()

	return onSpareThread(func() error { return restrictThreadAndStart(cmd, netFD, fsFD) })
}

var errMainThread = errors.New("sandbox: refusing to restrict the main thread")

// onSpareThread runs fn on a locked thread that is not the process's main
// thread, and lets that thread end with fn.
//
// Landlock and no_new_privs apply to the calling thread, and a child
// inherits them from the thread that forks it. The thread fn runs on is
// never unlocked, so the runtime ends it afterwards instead of running
// other code on it, and meanwhile creates new threads from a clean
// template thread.
func onSpareThread(fn func() error) error {
	errc := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		onMain, err := runOffMainThread(fn)
		if onMain {
			// fn ran on another thread; this one is clean.
			runtime.UnlockOSThread()
		}
		errc <- err
	}()
	return <-errc
}

// runOffMainThread runs fn on the calling thread, which the caller locked,
// unless it is the process's main thread. Then fn runs on a new locked
// thread, and the main thread stays held meanwhile so that fn cannot get
// it.
//
// The main thread must stay unrestricted: the runtime cannot end it, and
// the kernel checks a signal or a ptrace access to the process against
// it. A restricted main thread would put the runner inside the domain of
// the run that restricted it, and that run could signal the runner and
// read its memory.
func runOffMainThread(fn func() error) (onMain bool, err error) {
	if unix.Gettid() != unix.Getpid() {
		return false, fn()
	}
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		if unix.Gettid() == unix.Getpid() {
			done <- errMainThread // the caller holds the main thread; cannot happen
			return
		}
		done <- fn()
	}()
	return true, <-done
}

// restrictThreadAndStart restricts the calling thread with the layers and
// starts cmd from it.
func restrictThreadAndStart(cmd *exec.Cmd, layers ...int) error {
	// Required to restrict without CAP_SYS_ADMIN; it also stops a setuid
	// binary from gaining privileges in the run.
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("sandbox: no_new_privs: %w", err)
	}
	for _, fd := range layers {
		if fd < 0 {
			continue
		}
		if _, _, errno := unix.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF, uintptr(fd), 0, 0); errno != 0 {
			return fmt.Errorf("sandbox: landlock_restrict_self: %w", errno)
		}
	}
	return cmd.Start()
}

// ruleset builds the file system layer of 1 run. It also sets the scopes,
// which keep the run from signalling the runner (and other runs) and from
// reaching abstract Unix sockets outside it. Being in a Landlock domain
// also keeps it from ptrace and from /proc/<pid>/environ and mem of
// processes outside, such as PID 1, whose environment holds the token.
func (sb *sandbox) ruleset(abi int, workspace string) (fd int, err error) {
	handled := handledFS(abi)
	attr := unix.LandlockRulesetAttr{Access_fs: handled}
	if abi >= 6 {
		attr.Scoped = scopes
	}
	fd, err = createRuleset(&attr)
	if err != nil {
		return -1, err
	}
	defer func() {
		if err != nil {
			_ = unix.Close(fd)
			fd = -1
		}
	}()
	for _, set := range []struct {
		paths  []string
		access uint64
	}{
		{sb.exec, accessExec},
		{sb.read, accessRead},
		{sb.dev, accessDev},
		{sb.write, accessWrite},
		{[]string{workspace}, accessWrite},
	} {
		for _, p := range set.paths {
			if err := addPathRule(fd, p, set.access&handled); err != nil {
				return -1, err
			}
		}
	}
	return fd, nil
}

func createRuleset(attr *unix.LandlockRulesetAttr) (int, error) {
	// The kernel accepts the larger struct of a newer ABI as long as the
	// fields it does not know are 0.
	fd, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET,
		uintptr(unsafe.Pointer(attr)), unsafe.Sizeof(*attr), 0)
	if errno != 0 {
		return -1, fmt.Errorf("sandbox: landlock_create_ruleset: %w", errno)
	}
	return int(fd), nil
}

// addPathRule allows access beneath path. A path that does not exist or
// cannot be opened is skipped: it then stays out of reach, which is safe.
func addPathRule(ruleset int, path string, access uint64) error {
	fd, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, unix.ENOENT) || errors.Is(err, unix.ENOTDIR) || errors.Is(err, unix.EACCES) {
			return nil
		}
		return fmt.Errorf("sandbox: open %s: %w", path, err)
	}
	defer func() { _ = unix.Close(fd) }()
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return fmt.Errorf("sandbox: stat %s: %w", path, err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR {
		access &= accessFile
	}
	if access == 0 {
		return nil
	}
	attr := unix.LandlockPathBeneathAttr{Allowed_access: access, Parent_fd: int32(fd)}
	_, _, errno := unix.Syscall6(unix.SYS_LANDLOCK_ADD_RULE, uintptr(ruleset), unix.LANDLOCK_RULE_PATH_BENEATH,
		uintptr(unsafe.Pointer(&attr)), 0, 0, 0)
	if errno != 0 {
		return fmt.Errorf("sandbox: landlock_add_rule %s: %w", path, errno)
	}
	return nil
}

// netRuleset is the network layer shared by every run, or -1 when the
// kernel cannot restrict TCP (ABI < 4). A run may listen on an ephemeral
// port and connect to ephemeral ports, which is what tests with
// httptest.Server and net.Listen("tcp", "127.0.0.1:0") do. It may not
// connect to the fixed ports of services: the runner's own API and
// metrics, the Kubernetes API, DNS over TCP, web servers. The file
// descriptor stays open for the life of the process, because building the
// rules takes tens of milliseconds and they never change.
var netRuleset = sync.OnceValues(func() (int, error) {
	if landlockABI() < 4 {
		return -1, nil
	}
	fd, err := createRuleset(&unix.LandlockRulesetAttr{Access_net: netRights})
	if err != nil {
		return -1, err
	}
	lo, hi := ephemeralPorts()
	// Port 0 lets bind pick an ephemeral port.
	if err := addPortRule(fd, 0, unix.LANDLOCK_ACCESS_NET_BIND_TCP); err != nil {
		_ = unix.Close(fd)
		return -1, err
	}
	for port := lo; port <= hi; port++ {
		if err := addPortRule(fd, port, netRights); err != nil {
			_ = unix.Close(fd)
			return -1, err
		}
	}
	return fd, nil
})

func addPortRule(ruleset, port int, access uint64) error {
	attr := landlockNetPortAttr{allowedAccess: access, port: uint64(port)}
	_, _, errno := unix.Syscall6(unix.SYS_LANDLOCK_ADD_RULE, uintptr(ruleset), landlockRuleNetPort,
		uintptr(unsafe.Pointer(&attr)), 0, 0, 0)
	if errno != 0 {
		return fmt.Errorf("sandbox: landlock_add_rule port %d: %w", port, errno)
	}
	return nil
}

// Linux's default ephemeral port range, for when the sysctl cannot be read.
const (
	defaultEphemeralLow  = 32768
	defaultEphemeralHigh = 60999
)

// ephemeralPorts returns the range bind picks ports from in the runner's
// network namespace, which its children share.
func ephemeralPorts() (lo, hi int) {
	data, err := os.ReadFile("/proc/sys/net/ipv4/ip_local_port_range")
	if err != nil {
		return defaultEphemeralLow, defaultEphemeralHigh
	}
	return parsePortRange(string(data))
}

// parsePortRange parses "32768\t60999". A malformed range falls back to
// Linux's default.
func parsePortRange(s string) (lo, hi int) {
	f := strings.Fields(s)
	if len(f) != 2 {
		return defaultEphemeralLow, defaultEphemeralHigh
	}
	lo, err1 := strconv.Atoi(f[0])
	hi, err2 := strconv.Atoi(f[1])
	if err1 != nil || err2 != nil || lo < 1 || hi > 65535 || lo > hi {
		return defaultEphemeralLow, defaultEphemeralHigh
	}
	return lo, hi
}
