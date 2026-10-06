// Package tui is the bubbletea frontend over the frontend-agnostic
// ui.Session: it renders session snapshots and forwards keypresses to
// session commands. It contains no business logic — install/uninstall/
// launch/rollback semantics live in internal/ui and below.
package tui

import (
	"context"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/termopen"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
	"github.com/cr1cr1/optiscaler-manager/internal/version"
)

// screen identifies the active top-level screen (tab bar order).
type screen int

const (
	screenGames screen = iota
	screenDetail
	screenSettings
	screenHelp
	screenAbout
)

// inputMode identifies which text input is currently capturing keys.
type inputMode int

const (
	inputNone inputMode = iota
	inputFilter
	inputAddDir
	inputEditVersion
	inputEditTemplate
	inputEditUmuProton
	inputAddForkSlug
	inputAddForkPattern
)

// settingsFocus names the settings-screen list j/k/a/d act on: the scan
// directories (default, legacy behavior) or the OptiScaler sources list
// (tab toggles).
type settingsFocus int

const (
	settingsFocusDirs settingsFocus = iota
	settingsFocusForks
)

// Model is the bubbletea model bound to one ui.Session: one flat model with
// per-screen update/view handlers.
type Model struct {
	sess    *ui.Session
	version string // build version, rendered on the About screen

	screen          screen
	cursor          int // games row cursor
	dirCursor       int // settings directory cursor
	forkCursor      int // settings OptiScaler-source cursor
	settingsFocus   settingsFocus
	detailDir       string
	width           int
	height          int
	gamesVP         viewport.Model
	detailVP        viewport.Model
	settingsVP      viewport.Model
	input           textinput.Model
	mode            inputMode
	spin            spinner.Model
	confirmRmDir    string // directory pending inline remove confirmation
	confirmRmFork   string // fork slug pending inline remove confirmation
	pendingForkSlug string // slug carried from the add-fork slug input to the pattern input
	cycle           *stagedCycle
	backups         []stagedItem // detail dir's DLSS backups (menu + modal)
	restore         *restorePick // open restore-backup modal
}

// restorePick is the open restore-backup modal: the detail dir, its cached
// backup list, and the highlighted entry.
type restorePick struct {
	dir   string
	items []stagedItem
	sel   int
}

// stagedItem is one pickable entry: ID selects the target (an OptiScaler
// version tag, or a DLSS snapshot id); Label is what the stage line or the
// picker row renders.
type stagedItem struct {
	ID    string
	Label string
}

// stagedCycle is a staged version-switch pick: the session-provided
// candidates snapshotted at the staging keypress, the index of the
// currently shown candidate, and the version the row had at staging time
// (so confirming the unchanged version is suppressed here, not just in the
// core). latestIsCur records that the Latest row absorbed the current
// version (staging-time fact), so its confirm is suppressed like the S13
// wrap no-op.
type stagedCycle struct {
	dir         string
	items       []stagedItem
	idx         int
	cur         string
	latestIsCur bool
}

// stageCycle starts version staging on dir's row: the first 'v' snapshots
// Session.Versions(dir) (filesystem+memo reads, so it runs on the keypress
// only — never per frame) and immediately advances to the first candidate
// after the installed version. When the known-latest tag is set, the entry
// semver-equal to it is REPLACED by a single "Latest (tag)" row in place
// (never duplicated, GUI versionMenuRows parity); when the list carries no
// such entry the Latest row is prepended. The Latest row dispatches the
// literal "latest", which the session core resolves at pick time. Cycle
// order and the landing rule are unchanged when no latest is known.
// Never-installed rows and cycles with fewer than two selectable entries
// are a no-op.
func (m *Model) stageCycle(dir string) {
	row := findRow(m.sess.Snapshot().Rows, dir)
	if row == nil || !switchable(*row) {
		return
	}
	list := m.sess.Versions(dir)
	latest := m.sess.LatestKnown()
	items := make([]stagedItem, 0, len(list)+1)
	idx := -1 // not found: the advance below lands on the first entry
	latestSeen := false
	for _, v := range list {
		if latest != "" && !latestSeen && version.Compare(v, latest) == 0 {
			// ponytail: ID is the literal the session core resolves at pick
			// time (doSwitchVersion's "latest" branch), mirroring the GUI row.
			latestSeen = true
			items = append(items, stagedItem{ID: "latest", Label: "Latest (" + latest + ")"})
		} else {
			items = append(items, stagedItem{ID: v, Label: v})
		}
		if v == row.OptiScalerVersion {
			idx = len(items) - 1
		}
	}
	if latest != "" && !latestSeen {
		items = append([]stagedItem{{ID: "latest", Label: "Latest (" + latest + ")"}}, items...)
		if idx >= 0 {
			idx++ // the prepend shifts every concrete position
		}
	}
	if len(items) < 2 {
		return
	}
	m.cycle = &stagedCycle{dir: dir, items: items, idx: idx, cur: row.OptiScalerVersion,
		latestIsCur: latest != "" && version.Compare(latest, row.OptiScalerVersion) == 0}
	m.advanceCycle()
}

// refreshBackups caches dir's DLSS backup list for the detail menu and the
// restore modal: one disk read on detail entry and whenever a settled op
// event for the dir flows through Update (updates and restores both add a
// backup) — never per frame.
func (m *Model) refreshBackups(dir string) {
	snaps := m.sess.DLSSSnapshots(dir)
	items := make([]stagedItem, len(snaps))
	for i, s := range snaps {
		items[i] = stagedItem{ID: s.ID, Label: s.Label()}
	}
	m.backups = items
}

// settledOp reports whether kind ends an op that ran for a game dir.
func settledOp(k ui.EventKind) bool {
	switch k {
	case ui.EvOpDone, ui.EvOpFailed, ui.EvOpCancelled, ui.EvOpSettled:
		return true
	}
	return false
}

// advanceCycle moves the staged candidate to the next entry, wrapping
// around; the stage is dropped if the row lost its eligibility mid-cycle.
func (m *Model) advanceCycle() {
	c := m.cycle
	if c == nil {
		return
	}
	if row := findRow(m.sess.Snapshot().Rows, c.dir); row == nil || !switchable(*row) {
		m.cycle = nil
		return
	}
	c.idx = (c.idx + 1) % len(c.items)
}

// confirmCycle dispatches the staged version pick only when the candidate
// differs from the version at staging time (wrapping back to the current
// version, S13, dispatches nothing; a Latest row that absorbed the current
// version is that same no-op). A non-absorbed Latest row dispatches the
// literal "latest", which the session core resolves at pick time.
func (m *Model) confirmCycle() {
	c := m.cycle
	m.cycle = nil
	if c == nil {
		return
	}
	cand := c.items[c.idx]
	if cand.ID == "latest" && c.latestIsCur {
		return
	}
	if cand.ID != c.cur {
		m.sess.SwitchVersion(c.dir, cand.ID)
	}
}

// hasDLSS reports whether the row is DLSS-ready: the complete three-file
// NVIDIA runtime set is present, so update and restore can run. A bare
// "DLSS" pill (version-stripped DLLs) is ready; a DLSS-FG-only game is not.
func hasDLSS(r ui.GameRow) bool {
	return r.DLSSReady
}

// canRestore is the single gate for opening the restore picker: the row is
// DLSS-ready and at least one cached backup exists — the same condition
// under which the detail menu renders the action enabled.
func (m *Model) canRestore(row ui.GameRow) bool {
	return hasDLSS(row) && len(m.backups) > 0
}

// findRow returns the snapshot row for dir, or nil.
func findRow(rows []ui.GameRow, dir string) *ui.GameRow {
	for i := range rows {
		if rows[i].InstallDir == dir {
			return &rows[i]
		}
	}
	return nil
}

// switchable reports whether a row has an OptiScaler install the version
// cycler may retarget (committed or external — anything else has nothing
// to switch FROM).
func switchable(r ui.GameRow) bool {
	return r.HasInstall()
}

// eventMsg carries one session event into the update loop.
type eventMsg ui.Event

// New builds the TUI model over sess; version is the build version shown on
// the About screen ("" renders as "dev").
func New(sess *ui.Session, version string) Model {
	ti := textinput.New()
	return Model{
		sess:    sess,
		version: version,
		input:   ti,
		spin:    spinner.New(spinner.WithSpinner(spinner.Dot)),
	}
}

// Init boots the library cache-first (warm cache hydrates rows without a
// scan; a cold cache falls through to one) and subscribes to session events.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			m.sess.Start(context.Background())
			return nil
		},
		waitEvent(m.sess.Events()),
		m.spin.Tick,
	)
}

// waitEvent is the channel→Cmd bridge: one session event per tea.Msg,
// resubscribed after every event so the stream keeps flowing.
func waitEvent(events <-chan ui.Event) tea.Cmd {
	return func() tea.Msg {
		return eventMsg(<-events)
	}
}

// Update routes session events, terminal resizes, spinner ticks, and keys.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case eventMsg:
		// The event is only a poke; View re-reads the snapshot. A settled
		// op for the detail game also refreshes the cached backup list,
		// since updates and restores both add a backup — and re-syncs an
		// open picker, which may have been raised from the pre-op cache.
		if ev := ui.Event(msg); ev.GameDir == m.detailDir && settledOp(ev.Kind) {
			m.refreshBackups(m.detailDir)
			if m.restore != nil {
				m.restore.items = m.backups
				switch {
				case len(m.restore.items) == 0:
					m.restore = nil
				case m.restore.sel >= len(m.restore.items):
					m.restore.sel = len(m.restore.items) - 1
				}
			}
		}
		m.clamp()
		return m, waitEvent(m.sess.Events())
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case iniEditorMsg:
		if msg.err != nil {
			m.sess.Toast("editor: "+msg.err.Error(), true)
		}
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// clamp keeps both cursors inside their (possibly shrunken) lists.
func (m *Model) clamp() {
	if n := len(m.sess.VisibleRows()); n == 0 {
		m.cursor = 0
	} else if m.cursor >= n {
		m.cursor = n - 1
	}
	if n := len(m.sess.Settings().ExtraDirs); n == 0 {
		m.dirCursor = 0
	} else if m.dirCursor >= n {
		m.dirCursor = n - 1
	}
	if n := len(m.sess.Settings().Forks); n == 0 {
		m.forkCursor = 0
	} else if m.forkCursor >= n {
		m.forkCursor = n - 1
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	snap := m.sess.Snapshot()

	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	// A pending session confirmation is modal: only its answers are accepted.
	if snap.Confirm != nil {
		switch msg.String() {
		case "y", "Y":
			m.sess.AnswerConfirm(true)
		case "n", "N", "esc", "enter":
			m.sess.AnswerConfirm(false)
		}
		return m, nil
	}

	// The settings remove-directory confirmation is modal too.
	if m.confirmRmDir != "" {
		switch msg.String() {
		case "y", "Y":
			m.sess.RemoveDirectory(m.confirmRmDir)
			m.confirmRmDir = ""
		case "n", "N", "esc":
			m.confirmRmDir = ""
		}
		return m, nil
	}

	// As is the settings remove-fork confirmation.
	if m.confirmRmFork != "" {
		switch msg.String() {
		case "y", "Y":
			_ = m.sess.RemoveFork(m.confirmRmFork)
			m.confirmRmFork = ""
		case "n", "N", "esc":
			m.confirmRmFork = ""
		}
		return m, nil
	}

	// An open text input captures all keys until committed or cancelled.
	if m.mode != inputNone {
		switch msg.Type {
		case tea.KeyEsc:
			m.cancelInput()
			return m, nil
		case tea.KeyEnter:
			return m, m.commitInput()
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if m.mode == inputFilter {
			m.sess.SetQuery(m.input.Value())
			m.cursor = 0
		}
		return m, cmd
	}

	// The restore-backup picker is modal: only its navigation keys are
	// accepted until it is closed, so no staged pick, screen switch, or
	// quit can fire underneath it.
	if m.restore != nil {
		switch msg.String() {
		case "j", "down":
			if m.restore.sel < len(m.restore.items)-1 {
				m.restore.sel++
			}
		case "k", "up":
			if m.restore.sel > 0 {
				m.restore.sel--
			}
		case "enter":
			pick := m.restore
			m.restore = nil
			m.sess.RestoreDLSS(pick.dir, pick.items[pick.sel].ID)
		case "esc":
			m.restore = nil
		}
		return m, nil
	}

	// A staged version switch is row-modal: 'v' moves the candidate, Enter
	// confirms (dispatching only when the candidate differs from the
	// staged-from version), Esc cancels. Any other key drops the stage and
	// falls through to its normal binding, so cursor moves, screen
	// switches, and rescans can never carry a stale stage along.
	if m.cycle != nil {
		switch msg.String() {
		case "v":
			m.advanceCycle()
			return m, nil
		case "enter":
			m.confirmCycle()
			return m, nil
		case "esc":
			m.cycle = nil
			return m, nil
		default:
			m.cycle = nil
		}
	}

	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "1":
		m.screen = screenGames
		return m, nil
	case "2":
		m.screen = screenSettings
		return m, nil
	case "3":
		m.screen = screenHelp
		return m, nil
	case "4":
		m.screen = screenAbout
		return m, nil
	}

	switch m.screen {
	case screenGames:
		return m.gamesKey(msg)
	case screenDetail:
		return m.detailKey(msg)
	case screenSettings:
		return m.settingsKey(msg)
	}
	return m, nil
}

func (m Model) gamesKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := m.sess.VisibleRows()
	switch msg.String() {
	case "j", "down":
		if m.cursor < len(rows)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter":
		if dir := selectedDir(rows, m.cursor); dir != "" {
			// The TUI keeps its own detailDir (no session Select), so
			// re-probe the install state here like Select does.
			m.sess.RefreshInstallState(dir)
			m.detailDir = dir
			m.screen = screenDetail
			m.refreshBackups(dir)
		}
	case "i":
		if dir := selectedDir(rows, m.cursor); dir != "" {
			m.sess.QuickInstall(dir)
		}
	case "v":
		if dir := selectedDir(rows, m.cursor); dir != "" {
			m.stageCycle(dir)
		}
	case "l":
		if dir := selectedDir(rows, m.cursor); dir != "" {
			m.sess.Launch(dir)
		}
	case "u":
		if dir := selectedDir(rows, m.cursor); dir != "" {
			m.sess.UpdateDLSS(dir)
		}
	case "c":
		if dir := selectedDir(rows, m.cursor); dir != "" {
			m.sess.CancelOp(dir)
		}
	case "/":
		m.openInput(inputFilter, "/ ", m.sess.Snapshot().Query)
	case "R":
		m.sess.Scan(context.Background())
	case "s":
		if m.sess.Snapshot().Sort == ui.SortName {
			m.sess.SetSort(ui.SortDefault)
		} else {
			m.sess.SetSort(ui.SortName)
		}
	}
	return m, nil
}

func (m Model) detailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	dir := m.detailDir
	switch msg.String() {
	case "esc", "enter":
		m.screen = screenGames
	case "i":
		m.sess.QuickInstall(dir)
	case "v":
		m.stageCycle(dir)
	case "l":
		m.sess.Launch(dir)
	case "c":
		m.sess.CancelOp(dir)
	case "r":
		if row := m.detailRow(); row != nil && row.Actionable {
			m.sess.Rollback(dir)
		}
	case "d":
		m.sess.ToggleDisabled(dir)
	case "u":
		m.sess.UpdateDLSS(dir)
	case "p":
		// Restore opens the backup picker from the cache filled on detail
		// entry and settle events; anything the menu renders dimmed —
		// not DLSS-ready, or no backups — is a no-op.
		if row := m.detailRow(); row != nil && m.canRestore(*row) {
			m.restore = &restorePick{dir: dir, items: m.backups, sel: 0}
		}
	case "o":
		if row := m.detailRow(); row != nil && row.CanOpenINI() {
			return m, openINIEditor(m.sess, dir)
		}
	}
	return m, nil
}

// execEditor runs an external editor over the TUI (tea.ExecProcess in
// production); tests substitute a capture.
var execEditor = tea.ExecProcess

// iniEditorMsg reports the external editor's exit.
type iniEditorMsg struct{ err error }

// openINIEditor suspends the TUI and runs the user's terminal editor
// ($EDITOR → micro → nano → vi via termopen.Editor) on the game's
// OptiScaler.ini, resuming when it exits. tea.ExecProcess is the
// charmbracelet mechanism for external TUIs — an external process cannot
// render inside a subwindow, so the editor takes over the terminal as a
// modal and the TUI repaints on return.
func openINIEditor(sess *ui.Session, dir string) tea.Cmd {
	path := sess.INIPath(dir)
	if path == "" {
		return nil
	}
	argv, err := termopen.Editor(nil, nil)
	if err != nil {
		return func() tea.Msg { return iniEditorMsg{err} }
	}
	argv = append(argv, path)
	return execEditor(exec.Command(argv[0], argv[1:]...), func(err error) tea.Msg {
		return iniEditorMsg{err}
	})
}

func (m Model) settingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	dirs := m.sess.Settings().ExtraDirs
	forks := m.sess.Settings().Forks
	forkList := m.settingsFocus == settingsFocusForks
	switch msg.String() {
	case "tab":
		if forkList {
			m.settingsFocus = settingsFocusDirs
		} else {
			m.settingsFocus = settingsFocusForks
		}
	case "j", "down":
		if forkList {
			if m.forkCursor < len(forks)-1 {
				m.forkCursor++
			}
		} else if m.dirCursor < len(dirs)-1 {
			m.dirCursor++
		}
	case "k", "up":
		if forkList {
			if m.forkCursor > 0 {
				m.forkCursor--
			}
		} else if m.dirCursor > 0 {
			m.dirCursor--
		}
	case "enter":
		if forkList && m.forkCursor < len(forks) {
			_ = m.sess.SetActiveFork(forks[m.forkCursor].Slug)
		}
	case "e":
		m.openInput(inputEditVersion, "default version: ", m.sess.Settings().DefaultVersion)
	case "t":
		m.openInput(inputEditTemplate, "launch template: ", m.sess.Settings().LaunchTemplate)
	case "a":
		if forkList {
			m.openInput(inputAddForkSlug, "fork slug: ", "")
		} else {
			m.openInput(inputAddDir, "add dir: ", "")
		}
	case "d":
		if forkList {
			if m.forkCursor < len(forks) {
				slug := forks[m.forkCursor].Slug
				if slug == settings.DefaultForkSlug {
					// The session refusal toasts why; no inline confirm.
					_ = m.sess.RemoveFork(slug)
				} else {
					m.confirmRmFork = slug
				}
			}
		} else if m.dirCursor < len(dirs) {
			m.confirmRmDir = dirs[m.dirCursor]
		}
	case "o":
		m.sess.SetOnlineLookups(!m.sess.Settings().OnlineLookups)
	case "u":
		m.sess.SetUmuEnabled(!m.sess.Settings().UmuEnabled)
	case "p":
		m.openInput(inputEditUmuProton, "umu Proton path: ", m.sess.Settings().UmuProtonPath)
	case "x":
		m.sess.ClearBundleCache()
	}
	return m, nil
}

// detailRow re-reads the selected game's row from a single snapshot.
func (m Model) detailRow() *ui.GameRow {
	rows := m.sess.Snapshot().Rows
	for i := range rows {
		if rows[i].InstallDir == m.detailDir {
			row := rows[i]
			return &row
		}
	}
	return nil
}

func (m *Model) openInput(mode inputMode, prompt, initial string) {
	m.mode = mode
	m.input.Prompt = prompt
	m.input.SetValue(initial)
	m.input.Focus()
}

func (m *Model) cancelInput() {
	if m.mode == inputFilter {
		m.sess.SetQuery("")
	}
	m.mode = inputNone
	m.pendingForkSlug = ""
	m.input.SetValue("")
	m.input.Blur()
}

func (m *Model) commitInput() tea.Cmd {
	v := strings.TrimSpace(m.input.Value())
	// The add-fork flow is a two-input chain: slug, then asset glob.
	if m.mode == inputAddForkSlug && v != "" {
		m.pendingForkSlug = v
		m.openInput(inputAddForkPattern, "asset glob: ", "")
		return nil
	}
	var cmd tea.Cmd
	switch m.mode {
	case inputFilter:
		// the query already narrowed live; Enter only closes the input
	case inputAddDir:
		if v != "" {
			// AddDirectory classifies synchronously; run it as a command so
			// the update loop never blocks on the filesystem.
			cmd = func() tea.Msg {
				m.sess.AddDirectory(v) // invalid paths toast through the session
				return nil
			}
		}
	case inputEditVersion:
		m.sess.SetDefaultVersion(v)
	case inputEditTemplate:
		m.sess.SetLaunchTemplate(v)
	case inputEditUmuProton:
		m.sess.SetUmuProtonPath(v)
	case inputAddForkPattern:
		if v != "" && m.pendingForkSlug != "" {
			// AddFork validates and toasts the reason on refusal.
			_ = m.sess.AddFork(settings.Fork{Slug: m.pendingForkSlug, AssetPattern: v})
		}
		m.pendingForkSlug = ""
	}
	m.mode = inputNone
	m.input.SetValue("")
	m.input.Blur()
	return cmd
}

func selectedDir(rows []ui.GameRow, cursor int) string {
	if cursor < 0 || cursor >= len(rows) {
		return ""
	}
	return rows[cursor].InstallDir
}
