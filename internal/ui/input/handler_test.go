package input

import (
	"fmt"
	"testing"

	"github.com/gdamore/tcell/v2"
	statepkg "github.com/kk-code-lab/rdir/internal/state"
)

func TestInputHandlerEscapeClearsQueryButKeepsGlobalSearchActive(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{
		GlobalSearchActive: true,
		GlobalSearchQuery:  "foo",
	}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyEscape, 0, 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		if _, ok := action.(statepkg.GlobalSearchResetQueryAction); !ok {
			t.Fatalf("Expected statepkg.GlobalSearchResetQueryAction, got %T", action)
		}
	default:
		t.Fatal("Expected action to be emitted for escape key")
	}
}

func TestQuestionMarkTogglesHelpInNormalMode(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyRune, '?', 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		if _, ok := action.(statepkg.HelpToggleAction); !ok {
			t.Fatalf("Expected HelpToggleAction, got %T", action)
		}
	default:
		t.Fatal("Expected HelpToggleAction to be emitted for '?'")
	}
}

func TestEscapeHidesHelpBeforeOtherModes(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{
		HelpVisible:  true,
		FilterActive: true,
	}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyEscape, 0, 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		if _, ok := action.(statepkg.HelpHideAction); !ok {
			t.Fatalf("Expected HelpHideAction, got %T", action)
		}
	default:
		t.Fatal("Expected HelpHideAction to be emitted when help is visible")
	}
}

func TestQClosesHelpWithoutQuitting(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{HelpVisible: true}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyRune, 'q', 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		if _, ok := action.(statepkg.HelpHideAction); !ok {
			t.Fatalf("Expected HelpHideAction, got %T", action)
		}
	default:
		t.Fatal("Expected HelpHideAction when pressing q with help visible")
	}
}

func TestInputHandlerEscapeExitsGlobalSearchWhenQueryEmpty(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{
		GlobalSearchActive: true,
		GlobalSearchQuery:  "",
	}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyEscape, 0, 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		if _, ok := action.(statepkg.GlobalSearchClearAction); !ok {
			t.Fatalf("Expected statepkg.GlobalSearchClearAction, got %T", action)
		}
	default:
		t.Fatal("Expected action to be emitted for escape key")
	}
}

func TestInputHandlerEscapeExitsPreviewFullScreenBeforeFilter(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{
		PreviewFullScreen: true,
		PreviewData:       &statepkg.PreviewData{},
		FilterActive:      true,
	}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyEscape, 0, 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		if _, ok := action.(statepkg.PreviewExitFullScreenAction); !ok {
			t.Fatalf("Expected PreviewExitFullScreenAction, got %T", action)
		}
	default:
		t.Fatal("Expected PreviewExitFullScreenAction")
	}
}

func TestInputHandlerRightArrowMovesCursorInGlobalSearch(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{
		GlobalSearchActive: true,
		GlobalSearchQuery:  "foo",
	}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyRight, 0, 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		move, ok := action.(statepkg.GlobalSearchMoveCursorAction)
		if !ok {
			t.Fatalf("Expected statepkg.GlobalSearchMoveCursorAction, got %T", action)
		}
		if move.Direction != "right" {
			t.Fatalf("Expected direction 'right', got %q", move.Direction)
		}
	default:
		t.Fatal("Expected action to be emitted for right arrow")
	}
}

func TestInputHandlerRightArrowKeepsFilterActiveForFilePreview(t *testing.T) {
	actionChan := make(chan statepkg.Action, 2)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{
		FilterActive:    true,
		Files:           []statepkg.FileEntry{{Name: "file.txt", IsDir: false}},
		FilteredIndices: []int{0},
		SelectedIndex:   0,
	}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyRight, 0, 0)
	handler.ProcessEvent(event)

	firstAction := <-actionChan
	if _, ok := firstAction.(statepkg.RightArrowAction); !ok {
		t.Fatalf("Expected statepkg.RightArrowAction, got %T", firstAction)
	}

	select {
	case extra := <-actionChan:
		t.Fatalf("Did not expect extra action, got %T", extra)
	default:
	}
}

func TestInputHandlerRightArrowClearsFilterForDirectories(t *testing.T) {
	actionChan := make(chan statepkg.Action, 2)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{
		FilterActive:    true,
		Files:           []statepkg.FileEntry{{Name: "dir", IsDir: true}},
		FilteredIndices: []int{0},
		SelectedIndex:   0,
	}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyRight, 0, 0)
	handler.ProcessEvent(event)

	first := <-actionChan
	if _, ok := first.(statepkg.FilterClearAction); !ok {
		t.Fatalf("Expected statepkg.FilterClearAction first, got %T", first)
	}

	second := <-actionChan
	if _, ok := second.(statepkg.RightArrowAction); !ok {
		t.Fatalf("Expected statepkg.RightArrowAction second, got %T", second)
	}
}

func TestInputHandlerRightArrowIgnoredWhenPreviewFullScreen(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{
		PreviewFullScreen: true,
		PreviewData:       &statepkg.PreviewData{},
	}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyRight, 0, 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		t.Fatalf("Did not expect action, got %T", action)
	default:
	}
}

func TestInputHandlerEnterInFilterModeOnlyClearsFilter(t *testing.T) {
	actionChan := make(chan statepkg.Action, 2)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{
		FilterActive: true,
	}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyEnter, 0, 0)
	handler.ProcessEvent(event)

	first := <-actionChan
	if _, ok := first.(statepkg.FilterClearAction); !ok {
		t.Fatalf("Expected FilterClearAction, got %T", first)
	}

	select {
	case extra := <-actionChan:
		t.Fatalf("Did not expect second action, got %T", extra)
	default:
	}
}

func TestInputHandlerEnterOutsideFilterBehavesLikeRightArrow(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{
		FilterActive: false,
	}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyEnter, 0, 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		if _, ok := action.(statepkg.RightArrowAction); !ok {
			t.Fatalf("Expected RightArrowAction, got %T", action)
		}
	default:
		t.Fatal("Expected RightArrowAction")
	}
}

func TestInputHandlerUpArrowScrollsPreviewWhenFullScreen(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{
		PreviewFullScreen: true,
		PreviewData:       &statepkg.PreviewData{},
	}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyUp, 0, 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		if _, ok := action.(statepkg.PreviewScrollUpAction); !ok {
			t.Fatalf("Expected PreviewScrollUpAction, got %T", action)
		}
	default:
		t.Fatal("Expected PreviewScrollUpAction")
	}
}

func TestInputHandlerDotIgnoredInFullScreen(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{
		PreviewFullScreen: true,
	}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyRune, '.', 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		t.Fatalf("Did not expect action for '.', got %T", action)
	default:
	}
}

func TestInputHandlerDotTogglesHiddenNormally(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyRune, '.', 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		if _, ok := action.(statepkg.ToggleHiddenFilesAction); !ok {
			t.Fatalf("Expected ToggleHiddenFilesAction, got %T", action)
		}
	default:
		t.Fatal("Expected ToggleHiddenFilesAction for '.'")
	}
}

func TestInputHandlerLeftArrowMovesCursorInGlobalSearch(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{
		GlobalSearchActive: true,
		GlobalSearchQuery:  "foo",
	}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyLeft, 0, 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		move, ok := action.(statepkg.GlobalSearchMoveCursorAction)
		if !ok {
			t.Fatalf("Expected statepkg.GlobalSearchMoveCursorAction, got %T", action)
		}
		if move.Direction != "left" {
			t.Fatalf("Expected direction 'left', got %q", move.Direction)
		}
	default:
		t.Fatal("Expected action to be emitted for left arrow")
	}
}

func TestInputHandlerQExitsPreviewFullScreen(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{
		PreviewFullScreen: true,
		PreviewData:       &statepkg.PreviewData{},
	}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyRune, 'q', 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		if _, ok := action.(statepkg.PreviewExitFullScreenAction); !ok {
			t.Fatalf("Expected PreviewExitFullScreenAction, got %T", action)
		}
	default:
		t.Fatal("Expected PreviewExitFullScreenAction for 'q'")
	}
}

func TestInputHandlerXExitsPreviewFullScreen(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{
		PreviewFullScreen: true,
		PreviewData:       &statepkg.PreviewData{},
	}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyRune, 'x', 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		if _, ok := action.(statepkg.PreviewExitFullScreenAction); !ok {
			t.Fatalf("Expected PreviewExitFullScreenAction, got %T", action)
		}
	default:
		t.Fatal("Expected PreviewExitFullScreenAction for 'x'")
	}
}

func TestInputHandlerLeftArrowExitsPreviewFullScreen(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{
		PreviewFullScreen: true,
		PreviewData:       &statepkg.PreviewData{},
	}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyLeft, 0, 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		if _, ok := action.(statepkg.PreviewExitFullScreenAction); !ok {
			t.Fatalf("Expected PreviewExitFullScreenAction, got %T", action)
		}
	default:
		t.Fatal("Expected PreviewExitFullScreenAction")
	}
}

func TestInputHandlerWrapToggleInFullScreen(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{
		PreviewFullScreen: true,
		PreviewData:       &statepkg.PreviewData{},
	}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyRune, 'w', 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		if _, ok := action.(statepkg.TogglePreviewWrapAction); !ok {
			t.Fatalf("Expected TogglePreviewWrapAction, got %T", action)
		}
	default:
		t.Fatal("Expected TogglePreviewWrapAction for 'w'")
	}
}

func TestInputHandlerRunePOpensPager(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyRune, 'P', 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		if _, ok := action.(statepkg.OpenPagerAction); !ok {
			t.Fatalf("Expected OpenPagerAction, got %T", action)
		}
	default:
		t.Fatal("Expected OpenPagerAction for 'P'")
	}
}

func TestInputHandlerLeftArrowResetsFilterQuery(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{
		FilterActive: true,
		FilterQuery:  "abc",
	}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyLeft, 0, 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		if _, ok := action.(statepkg.FilterResetQueryAction); !ok {
			t.Fatalf("Expected FilterResetQueryAction, got %T", action)
		}
	default:
		t.Fatal("Expected action for left arrow in filter mode")
	}
}

func TestInputHandlerLeftArrowClearsEmptyFilter(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{
		FilterActive: true,
		FilterQuery:  "",
	}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyLeft, 0, 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		if _, ok := action.(statepkg.FilterClearAction); !ok {
			t.Fatalf("Expected FilterClearAction, got %T", action)
		}
	default:
		t.Fatal("Expected action for left arrow in filter mode")
	}
}

func TestInputHandlerDeleteInGlobalSearch(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{
		GlobalSearchActive: true,
		GlobalSearchQuery:  "foo",
	}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyDelete, 0, 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		if _, ok := action.(statepkg.GlobalSearchDeleteAction); !ok {
			t.Fatalf("Expected statepkg.GlobalSearchDeleteAction, got %T", action)
		}
	default:
		t.Fatal("Expected action to be emitted for delete key")
	}
}

func TestInputHandlerBackspaceInGlobalSearch(t *testing.T) {
	keys := []tcell.Key{tcell.KeyBackspace, tcell.KeyBackspace2}

	for _, key := range keys {
		t.Run(fmt.Sprintf("key_%d", key), func(t *testing.T) {
			actionChan := make(chan statepkg.Action, 1)
			handler := NewInputHandler(actionChan)

			state := &statepkg.AppState{
				GlobalSearchActive: true,
				GlobalSearchQuery:  "foo",
			}
			handler.SetState(state)

			event := tcell.NewEventKey(key, 0, 0)
			handler.ProcessEvent(event)

			select {
			case action := <-actionChan:
				if _, ok := action.(statepkg.GlobalSearchBackspaceAction); !ok {
					t.Fatalf("Expected statepkg.GlobalSearchBackspaceAction, got %T", action)
				}
			default:
				t.Fatal("Expected action to be emitted for backspace key")
			}
		})
	}
}

func TestInputHandlerHomeMovesCursorInGlobalSearch(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{
		GlobalSearchActive: true,
		GlobalSearchQuery:  "foo",
	}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyHome, 0, 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		move, ok := action.(statepkg.GlobalSearchMoveCursorAction)
		if !ok {
			t.Fatalf("Expected statepkg.GlobalSearchMoveCursorAction, got %T", action)
		}
		if move.Direction != "home" {
			t.Fatalf("Expected direction 'home', got %q", move.Direction)
		}
	default:
		t.Fatal("Expected action to be emitted for home key")
	}
}

func TestInputHandlerEndMovesCursorInGlobalSearch(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{
		GlobalSearchActive: true,
		GlobalSearchQuery:  "foo",
	}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyEnd, 0, 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		move, ok := action.(statepkg.GlobalSearchMoveCursorAction)
		if !ok {
			t.Fatalf("Expected statepkg.GlobalSearchMoveCursorAction, got %T", action)
		}
		if move.Direction != "end" {
			t.Fatalf("Expected direction 'end', got %q", move.Direction)
		}
	default:
		t.Fatal("Expected action to be emitted for end key")
	}
}

func TestInputHandlerHomeScrollsListInNormalMode(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)
	handler.SetState(&statepkg.AppState{})

	event := tcell.NewEventKey(tcell.KeyHome, 0, 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		if _, ok := action.(statepkg.ScrollToStartAction); !ok {
			t.Fatalf("Expected ScrollToStartAction, got %T", action)
		}
	default:
		t.Fatal("Expected ScrollToStartAction for Home key in normal mode")
	}
}

func TestInputHandlerEndScrollsListInNormalMode(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)
	handler.SetState(&statepkg.AppState{})

	event := tcell.NewEventKey(tcell.KeyEnd, 0, 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		if _, ok := action.(statepkg.ScrollToEndAction); !ok {
			t.Fatalf("Expected ScrollToEndAction, got %T", action)
		}
	default:
		t.Fatal("Expected ScrollToEndAction for End key in normal mode")
	}
}

func TestInputHandlerCtrlArrowsMoveByWordInGlobalSearch(t *testing.T) {
	tests := []struct {
		key       tcell.Key
		direction string
	}{
		{tcell.KeyLeft, "word-left"},
		{tcell.KeyRight, "word-right"},
	}

	for _, tt := range tests {
		t.Run(tt.direction, func(t *testing.T) {
			actionChan := make(chan statepkg.Action, 1)
			handler := NewInputHandler(actionChan)

			state := &statepkg.AppState{
				GlobalSearchActive: true,
				GlobalSearchQuery:  "foo bar",
			}
			handler.SetState(state)

			event := tcell.NewEventKey(tt.key, 0, tcell.ModCtrl)
			handler.ProcessEvent(event)

			select {
			case action := <-actionChan:
				move, ok := action.(statepkg.GlobalSearchMoveCursorAction)
				if !ok {
					t.Fatalf("Expected statepkg.GlobalSearchMoveCursorAction, got %T", action)
				}
				if move.Direction != tt.direction {
					t.Fatalf("Expected direction %q, got %q", tt.direction, move.Direction)
				}
			default:
				t.Fatal("Expected action to be emitted for ctrl arrow key")
			}
		})
	}
}

func TestInputHandlerCtrlShortcutsInGlobalSearch(t *testing.T) {
	tests := []struct {
		name   string
		key    tcell.Key
		expect statepkg.Action
	}{
		{"ctrl-a", tcell.KeyCtrlA, statepkg.GlobalSearchMoveCursorAction{Direction: "home"}},
		{"ctrl-e", tcell.KeyCtrlE, statepkg.GlobalSearchMoveCursorAction{Direction: "end"}},
		{"ctrl-w", tcell.KeyCtrlW, statepkg.GlobalSearchDeleteWordAction{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actionChan := make(chan statepkg.Action, 1)
			handler := NewInputHandler(actionChan)

			state := &statepkg.AppState{
				GlobalSearchActive: true,
				GlobalSearchQuery:  "foo bar",
			}
			handler.SetState(state)

			event := tcell.NewEventKey(tc.key, 0, tcell.ModCtrl)
			handler.ProcessEvent(event)

			select {
			case action := <-actionChan:
				switch expected := tc.expect.(type) {
				case statepkg.GlobalSearchMoveCursorAction:
					move, ok := action.(statepkg.GlobalSearchMoveCursorAction)
					if !ok {
						t.Fatalf("Expected statepkg.GlobalSearchMoveCursorAction, got %T", action)
					}
					if move.Direction != expected.Direction {
						t.Fatalf("Expected direction %q, got %q", expected.Direction, move.Direction)
					}
				case statepkg.GlobalSearchDeleteWordAction:
					if _, ok := action.(statepkg.GlobalSearchDeleteWordAction); !ok {
						t.Fatalf("Expected statepkg.GlobalSearchDeleteWordAction, got %T", action)
					}
				default:
					t.Fatalf("Unexpected expected action type %T", tc.expect)
				}
			default:
				t.Fatal("Expected action to be emitted for ctrl shortcut")
			}
		})
	}
}

func TestInputHandlerTildeTriggersGoHome(t *testing.T) {
	actionChan := make(chan statepkg.Action, 1)
	handler := NewInputHandler(actionChan)

	state := &statepkg.AppState{}
	handler.SetState(state)

	event := tcell.NewEventKey(tcell.KeyRune, '~', 0)
	handler.ProcessEvent(event)

	select {
	case action := <-actionChan:
		if _, ok := action.(statepkg.GoHomeAction); !ok {
			t.Fatalf("Expected GoHomeAction, got %T", action)
		}
	default:
		t.Fatal("Expected GoHomeAction for tilde key")
	}
}
