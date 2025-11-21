package app

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/gdamore/tcell/v2"
	statepkg "github.com/kk-code-lab/rdir/internal/state"
	"github.com/kk-code-lab/rdir/internal/ui/input"
	pagerui "github.com/kk-code-lab/rdir/internal/ui/pager"
	renderui "github.com/kk-code-lab/rdir/internal/ui/render"
	"github.com/mattn/go-runewidth"
)

const doubleClickThreshold = 300 * time.Millisecond

func NewApplication() (*Application, error) {
	if runtime.GOOS == "windows" {
		_ = os.Setenv("TCELL_ALTSCREEN", "disable")
	}
	screen, err := tcell.NewScreen()
	if err != nil {
		return nil, err
	}
	if err := screen.Init(); err != nil {
		return nil, err
	}
	// Parse mouse sequences so modified clicks don't leak as key events.
	screen.EnableMouse()

	cwd, err := GetCwd()
	if err != nil {
		screen.Fini()
		return nil, err
	}

	clipboardCmd, clipboardAvail := detectClipboard()
	editorCmd, editorAvail := detectEditorCommand()

	state := newInitialState(cwd, clipboardAvail, editorAvail)
	state.DirectoryLoader = statepkg.NewAsyncDirectoryLoader()
	state.PreviewLoader = statepkg.NewAsyncPreviewLoader()
	w, h := screen.Size()
	state.ScreenWidth = w
	state.ScreenHeight = h

	actionCh := make(chan statepkg.Action, 10)
	state.SetDispatch(func(action statepkg.Action) {
		select {
		case actionCh <- action:
		default:
			go func() { actionCh <- action }()
		}
	})

	reducer := statepkg.NewStateReducer()
	renderer := renderui.NewRenderer(screen)
	inputHandler := input.NewInputHandler(actionCh)

	var debugFile *os.File
	var debugLogger *log.Logger
	commit := BuildCommit
	if commit == "" {
		commit = "unknown"
	}
	if debugLoggingEnabled {
		path := filepath.Join(os.TempDir(), "rdir_debug.log")
		if f, err := os.Create(path); err == nil {
			debugFile = f
			debugLogger = log.New(f, "", 0)
			log.SetOutput(f)
			log.SetFlags(0)
		} else {
			// Fallback to stderr if temp file unavailable.
			debugLogger = log.New(os.Stderr, "", 0)
		}
	}

	if err := statepkg.LoadDirectory(state); err != nil {
		screen.Fini()
		if debugFile != nil {
			_ = debugFile.Close()
		}
		return nil, err
	}
	state.RefreshParentEntries()

	app := &Application{
		screen:         screen,
		state:          state,
		reducer:        reducer,
		renderer:       renderer,
		input:          inputHandler,
		actionCh:       actionCh,
		debugLog:       debugLogger,
		debugLogFile:   debugFile,
		currentPath:    cwd,
		clipboardCmd:   clipboardCmd,
		clipboardAvail: clipboardAvail,
		editorCmd:      editorCmd,
	}

	inputHandler.SetState(state)

	if debugLogger != nil {
		ts := time.Now().Format("2006-01-02 15:04:05.000000 -0700 MST")
		debugLogger.Printf("%s session start pid=%d goos=%s goarch=%s cwd=%s commit=%s", ts, os.Getpid(), runtime.GOOS, runtime.GOARCH, cwd, commit)
	}
	_ = reducer.GeneratePreview(state)
	return app, nil
}

func newInitialState(cwd string, clipboardAvail, editorAvail bool) *statepkg.AppState {
	return &statepkg.AppState{
		CurrentPath:        cwd,
		Files:              []statepkg.FileEntry{},
		History:            []string{cwd},
		HistoryIndex:       0,
		SelectedIndex:      0,
		ScrollOffset:       0,
		FilterActive:       false,
		FilterQuery:        "",
		PreviewData:        nil,
		ClipboardAvailable: clipboardAvail,
		EditorAvailable:    editorAvail,
		HideHiddenFiles:    true,
	}
}

func (app *Application) Run() {
	defer app.screen.Fini()
	defer app.logf("session end")

	app.renderer.Render(app.state)
	renderPending := false

	app.startEventPoller()
	defer app.stopEventPoller()

	const animationInterval = 50 * time.Millisecond
	var animationTimer *time.Timer
	var animationCh <-chan time.Time

	startAnimation := func() {
		if animationTimer == nil {
			animationTimer = time.NewTimer(animationInterval)
		} else {
			if !animationTimer.Stop() {
				select {
				case <-animationTimer.C:
				default:
				}
			}
			animationTimer.Reset(animationInterval)
		}
		animationCh = animationTimer.C
	}

	stopAnimation := func() {
		if animationTimer == nil {
			return
		}
		if !animationTimer.Stop() {
			select {
			case <-animationTimer.C:
			default:
			}
		}
		animationCh = nil
	}

	for !app.shouldQuit {
		if renderPending {
			app.renderer.Render(app.state)
			renderPending = false
		}

		if app.shouldAnimate() {
			startAnimation()
		} else {
			stopAnimation()
		}

		select {
		case ev, ok := <-app.eventChan:
			if !ok {
				app.logf("event channel closed; restarting poller")
				app.startEventPoller()
				continue
			}
			app.logf("recv event: %s", formatTcellEvent(ev))
			if ev != nil && app.handleEvent(ev) {
				renderPending = true
			}
		case <-animationCh:
			renderPending = true
		case action := <-app.actionCh:
			app.logf("action: %T", action)
			if app.handleAction(action) {
				renderPending = true
			}
		}

		if app.processActions() {
			renderPending = true
		}
	}

	stopAnimation()
}

func (app *Application) handleEvent(ev tcell.Event) bool {
	switch ev := ev.(type) {
	case *tcell.EventKey:
		if !app.input.ProcessEvent(ev) {
			app.shouldQuit = true
		}
	case *tcell.EventResize:
		if !app.input.ProcessEvent(ev) {
			app.shouldQuit = true
		}
	case *tcell.EventMouse:
		if !app.handleMouse(ev) {
			app.shouldQuit = true
		}
		return true
	case *tcell.EventInterrupt:
		return true
	default:
		return false
	}
	return true
}

// reinitScreen rebuilds the tcell screen and renderer after returning from
// the external pager/editor to avoid Windows console quirks.
func (app *Application) reinitScreen() error {
	if _, sim := app.screen.(tcell.SimulationScreen); sim {
		// Tests use SimulationScreen; rebuilding it confuses its internals (double Fini).
		return nil
	}
	app.logf("reinitScreen: start")
	app.stopEventPoller()
	app.drainPendingEvents()
	_ = flushConsoleInput()

	if app.screen != nil {
		app.screen.Fini()
	}

	if runtime.GOOS == "windows" {
		_ = os.Setenv("TCELL_ALTSCREEN", "disable")
	}

	scr, err := tcell.NewScreen()
	if err != nil {
		app.logf("reinitScreen: NewScreen err=%v", err)
		return err
	}
	if err := scr.Init(); err != nil {
		app.logf("reinitScreen: Init err=%v", err)
		return err
	}
	scr.EnableMouse()

	app.screen = scr
	app.renderer = renderui.NewRenderer(scr)
	app.input = input.NewInputHandler(app.actionCh)
	app.input.SetState(app.state)

	w, h := scr.Size()
	app.state.ScreenWidth = w
	app.state.ScreenHeight = h

	app.renderer.Render(app.state)
	app.screen.Show()
	app.startEventPoller()
	if err := app.screen.PostEvent(tcell.NewEventResize(w, h)); err != nil {
		app.logf("reinitScreen: PostEvent resize err=%v", err)
	}
	if err := app.screen.PostEvent(tcell.NewEventInterrupt(nil)); err != nil {
		app.logf("reinitScreen: PostEvent interrupt err=%v", err)
	}
	app.logf("reinitScreen: done %dx%d", w, h)
	return nil
}

// handleMouse maps primary-clicks to selection and navigation.
func (app *Application) handleMouse(ev *tcell.EventMouse) bool {
	if app.state == nil {
		return true
	}
	// Mouse wheel scrolls the main list regardless of button 1.
	if ev.Buttons()&tcell.WheelUp != 0 {
		if app.state != nil && app.state.GlobalSearchActive {
			app.actionCh <- statepkg.GlobalSearchNavigateAction{Direction: "up"}
		} else {
			app.actionCh <- statepkg.ScrollUpAction{}
		}
		return true
	}
	if ev.Buttons()&tcell.WheelDown != 0 {
		if app.state != nil && app.state.GlobalSearchActive {
			app.actionCh <- statepkg.GlobalSearchNavigateAction{Direction: "down"}
		} else {
			app.actionCh <- statepkg.ScrollDownAction{}
		}
		return true
	}

	if ev.Buttons()&tcell.Button1 == 0 {
		return true
	}
	if app.state.PreviewFullScreen {
		return true
	}

	x, y := ev.Position()

	// Breadcrumb (top row)
	if y == 0 {
		if app.handleBreadcrumbClick(x) {
			return true
		}
	}

	sidebarWidth := renderui.SidebarWidthForWidth(app.state.ScreenWidth, app.state)

	// Sidebar click: jump to parent directory (any row).
	if sidebarWidth > 0 && x < sidebarWidth {
		if app.handleSidebarClick(y) {
			return true
		}
		// otherwise fall through to list handling
	}

	listStartY := 1
	if app.state.FilterActive || app.state.GlobalSearchActive {
		listStartY = 2
	}
	bottomLimit := app.state.ScreenHeight - 2 // leave room for status line
	if y < listStartY || y >= bottomLimit {
		return true
	}
	row := y - listStartY
	if row < 0 {
		return true
	}

	if app.state.GlobalSearchActive {
		idx := app.state.GlobalSearchScroll + row
		if idx >= 0 && idx < len(app.state.GlobalSearchResults) {
			result := app.state.GlobalSearchResults[idx]
			doubleClick := app.registerClick(fmt.Sprintf("gs-%s", result.FilePath))
			app.actionCh <- statepkg.GlobalSearchSelectIndexAction{Index: idx}
			if doubleClick {
				app.actionCh <- statepkg.GlobalSearchOpenAction{}
			}
		}
		return true
	}

	displayIdx := app.state.ScrollOffset + row
	displayFiles := app.state.DisplayFiles()
	if displayIdx < 0 || displayIdx >= len(displayFiles) {
		return true
	}
	doubleClick := app.registerClick(displayFiles[displayIdx].FullPath)
	app.actionCh <- statepkg.MouseSelectAction{DisplayIndex: displayIdx}
	if doubleClick {
		app.actionCh <- statepkg.RightArrowAction{}
	}
	return true
}

// registerClick remembers the last click key and reports whether it is a double-click.
func (app *Application) registerClick(key string) bool {
	doubleClick := app.lastClickKey == key && time.Since(app.lastClickTime) <= doubleClickThreshold
	app.lastClickKey = key
	app.lastClickTime = time.Now()
	return doubleClick
}

func (app *Application) handleSidebarClick(y int) bool {
	entries := app.state.ParentEntries
	if len(entries) == 0 {
		return false
	}

	parentPath := filepath.Dir(app.state.CurrentPath)
	hasParent := parentPath != "" && parentPath != app.state.CurrentPath
	if !hasParent {
		return false
	}

	h := app.state.ScreenHeight
	maxRows := h - 2
	if maxRows < 1 {
		maxRows = 1
	}

	currentName := filepath.Base(app.state.CurrentPath)
	currentIdx := 0
	foundCurrent := false
	for idx, entry := range entries {
		if entry.Name == currentName {
			currentIdx = idx
			foundCurrent = true
			break
		}
	}
	if !foundCurrent {
		currentIdx = 0
	}

	startIdx := 0
	if len(entries) > maxRows {
		startIdx = currentIdx - maxRows/2
		if startIdx < 0 {
			startIdx = 0
		}
		if startIdx > len(entries)-maxRows {
			startIdx = len(entries) - maxRows
		}
	}
	endIdx := len(entries)
	if endIdx-startIdx > maxRows {
		endIdx = startIdx + maxRows
	}

	row := y - 1 // sidebar starts at y=1
	if row < 0 || row >= endIdx-startIdx {
		return false
	}

	_ = app.registerClick(fmt.Sprintf("sidebar-%d", row))

	app.actionCh <- statepkg.GoUpAction{}
	return true
}

func (app *Application) handleBreadcrumbClick(x int) bool {
	if x < 0 || app.state == nil {
		return false
	}
	headerText := "rdir"
	pos := runewidth.StringWidth(headerText)
	if x < pos {
		return false
	}
	if pos < app.state.ScreenWidth {
		pos++ // space after header
	}

	available := app.state.ScreenWidth - pos
	segments := renderui.FormatBreadcrumbSegments(app.state.CurrentPath)
	if len(segments) == 0 {
		return false
	}

	// Build full breadcrumb text and widths; if it doesn't fit, ignore clicks to avoid mismap.
	totalWidth := 0
	for i, s := range segments {
		if i > 0 {
			totalWidth += runewidth.StringWidth(" › ")
		}
		totalWidth += runewidth.StringWidth(s)
	}
	if totalWidth > available {
		return false
	}

	currentX := pos
	for i, s := range segments {
		if i > 0 {
			sepW := runewidth.StringWidth(" › ")
			if x >= currentX && x < currentX+sepW {
				// click on separator -> treat as previous segment
				if i > 0 {
					app.jumpToBreadcrumb(segments, i-1)
				}
				return true
			}
			currentX += sepW
		}

		segW := runewidth.StringWidth(s)
		if x >= currentX && x < currentX+segW {
			app.jumpToBreadcrumb(segments, i)
			return true
		}
		currentX += segW
	}
	return false
}

func (app *Application) jumpToBreadcrumb(segments []string, idx int) {
	if idx < 0 || idx >= len(segments) {
		return
	}

	path := buildBreadcrumbPath(segments, idx)
	if path == "" {
		return
	}

	app.actionCh <- statepkg.GoToPathAction{Path: path}
}

// buildBreadcrumbPath assembles an absolute path from breadcrumb segments.
// It preserves Windows drive roots ("C:") so clicks stay absolute there.
func buildBreadcrumbPath(segments []string, idx int) string {
	if idx < 0 || idx >= len(segments) {
		return ""
	}

	sep := string(filepath.Separator)
	first := segments[0]
	isDrive := len(first) == 2 && (first[1] == ':') && ((first[0] >= 'A' && first[0] <= 'Z') || (first[0] >= 'a' && first[0] <= 'z'))

	// Handle Windows drive letters like "C:".
	if isDrive {
		if idx == 0 {
			return first + sep
		}

		path := first + sep
		for i := 1; i <= idx && i < len(segments); i++ {
			path = filepath.Join(path, segments[i])
		}
		return path
	}

	// Unix-style absolute path starting with root.
	if first == "/" || first == sep {
		path := sep
		for i := 1; i <= idx && i < len(segments); i++ {
			if segments[i] == "/" {
				continue
			}
			path = filepath.Join(path, segments[i])
		}
		return path
	}

	// Fallback: relative path construction.
	path := first
	for i := 1; i <= idx && i < len(segments); i++ {
		path = filepath.Join(path, segments[i])
	}
	if path == "" {
		return sep
	}
	return path
}

func (app *Application) processActions() bool {
	changed := false
	for {
		select {
		case action := <-app.actionCh:
			if app.handleAction(action) {
				changed = true
			}
		default:
			return changed
		}
	}
}

func (app *Application) shouldAnimate() bool {
	if app.state == nil {
		return false
	}
	if app.state.PreviewLoading {
		return true
	}
	if app.state.LastYankTime.IsZero() {
		return false
	}
	return time.Since(app.state.LastYankTime) < 100*time.Millisecond
}

func (app *Application) handleAction(action statepkg.Action) bool {
	if action == nil {
		return false
	}

	switch action.(type) {
	case statepkg.QuitAction:
		app.logf("handleAction QuitAction")
		app.shouldQuit = true
		return false
	case statepkg.QuitAndChangeAction:
		app.logf("handleAction QuitAndChangeAction")
		app.currentPath = app.state.CurrentPath
		app.shouldQuit = true
		return false
	}

	return app.handleAppAction(action)
}

func (app *Application) handleAppAction(action statepkg.Action) bool {
	switch action.(type) {
	case statepkg.YankPathAction:
		app.logf("handleAppAction YankPathAction")
		return app.handleClipboard()
	case statepkg.RightArrowAction:
		app.logf("handleAppAction RightArrowAction")
		return app.handleRightArrow()
	case statepkg.OpenEditorAction:
		app.logf("handleAppAction OpenEditorAction")
		return app.handleEditorOpen()
	case statepkg.OpenPagerAction:
		app.logf("handleAppAction OpenPagerAction")
		_ = app.reducer.EnsurePreviewCurrent(app.state)
		return app.handleOpenPager()
	}

	if _, err := app.reducer.Reduce(app.state, action); err != nil {
		app.state.LastError = err
	}
	return true
}

func (app *Application) runPreviewPager() (err error) {
	view, err := pagerui.NewPreviewPager(app.state, app.editorCmd, app.reducer, app.clipboardCmd)
	if err != nil {
		return err
	}

	app.stopEventPoller()
	app.logf("runPreviewPager: suspending screen")
	if err := app.screen.Suspend(); err != nil {
		app.startEventPoller()
		return err
	}
	defer func() {
		if resumeErr := app.screen.Resume(); resumeErr != nil && err == nil {
			err = resumeErr
		}
		app.logf("runPreviewPager: resumed screen")
		app.drainPendingEvents()
		_ = flushConsoleInput()
		if errReinit := app.reinitScreen(); errReinit != nil && err == nil {
			err = errReinit
		}
		if app.processActions() {
			app.renderer.Render(app.state)
			app.screen.Show()
			app.logf("runPreviewPager: rendered pending actions after reinit")
		}
	}()

	return view.Run()
}
