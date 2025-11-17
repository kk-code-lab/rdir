package app

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
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

	if err := statepkg.LoadDirectory(state); err != nil {
		screen.Fini()
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
		currentPath:    cwd,
		clipboardCmd:   clipboardCmd,
		clipboardAvail: clipboardAvail,
		editorCmd:      editorCmd,
	}

	inputHandler.SetState(state)
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

	app.renderer.Render(app.state)
	renderPending := false

	eventChan := make(chan tcell.Event)
	go func() {
		for {
			eventChan <- app.screen.PollEvent()
		}
	}()

	var sigContCh chan os.Signal
	if sigs := contSignals(); len(sigs) > 0 {
		sigContCh = make(chan os.Signal, 1)
		signal.Notify(sigContCh, sigs...)
		defer signal.Stop(sigContCh)
	}

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
		case ev := <-eventChan:
			if app.handleEvent(ev) {
				renderPending = true
			}
		case <-animationCh:
			renderPending = true
		case action := <-app.actionCh:
			if app.handleAction(action) {
				renderPending = true
			}
		case <-sigContCh:
			if app.resumeAfterStop() {
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

// handleMouse maps primary-clicks to selection and navigation.
func (app *Application) handleMouse(ev *tcell.EventMouse) bool {
	if app.state == nil {
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

	// Sidebar click: go up and select sibling.
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

	clickKey := fmt.Sprintf("list-%d", row)
	doubleClick := app.lastClickKey == clickKey && time.Since(app.lastClickTime) <= doubleClickThreshold
	app.lastClickKey = clickKey
	app.lastClickTime = time.Now()

	if app.state.GlobalSearchActive {
		idx := app.state.GlobalSearchScroll + row
		if idx >= 0 && idx < len(app.state.GlobalSearchResults) {
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
	app.actionCh <- statepkg.MouseSelectAction{DisplayIndex: displayIdx}
	if doubleClick {
		app.actionCh <- statepkg.RightArrowAction{}
	}
	return true
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

	clickKey := fmt.Sprintf("sidebar-%d", row)
	app.lastClickKey = clickKey
	app.lastClickTime = time.Now()

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

	// Rebuild path from segments up to idx.
	path := ""
	if len(segments) > 0 && segments[0] == "/" {
		path = "/"
	}
	for i := 0; i <= idx; i++ {
		if segments[i] == "/" {
			continue
		}
		if path == "" || path == "/" {
			path = filepath.Join(path, segments[i])
		} else {
			path = filepath.Join(path, segments[i])
		}
	}
	if path == "" {
		path = "/"
	}

	app.actionCh <- statepkg.GoToPathAction{Path: path}
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
	if app.state == nil || app.state.LastYankTime.IsZero() {
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
		app.shouldQuit = true
		return false
	case statepkg.QuitAndChangeAction:
		app.currentPath = app.state.CurrentPath
		app.shouldQuit = true
		return false
	case statepkg.SuspendAction:
		app.suspendToShell()
		app.resumeAfterStop()
		return true
	}

	return app.handleAppAction(action)
}

func (app *Application) handleAppAction(action statepkg.Action) bool {
	switch action.(type) {
	case statepkg.YankPathAction:
		return app.handleClipboard()
	case statepkg.RightArrowAction:
		return app.handleRightArrow()
	case statepkg.OpenEditorAction:
		return app.handleEditorOpen()
	case statepkg.OpenPagerAction:
		return app.handleOpenPager()
	}

	if _, err := app.reducer.Reduce(app.state, action); err != nil {
		app.state.LastError = err
	}
	return true
}

func (app *Application) runPreviewPager() (err error) {
	view, err := pagerui.NewPreviewPager(app.state)
	if err != nil {
		return err
	}

	if err := app.screen.Suspend(); err != nil {
		return err
	}
	defer func() {
		if resumeErr := app.screen.Resume(); resumeErr != nil && err == nil {
			err = resumeErr
		}
		app.screen.Sync()
	}()

	return view.Run()
}
