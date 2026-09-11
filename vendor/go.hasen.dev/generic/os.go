package generic

import (
	"os"
	"strings"
)

var cleanupFns []func()

func AddExitCleanup(fn func()) {
	Append(&cleanupFns, fn)
}

func safeCall(fn func()) {
	defer func() {
		recover()
	}()
	fn()
}

func ExitWithCleanup(code int) {
	Cleanup()
	os.Exit(code)
}

func Cleanup() {
	for i := len(cleanupFns) - 1; i >= 0; i-- {
		safeCall(cleanupFns[i])
	}
}

func envNorm(v string) string {
	return strings.TrimSpace(strings.ToLower(v))
}

func EnvTruthy(key string) bool {
	switch envNorm(os.Getenv(key)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// EnvFalsy is the inverse token set of EnvTruthy. Unset is neither.
func EnvFalsy(key string) bool {
	switch envNorm(os.Getenv(key)) {
	case "0", "false", "off", "no":
		return true
	}
	return false
}
