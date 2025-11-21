package app

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	statepkg "github.com/kk-code-lab/rdir/internal/state"
	renderui "github.com/kk-code-lab/rdir/internal/ui/render"
)

func TestHandleMouseIgnoresPreviewClicks(t *testing.T) {
	scr := tcell.NewSimulationScreen("")
	if err := scr.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	defer scr.Fini()
	scr.SetSize(160, 40)

	renderer := renderui.NewRenderer(scr)

	state := &statepkg.AppState{
		CurrentPath:     "/tmp",
		Files:           []statepkg.FileEntry{{Name: "a.txt", FullPath: "/tmp/a.txt"}, {Name: "b.txt", FullPath: "/tmp/b.txt"}},
		SelectedIndex:   0,
		HideHiddenFiles: false,
		ScreenWidth:     160,
		ScreenHeight:    40,
		PreviewData: &statepkg.PreviewData{
			Name:      "a.txt",
			TextLines: []string{"hello world"},
			TextLineMeta: []statepkg.TextLineMetadata{
				{DisplayWidth: 11},
			},
		},
	}

	renderer.Render(state)
	layout, ok := renderer.LastLayout()
	if !ok || !layout.ShowPreview {
		t.Fatalf("expected preview to be visible for test layout")
	}

	actionCh := make(chan statepkg.Action, 2)
	app := &Application{
		renderer: renderer,
		state:    state,
		actionCh: actionCh,
	}

	clickY := 2 // inside list area rows
	clickX := layout.PreviewStart + 1
	ev := tcell.NewEventMouse(clickX, clickY, tcell.Button1, tcell.ModNone)
	if !app.handleMouse(ev) {
		t.Fatalf("handleMouse returned false")
	}

	select {
	case act := <-actionCh:
		t.Fatalf("expected no action for preview click, got %T", act)
	default:
	}
}

func TestHandleMouseSelectsFromMainPanelOnly(t *testing.T) {
	scr := tcell.NewSimulationScreen("")
	if err := scr.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	defer scr.Fini()
	scr.SetSize(160, 40)

	renderer := renderui.NewRenderer(scr)

	state := &statepkg.AppState{
		CurrentPath:     "/tmp",
		Files:           []statepkg.FileEntry{{Name: "a.txt", FullPath: "/tmp/a.txt"}, {Name: "b.txt", FullPath: "/tmp/b.txt"}},
		SelectedIndex:   0,
		HideHiddenFiles: false,
		ScreenWidth:     160,
		ScreenHeight:    40,
		PreviewData: &statepkg.PreviewData{
			Name:      "a.txt",
			TextLines: []string{"hello world"},
			TextLineMeta: []statepkg.TextLineMetadata{
				{DisplayWidth: 11},
			},
		},
	}

	renderer.Render(state)
	layout, ok := renderer.LastLayout()
	if !ok {
		t.Fatalf("expected layout after render")
	}

	actionCh := make(chan statepkg.Action, 2)
	app := &Application{
		renderer: renderer,
		state:    state,
		actionCh: actionCh,
	}

	clickY := 2
	clickX := layout.MainPanelStart + 1
	ev := tcell.NewEventMouse(clickX, clickY, tcell.Button1, tcell.ModNone)
	if !app.handleMouse(ev) {
		t.Fatalf("handleMouse returned false")
	}

	select {
	case act := <-actionCh:
		if _, ok := act.(statepkg.MouseSelectAction); !ok {
			t.Fatalf("expected MouseSelectAction from main panel click, got %T", act)
		}
	default:
		t.Fatalf("expected selection action from main panel click")
	}
}

func TestHandleMouseIgnoresDragWhileHeld(t *testing.T) {
	scr := tcell.NewSimulationScreen("")
	if err := scr.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	defer scr.Fini()
	scr.SetSize(160, 40)

	renderer := renderui.NewRenderer(scr)

	state := &statepkg.AppState{
		CurrentPath:     "/tmp",
		Files:           []statepkg.FileEntry{{Name: "a.txt", FullPath: "/tmp/a.txt"}, {Name: "b.txt", FullPath: "/tmp/b.txt"}},
		SelectedIndex:   0,
		HideHiddenFiles: false,
		ScreenWidth:     160,
		ScreenHeight:    40,
	}

	renderer.Render(state)
	layout, ok := renderer.LastLayout()
	if !ok {
		t.Fatalf("expected layout after render")
	}

	actionCh := make(chan statepkg.Action, 4)
	app := &Application{
		renderer: renderer,
		state:    state,
		actionCh: actionCh,
	}

	// First press selects the row.
	clickX := layout.MainPanelStart + 1
	firstY := 1 // first list row underneath header
	ev := tcell.NewEventMouse(clickX, firstY, tcell.Button1, tcell.ModNone)
	if !app.handleMouse(ev) {
		t.Fatalf("handleMouse returned false")
	}

	select {
	case act := <-actionCh:
		if _, ok := act.(statepkg.MouseSelectAction); !ok {
			t.Fatalf("expected MouseSelectAction from first click, got %T", act)
		}
	default:
		t.Fatalf("expected selection action from first click")
	}

	// Drag while holding the button should be ignored.
	dragY := firstY + 1
	dragEv := tcell.NewEventMouse(clickX, dragY, tcell.Button1, tcell.ModNone)
	if !app.handleMouse(dragEv) {
		t.Fatalf("handleMouse returned false during drag")
	}
	select {
	case act := <-actionCh:
		t.Fatalf("expected no action during drag, got %T", act)
	default:
	}

	// Release the button to reset edge detection.
	releaseEv := tcell.NewEventMouse(clickX, dragY, tcell.ButtonNone, tcell.ModNone)
	_ = app.handleMouse(releaseEv)

	// Next independent click should be processed.
	secondEv := tcell.NewEventMouse(clickX, dragY, tcell.Button1, tcell.ModNone)
	if !app.handleMouse(secondEv) {
		t.Fatalf("handleMouse returned false on second click")
	}
	select {
	case act := <-actionCh:
		if _, ok := act.(statepkg.MouseSelectAction); !ok {
			t.Fatalf("expected MouseSelectAction from second click, got %T", act)
		}
	default:
		t.Fatalf("expected selection action from second click")
	}
}
