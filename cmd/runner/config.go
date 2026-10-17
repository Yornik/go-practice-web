package main

import (
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// envPrefix is the prefix of every runner variable.
const envPrefix = "GPR_"

// minTokenLen is the shortest accepted GPR_TOKEN. A token is generated
// (openssl rand -hex 32 gives 64 characters); a short one is a placeholder
// or a typo, and the runner executes code for whoever holds it.
const minTokenLen = 32

// config is the runner configuration from GPR_* variables; see the Runner
// table in docs/DESIGN.md.
type config struct {
	Addr          string
	MetricsAddr   string
	Token         string
	CourseDir     string
	WorkDir       string
	GoCache       string
	GoCacheSeed   string
	MaxConcurrent int
	MaxQueue      int
	MaxQueueWait  time.Duration
	TestTimeout   time.Duration
	CheckTimeout  time.Duration
}

// knownVars are the variables loadConfig reads. Any other GPR_ variable is
// an error: a typo would otherwise silently fall back to a default.
var knownVars = []string{
	"GPR_ADDR", "GPR_METRICS_ADDR", "GPR_TOKEN", "GPR_COURSE_DIR", "GPR_WORK_DIR",
	"GPR_GOCACHE", "GPR_GOCACHE_SEED", "GPR_MAX_CONCURRENT", "GPR_MAX_QUEUE",
	"GPR_MAX_QUEUE_WAIT", "GPR_TEST_TIMEOUT", "GPR_CHECK_TIMEOUT",
}

// loadConfig reads the configuration from environ (os.Environ format). It
// reports every problem at once, so that 1 failed start shows them all.
func loadConfig(environ []string) (config, error) {
	env := make(map[string]string)
	var errs []error
	for _, kv := range environ {
		key, value, _ := strings.Cut(kv, "=")
		if !strings.HasPrefix(key, envPrefix) {
			continue
		}
		if !slices.Contains(knownVars, key) {
			errs = append(errs, fmt.Errorf("%s: unknown variable", key))
			continue
		}
		env[key] = value
	}
	get := func(key, def string) string {
		if v, ok := env[key]; ok {
			return v
		}
		return def
	}

	c := config{
		Addr:        get("GPR_ADDR", ":8081"),
		MetricsAddr: get("GPR_METRICS_ADDR", ":9091"),
		Token:       get("GPR_TOKEN", ""),
		CourseDir:   get("GPR_COURSE_DIR", "/course"),
		WorkDir:     get("GPR_WORK_DIR", "/tmp/runs"),
		GoCache:     get("GPR_GOCACHE", "/cache/go-build"),
		GoCacheSeed: get("GPR_GOCACHE_SEED", "/opt/gocache-seed"),
	}
	errs = append(errs,
		checkAddr("GPR_ADDR", c.Addr),
		checkAddr("GPR_METRICS_ADDR", c.MetricsAddr),
		checkAbs("GPR_COURSE_DIR", c.CourseDir),
		checkAbs("GPR_WORK_DIR", c.WorkDir),
		checkAbs("GPR_GOCACHE", c.GoCache),
	)
	if c.GoCacheSeed != "" {
		errs = append(errs, checkAbs("GPR_GOCACHE_SEED", c.GoCacheSeed))
	}
	if len(c.Token) < minTokenLen {
		errs = append(errs, fmt.Errorf("GPR_TOKEN: required, at least %d characters", minTokenLen))
	}
	if c.Addr == c.MetricsAddr {
		errs = append(errs, errors.New("GPR_METRICS_ADDR: must differ from GPR_ADDR"))
	}

	var err error
	c.MaxConcurrent, err = parseInt("GPR_MAX_CONCURRENT", get("GPR_MAX_CONCURRENT", "2"), 1)
	errs = append(errs, err)
	c.MaxQueue, err = parseInt("GPR_MAX_QUEUE", get("GPR_MAX_QUEUE", "8"), 0)
	errs = append(errs, err)
	c.MaxQueueWait, err = parseDuration("GPR_MAX_QUEUE_WAIT", get("GPR_MAX_QUEUE_WAIT", "12s"))
	errs = append(errs, err)
	c.TestTimeout, err = parseDuration("GPR_TEST_TIMEOUT", get("GPR_TEST_TIMEOUT", "40s"))
	errs = append(errs, err)
	c.CheckTimeout, err = parseDuration("GPR_CHECK_TIMEOUT", get("GPR_CHECK_TIMEOUT", "20s"))
	errs = append(errs, err)

	if err := errors.Join(errs...); err != nil {
		return config{}, fmt.Errorf("invalid configuration:\n%w", err)
	}
	return c, nil
}

func checkAddr(key, addr string) error {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return fmt.Errorf("%s: %q is not a host:port address: %w", key, addr, err)
	}
	return nil
}

// checkAbs requires an absolute path: GOCACHE must be absolute for go, and
// a relative path would depend on the working directory of the pod.
func checkAbs(key, path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("%s: %q is not an absolute path", key, path)
	}
	return nil
}

func parseInt(key, s string, minimum int) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%s: %q is not an integer", key, s)
	}
	if n < minimum {
		return 0, fmt.Errorf("%s: %d is below the minimum %d", key, n, minimum)
	}
	return n, nil
}

func parseDuration(key, s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("%s: %q is not a duration such as 40s", key, s)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s: must be positive", key)
	}
	return d, nil
}
