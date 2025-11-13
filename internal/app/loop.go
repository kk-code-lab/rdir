package app

import (
	"time"

	"github.com/gdamore/tcell/v2"
	statepkg "github.com/kk-code-lab/rdir/internal/state"
	"github.com/kk-code-lab/rdir/internal/ui/input"
	pagerui "github.com/kk-code-lab/rdir/internal/ui/pager"
	renderui "github.com/kk-code-lab/rdir/internal/ui/render"
)

func NewApplication() (*Application, error) {
	screen, err := tcell.NewScreen()
	if err != nil {
		return nil, err
	}
	if err := screen.Init(); err != nil {
		return nil, err
	}

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
	default:
		return false
	}
	return true
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
