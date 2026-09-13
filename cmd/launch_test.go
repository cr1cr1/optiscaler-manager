package optiscalermanager

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/gh"
	"github.com/cr1cr1/optiscaler-manager/internal/launch"
)

// cmdLaunchCapture records the argv handed to the injected runner.
type cmdLaunchCapture struct {
	mu   sync.Mutex
	name string
	args []string
}

func (c *cmdLaunchCapture) runner() launch.Runner {
	return func(_ context.Context, dir, name string, args ...string) error {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.name, c.args = name, append([]string(nil), args...)
		return nil
	}
}

func (c *cmdLaunchCapture) argv() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.name == "" {
		return nil
	}
	return append([]string{c.name}, c.args...)
}

// TestLaunchCommandRunsRunner: launch through the injected launcher seam —
// the steam:// deep link fires and the command reports the request.
func TestLaunchCommandRunsRunner(t *testing.T) {
	srv := fakeGitHub(t)
	steamRoot, gameRoot := fakeSteam(t)
	d, out := testDeps(t, gh.NewWithBaseURL(nil, filepath.Join(t.TempDir(), "cache"), srv.URL))
	d.SteamRoot = steamRoot
	capture := &cmdLaunchCapture{}
	d.Launcher = launch.New(capture.runner(), "linux", func(string) (string, error) { return "", errors.New("not found") })
	cmdQuickInstall(t, d, gameRoot)

	if err := (&LaunchCmd{Path: gameRoot}).Run(d); err != nil {
		t.Fatalf("LaunchCmd: %v", err)
	}
	argv := capture.argv()
	found := false
	for _, a := range argv {
		if strings.Contains(a, "steam://rungameid/100") {
			found = true
		}
	}
	if !found {
		t.Fatalf("captured argv %v lacks steam://rungameid/100", argv)
	}
	if !strings.Contains(out.String(), "Launch requested") {
		t.Errorf("output missing the launch report:\n%s", out.String())
	}
	t.Logf("captured launch argv: %v; output:\n%s", argv, out.String())
}

// TestLaunchCommandUnknownDirFails: an unknown dir is a clean exit-1
// failure, not a hang.
func TestLaunchCommandUnknownDirFails(t *testing.T) {
	srv := fakeGitHub(t)
	steamRoot, _ := fakeSteam(t)
	d, _ := testDeps(t, gh.NewWithBaseURL(nil, filepath.Join(t.TempDir(), "cache"), srv.URL))
	d.SteamRoot = steamRoot

	err := (&LaunchCmd{Path: filepath.Join(steamRoot, "nowhere"), Timeout: 5 * time.Second}).Run(d)
	exit, ok := err.(*ExitError)
	if !ok {
		t.Fatalf("err = %v, want *ExitError", err)
	}
	if exit.Code != 1 {
		t.Errorf("exit code = %d, want 1", exit.Code)
	}
}

// TestLaunchCommandFailureFails: a failing runner seam settles the launch
// with EvOpFailed and the command exits 1 with the reason.
func TestLaunchCommandFailureFails(t *testing.T) {
	srv := fakeGitHub(t)
	steamRoot, gameRoot := fakeSteam(t)
	d, out := testDeps(t, gh.NewWithBaseURL(nil, filepath.Join(t.TempDir(), "cache"), srv.URL))
	d.SteamRoot = steamRoot
	d.Launcher = launch.New(func(context.Context, string, string, ...string) error {
		return errors.New("boom")
	}, "linux", func(string) (string, error) { return "", errors.New("not found") })
	cmdQuickInstall(t, d, gameRoot)

	err := (&LaunchCmd{Path: gameRoot}).Run(d)
	exit, ok := err.(*ExitError)
	if !ok {
		t.Fatalf("err = %v, want *ExitError", err)
	}
	if exit.Code != 1 {
		t.Errorf("exit code = %d, want 1", exit.Code)
	}
	if !strings.Contains(exit.Error(), "boom") {
		t.Errorf("err = %v, want the runner's error", exit.Error())
	}
	t.Logf("failed launch: %v; output:\n%s", err, out.String())
}
