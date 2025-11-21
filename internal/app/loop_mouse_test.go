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
