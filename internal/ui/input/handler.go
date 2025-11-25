package input

import (
	"unicode"

	"github.com/gdamore/tcell/v2"
	statepkg "github.com/kk-code-lab/rdir/internal/state"
)

// InputHandler converts tcell events to Actions
type InputHandler struct {
	actionChan chan statepkg.Action
	state      *statepkg.AppState // Reference to current state for mode checking
}

// NewInputHandler creates a new input handler
func NewInputHandler(actionChan chan statepkg.Action) *InputHandler {
	return &InputHandler{
		actionChan: actionChan,
	}
}

// SetState sets the state reference for mode checking
func (ih *InputHandler) SetState(state *statepkg.AppState) {
	ih.state = state
}

// ProcessEvent converts a tcell event into an Action
func (ih *InputHandler) ProcessEvent(ev tcell.Event) bool {
	switch ev := ev.(type) {
	case *tcell.EventKey:
		return ih.processKeyEvent(ev)
	case *tcell.EventResize:
		w, h := ev.Size()
		ih.actionChan <- statepkg.ResizeAction{Width: w, Height: h}
		return true
	default:
		return true
	}
}

// processKeyEvent handles keyboard input
func (ih *InputHandler) processKeyEvent(ev *tcell.EventKey) bool {
	// Check if we're in search mode (filter or global)
	inFilterMode := ih.state != nil && ih.state.FilterActive
	inGlobalSearch := ih.state != nil && ih.state.GlobalSearchActive
	inSearchMode := inFilterMode || inGlobalSearch
	helpVisible := ih.state != nil && ih.state.HelpVisible
	previewFullScreen := ih.state != nil && ih.state.PreviewFullScreen
	previewAvailable := ih.state != nil && ih.state.PreviewData != nil

	if helpVisible {
		switch ev.Key() {
		case tcell.KeyCtrlC:
			ih.actionChan <- statepkg.QuitAction{}
			return false
		case tcell.KeyEscape:
			ih.actionChan <- statepkg.HelpHideAction{}
			return true
		case tcell.KeyRune:
			r := ev.Rune()
			if r == '?' || r == 'q' || r == 'Q' {
				ih.actionChan <- statepkg.HelpHideAction{}
			}
			return true
		default:
			return true
		}
	}

	// Handle special keys first
	switch ev.Key() {
	case tcell.KeyEscape:
		if previewFullScreen {
			ih.actionChan <- statepkg.PreviewExitFullScreenAction{}
		} else if inGlobalSearch {
			if ih.state != nil && ih.state.GlobalSearchQuery != "" {
				ih.actionChan <- statepkg.GlobalSearchResetQueryAction{}
			} else {
				ih.actionChan <- statepkg.GlobalSearchClearAction{}
			}
		} else if inFilterMode {
			ih.actionChan <- statepkg.FilterClearAction{}
		}
		return true

	case tcell.KeyCtrlC:
		ih.actionChan <- statepkg.QuitAction{}
		return false

	case tcell.KeyUp:
		if previewFullScreen {
			ih.actionChan <- statepkg.PreviewScrollUpAction{}
		} else if inGlobalSearch {
			ih.actionChan <- statepkg.GlobalSearchNavigateAction{Direction: "up"}
		} else if inFilterMode || !inSearchMode {
			ih.actionChan <- statepkg.NavigateUpAction{}
		}
		return true

	case tcell.KeyDown:
		if previewFullScreen {
			ih.actionChan <- statepkg.PreviewScrollDownAction{}
		} else if inGlobalSearch {
			ih.actionChan <- statepkg.GlobalSearchNavigateAction{Direction: "down"}
		} else if inFilterMode || !inSearchMode {
			ih.actionChan <- statepkg.NavigateDownAction{}
		}
		return true

	case tcell.KeyEnter:
		if inGlobalSearch {
			ih.actionChan <- statepkg.GlobalSearchOpenAction{}
		} else if inFilterMode {
			ih.actionChan <- statepkg.FilterClearAction{}
		} else {
			ih.actionChan <- statepkg.RightArrowAction{}
		}
		return true

	case tcell.KeyRight:
		if inGlobalSearch {
			if ev.Modifiers()&tcell.ModCtrl != 0 {
				ih.actionChan <- statepkg.GlobalSearchMoveCursorAction{Direction: "word-right"}
			} else {
				ih.actionChan <- statepkg.GlobalSearchMoveCursorAction{Direction: "right"}
			}
			return true
		}
		if previewFullScreen {
			return true
		}

		if inFilterMode {
			// Stay in filter mode when previewing files; only clear when navigating into dirs
			shouldClearFilter := true
			if ih.state != nil {
				if file := ih.state.CurrentFile(); file != nil && !file.IsDir {
					shouldClearFilter = false
				}
			}
			if shouldClearFilter {
				ih.actionChan <- statepkg.FilterClearAction{}
			}
			ih.actionChan <- statepkg.RightArrowAction{}
			return true
		}

		ih.actionChan <- statepkg.RightArrowAction{}
		return true

	case tcell.KeyLeft:
		if inGlobalSearch {
			if ev.Modifiers()&tcell.ModCtrl != 0 {
				ih.actionChan <- statepkg.GlobalSearchMoveCursorAction{Direction: "word-left"}
			} else {
				ih.actionChan <- statepkg.GlobalSearchMoveCursorAction{Direction: "left"}
			}
		} else if previewFullScreen {
			ih.actionChan <- statepkg.PreviewExitFullScreenAction{}
		} else if inFilterMode {
			queryEmpty := true
			if ih.state != nil {
				queryEmpty = ih.state.FilterQuery == ""
			}
			if queryEmpty {
				ih.actionChan <- statepkg.FilterClearAction{}
			} else {
				ih.actionChan <- statepkg.FilterResetQueryAction{}
			}
		} else if !inSearchMode {
			ih.actionChan <- statepkg.GoUpAction{}
		}
		return true

	case tcell.KeyPgUp:
		if previewFullScreen {
			ih.actionChan <- statepkg.PreviewScrollPageUpAction{}
		} else if inGlobalSearch {
			ih.actionChan <- statepkg.GlobalSearchPageUpAction{}
		} else if !inFilterMode {
			ih.actionChan <- statepkg.ScrollPageUpAction{}
		}
		return true

	case tcell.KeyPgDn:
		if previewFullScreen {
			ih.actionChan <- statepkg.PreviewScrollPageDownAction{}
		} else if inGlobalSearch {
			ih.actionChan <- statepkg.GlobalSearchPageDownAction{}
		} else if !inFilterMode {
			ih.actionChan <- statepkg.ScrollPageDownAction{}
		}
		return true

	case tcell.KeyHome:
		if previewFullScreen {
			ih.actionChan <- statepkg.PreviewScrollToStartAction{}
		} else if inGlobalSearch {
			ih.actionChan <- statepkg.GlobalSearchMoveCursorAction{Direction: "home"}
		} else {
			ih.actionChan <- statepkg.ScrollToStartAction{}
		}
		return true

	case tcell.KeyEnd:
		if previewFullScreen {
			ih.actionChan <- statepkg.PreviewScrollToEndAction{}
		} else if inGlobalSearch {
			ih.actionChan <- statepkg.GlobalSearchMoveCursorAction{Direction: "end"}
		} else {
			ih.actionChan <- statepkg.ScrollToEndAction{}
		}
		return true

	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if inGlobalSearch {
			ih.actionChan <- statepkg.GlobalSearchBackspaceAction{}
		} else if inFilterMode {
			ih.actionChan <- statepkg.FilterBackspaceAction{}
		}
		return true

	case tcell.KeyDelete:
		if inGlobalSearch {
			ih.actionChan <- statepkg.GlobalSearchDeleteAction{}
		} else if inFilterMode {
			ih.actionChan <- statepkg.FilterBackspaceAction{}
		}
		return true

	case tcell.KeyCtrlA:
		if inGlobalSearch {
			ih.actionChan <- statepkg.GlobalSearchMoveCursorAction{Direction: "home"}
		}
		return true

	case tcell.KeyCtrlE:
		if inGlobalSearch {
			ih.actionChan <- statepkg.GlobalSearchMoveCursorAction{Direction: "end"}
		}
		return true

	case tcell.KeyCtrlW:
		if inGlobalSearch {
			ih.actionChan <- statepkg.GlobalSearchDeleteWordAction{}
		}
		return true

	case tcell.KeyRune:
		r := ev.Rune()
		if ev.Modifiers()&tcell.ModCtrl != 0 {
			switch r {
			case 'a', 'A':
				if inGlobalSearch {
					ih.actionChan <- statepkg.GlobalSearchMoveCursorAction{Direction: "home"}
					return true
				}
			case 'e', 'E':
				if inGlobalSearch {
					ih.actionChan <- statepkg.GlobalSearchMoveCursorAction{Direction: "end"}
					return true
				}
			case 'w', 'W':
				if inGlobalSearch {
					ih.actionChan <- statepkg.GlobalSearchDeleteWordAction{}
					return true
				}
			case 'h', 'H':
				if inFilterMode {
					ih.actionChan <- statepkg.FilterBackspaceAction{}
					return true
				}
			}
		}
		if ev.Modifiers()&tcell.ModShift != 0 {
			// Normalize shifted alphabetic runes to reflect user intent (Shift+A => 'A')
			r = unicode.ToUpper(r)
		}

		// If in search mode, allow all characters as search input (including 'q')
		if inSearchMode {
			// For global search, allow all characters as search input
			if inGlobalSearch {
				ih.actionChan <- statepkg.GlobalSearchCharAction{Char: r}
				return true
			}

			// For filter mode, allow all characters as search input
			if inFilterMode {
				ih.actionChan <- statepkg.FilterCharAction{Char: r}
				return true
			}
		}

		// Normal mode keys (only when not in search)
		if !inSearchMode {
			switch r {
			case 'q':
				if previewFullScreen {
					ih.actionChan <- statepkg.PreviewExitFullScreenAction{}
					return true
				}
				ih.actionChan <- statepkg.QuitAction{}
				return false

			case 'x':
				if previewFullScreen {
					ih.actionChan <- statepkg.PreviewExitFullScreenAction{}
					return true
				}
				ih.actionChan <- statepkg.QuitAndChangeAction{}
				return false

			case '?':
				ih.actionChan <- statepkg.HelpToggleAction{}
				return true

			case '.':
				if previewFullScreen {
					return true
				}
				ih.actionChan <- statepkg.ToggleHiddenFilesAction{}
				return true

			case 'w', 'W':
				if previewFullScreen && previewAvailable {
					ih.actionChan <- statepkg.TogglePreviewWrapAction{}
				}
				return true

			case 'P':
				ih.actionChan <- statepkg.OpenPagerAction{}
				return true

			case '/':
				ih.actionChan <- statepkg.FilterStartAction{}
				return true

			case 'f':
				ih.actionChan <- statepkg.GlobalSearchStartAction{}
				return true

			case '[':
				ih.actionChan <- statepkg.GoToHistoryAction{Direction: "back"}
				return true

			case ']':
				ih.actionChan <- statepkg.GoToHistoryAction{Direction: "forward"}
				return true

			case 'y':
				ih.actionChan <- statepkg.YankPathAction{}
				return true

			case '~':
				ih.actionChan <- statepkg.GoHomeAction{}
				return true

			case 'e', 'E':
				if ih.state != nil && ih.state.EditorAvailable {
					ih.actionChan <- statepkg.OpenEditorAction{}
				}
				return true

			case 'r', 'R':
				ih.actionChan <- statepkg.RefreshDirectoryAction{}
				return true

			case 'h':
				return true
			}
		}

		return true

	default:
		return true
	}
}
