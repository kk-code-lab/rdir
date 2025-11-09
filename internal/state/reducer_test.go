package state

// ===== REDUCER TESTS =====
//
// Tests have been split into focused files for better maintainability:
// - reducer_navigation_test.go: NavigateDown, NavigateUp, Scroll tests (13 tests)
// - reducer_filter_test.go: Filter activation, typing, backspace tests (8 tests)
// - reducer_hidden_files_test.go: ToggleHiddenFiles and cursor positioning (16 tests)
// - reducer_filter_hidden_test.go: Filter + Hidden file interactions (4 tests)
// - reducer_state_test.go: State helper tests (getDisplayFiles, sortFiles, etc) (7 tests)
//
// Additional test files:
// - reducer_io_test.go: Filesystem integration tests
// - reducer_history_simple_test.go: History navigation tests
// - fuzzy_test.go: Fuzzy matching algorithm tests
// - fuzzy_integration_test.go: Fuzzy search end-to-end tests
// - test_esc_logic_test.go: Esc key behavior tests
// - test_filter_cursor_restoration_test.go: Filter cursor restoration tests
//
// Total: 79 tests covering all reducer functionality
