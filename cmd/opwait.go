package optiscalermanager

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// answerGateFn indirection lets tests accept/decline consent gates
// without a real terminal (the CLI's stdin is /dev/null under go test).
// It answers the pending gate and reports whether it was accepted.
var answerGateFn = answerGate

// defaultOpTimeout is the ops' wait ceiling. A zero Timeout (struct built
// without the flag machinery, or --timeout 0) means the default.
const defaultOpTimeout = 10 * time.Minute

func opTimeout(t time.Duration) time.Duration {
	if t <= 0 {
		return defaultOpTimeout
	}
	return t
}

// waitForOp blocks until the session finishes, refuses, or cancels the op
// for gameDir — the CLI's equivalent of the GUI/TUI event loops. Consent
// gates (EvConfirm) are answered on the terminal (prompts go to w); a
// declined or non-interactive gate ends the wait with a "declined" error
// and the EvConfirm event — a declined op never started, so nothing
// further arrives for it. Consent is never bypassed (docs/safety.md);
// events for other game dirs are ignored.
func waitForOp(sess *ui.Session, dir string, timeout time.Duration, w io.Writer) (ui.Event, error) {
	return waitForOpKinds(sess, dir, timeout, w, ui.EvOpDone, ui.EvOpFailed, ui.EvOpCancelled)
}

// waitForOpKinds is waitForOp with an explicit terminal-kind set: compound
// ops (the version-switch chain) settle ONLY with EvOpSettled — their
// uninstall/install sub-legs emit done/failed events mid-flight that must
// not be mistaken for the end.
func waitForOpKinds(sess *ui.Session, dir string, timeout time.Duration, w io.Writer, terminals ...ui.EventKind) (ui.Event, error) {
	terminal := func(k ui.EventKind) bool {
		for _, t := range terminals {
			if k == t {
				return true
			}
		}
		return false
	}
	deadline := time.After(timeout)
	for {
		select {
		case ev := <-sess.Events():
			if ev.GameDir != "" && ev.GameDir != dir {
				continue
			}
			if ev.Kind == ui.EvConfirm {
				if !answerGateFn(sess, w) {
					return ev, fmt.Errorf("declined: %s", ev.Text)
				}
				continue
			}
			if terminal(ev.Kind) {
				return ev, nil
			}
		case <-deadline:
			return ui.Event{}, fmt.Errorf("timeout after %s waiting for the operation on %s", timeout, dir)
		}
	}
}

// awaitRow blocks until dir is in the session snapshot. A one-shot boot is
// asynchronous: a warm games cache settles synchronously (rows present when
// Start returns), a cold boot scans in flight — await its settle, then
// report the row or a not-found error. When the warm cache predates the
// game (installed since the last scan), one explicit rescan can surface it
// — a one-shot command cannot scan interactively.
func awaitRow(sess *ui.Session, dir string, timeout time.Duration) (ui.GameRow, error) {
	if row, ok := rowOfSession(sess, dir); ok {
		return row, nil
	}
	if len(sess.Snapshot().Rows) > 0 {
		sess.Scan(cmdContext())
	} // else a cold boot's scan is already in flight
	deadline := time.After(timeout)
	for {
		select {
		case ev := <-sess.Events():
			if ev.Kind != ui.EvScanDone && ev.Kind != ui.EvScanFailed {
				continue
			}
			if row, ok := rowOfSession(sess, dir); ok {
				return row, nil
			}
			if ev.Kind == ui.EvScanFailed {
				return ui.GameRow{}, fmt.Errorf("scan failed (%s): game dir %s not found", ev.Text, dir)
			}
			return ui.GameRow{}, fmt.Errorf("game dir %s not found in the scanned libraries", dir)
		case <-deadline:
			return ui.GameRow{}, fmt.Errorf("timeout after %s waiting for the games list", timeout)
		}
	}
}

// answerGate renders the pending confirmation to w and answers it from
// the terminal. A non-TTY stdin declines (consent is never bypassed);
// reports whether the gate was accepted.
func answerGate(sess *ui.Session, w io.Writer) bool {
	c := sess.Snapshot().Confirm
	if c == nil {
		return true // nothing pending; the gate was answered elsewhere
	}
	if !term.IsTerminal(os.Stdin.Fd()) {
		fmt.Fprintf(w, "refused: %s requires consent and stdin is not interactive\n", c.Message)
		sess.AnswerConfirm(false)
		return false
	}
	fmt.Fprintf(w, "%s\nproceed? [y/n] ", c.Message)
	ans, err := bufio.NewReader(os.Stdin).ReadString('\n')
	a := strings.ToLower(strings.TrimSpace(ans))
	accept := err == nil && (a == "y" || a == "yes")
	sess.AnswerConfirm(accept)
	if !accept {
		fmt.Fprintln(w, "declined")
	}
	return accept
}
