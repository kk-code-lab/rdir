package state

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	fsutil "github.com/kk-code-lab/rdir/internal/fs"
)

// ===== I/O TESTS =====
// These tests verify file system operations without needing mock UI

func TestLoadDirectory_ValidDirectory(t *testing.T) {
	// Create temporary directory with test files
	tmpDir, err := os.MkdirTemp("", "rdir-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create test files
	if err := os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("content1"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("content2"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	if err := os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	// Create state and load directory
	state := &AppState{
		CurrentPath:  tmpDir,
		ScreenHeight: 24,
		ScreenWidth:  80,
	}

	state.updateParentEntries()

	reducer := NewStateReducer()
	err = reducer.changeDirectory(state, tmpDir)
	if err != nil {
		t.Fatalf("Failed to load directory: %v", err)
	}

	// Verify files loaded
	if len(state.Files) != 3 {
		t.Errorf("Expected 3 files, got %d", len(state.Files))
	}

	// Verify sorting: directories first
	if !state.Files[0].IsDir {
		t.Error("First file should be directory")
	}
	if state.Files[0].Name != "subdir" {
		t.Errorf("Directory should be named 'subdir', got %s", state.Files[0].Name)
	}

	// Verify alphabetical sorting within groups
	if state.Files[1].Name != "file1.txt" {
		t.Errorf("Expected file1.txt, got %s", state.Files[1].Name)
	}
}

func TestLoadDirectory_InvalidDirectory(t *testing.T) {
	state := &AppState{
		CurrentPath:  "/nonexistent/path/that/does/not/exist",
		ScreenHeight: 24,
		ScreenWidth:  80,
	}

	reducer := NewStateReducer()
	err := reducer.changeDirectory(state, state.CurrentPath)

	if err == nil {
		t.Error("Expected error for invalid directory")
	}
}

func TestUpdateParentEntries_HideHiddenFiles(t *testing.T) {
	tmpDir := t.TempDir()
	parentDir := filepath.Join(tmpDir, "parent")
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		t.Fatalf("Failed to create parent dir: %v", err)
	}

	dirs := []string{"visible", "other", ".hidden", ".current"}
	for _, name := range dirs {
		if err := os.MkdirAll(filepath.Join(parentDir, name), 0o755); err != nil {
			t.Fatalf("Failed to create child dir %q: %v", name, err)
		}
		if strings.HasPrefix(name, ".") {
			ensureHidden(t, filepath.Join(parentDir, name))
		}
	}

	state := &AppState{CurrentPath: filepath.Join(parentDir, "visible")}

	state.updateParentEntries()
	names := extractParentNames(state.ParentEntries)
	expectedAll := []string{".current", ".hidden", "other", "visible"}
	if !slices.Equal(names, expectedAll) {
		t.Fatalf("Expected parent entries %v, got %v", expectedAll, names)
	}

	state.HideHiddenFiles = true
	state.updateParentEntries()
	names = extractParentNames(state.ParentEntries)
	expectedVisible := []string{"other", "visible"}
	if !slices.Equal(names, expectedVisible) {
		t.Fatalf("Expected parent entries %v when hiding hidden files, got %v", expectedVisible, names)
	}
}

func TestUpdateParentEntries_HideHiddenKeepsCurrentEntry(t *testing.T) {
	tmpDir := t.TempDir()
	parentDir := filepath.Join(tmpDir, "parent")
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		t.Fatalf("Failed to create parent dir: %v", err)
	}

	for _, name := range []string{".current", ".hidden", "visible"} {
		if err := os.MkdirAll(filepath.Join(parentDir, name), 0o755); err != nil {
			t.Fatalf("Failed to create child dir %q: %v", name, err)
		}
		if strings.HasPrefix(name, ".") {
			ensureHidden(t, filepath.Join(parentDir, name))
		}
	}

	state := &AppState{
		CurrentPath:     filepath.Join(parentDir, ".current"),
		HideHiddenFiles: true,
	}

	state.updateParentEntries()
	names := extractParentNames(state.ParentEntries)
	expected := []string{".current", "visible"}
	if !slices.Equal(names, expected) {
		t.Fatalf("Expected parent entries %v when current directory is hidden, got %v", expected, names)
	}
}

func TestUpdateParentEntries_ShouldHideFromListing(t *testing.T) {
	tmpDir := t.TempDir()
	parentDir := filepath.Join(tmpDir, "parent")
	currentDir := filepath.Join(parentDir, "current")
	skipDir := filepath.Join(parentDir, "skip-me")
	keepDir := filepath.Join(parentDir, "keep-me")

	for _, dir := range []string{currentDir, skipDir, keepDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("Failed to create dir %q: %v", dir, err)
		}
		if strings.HasPrefix(filepath.Base(dir), ".") {
			ensureHidden(t, dir)
		}
	}

	state := &AppState{
		CurrentPath:     currentDir,
		HideHiddenFiles: false,
	}

	prev := shouldHideFromListingFn
	shouldHideFromListingFn = func(fullPath, name string) bool {
		return filepath.Base(fullPath) == "skip-me" || name == "skip-me"
	}
	defer func() {
		shouldHideFromListingFn = prev
	}()

	state.updateParentEntries()
	names := extractParentNames(state.ParentEntries)
	if containsName(names, "skip-me") {
		t.Fatalf("skip-me should be omitted, got %v", names)
	}
	if !containsName(names, "keep-me") {
		t.Fatalf("keep-me should remain, got %v", names)
	}
}

func TestChangeDirectoryRespectsShouldHideFromListing(t *testing.T) {
	tmpDir := t.TempDir()
	skipDir := filepath.Join(tmpDir, "skip-me")
	keepDir := filepath.Join(tmpDir, "keep-me")

	for _, dir := range []string{skipDir, keepDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("Failed to create dir %q: %v", dir, err)
		}
	}

	prev := shouldHideFromListingFn
	shouldHideFromListingFn = func(fullPath, name string) bool {
		return name == "skip-me"
	}
	defer func() {
		shouldHideFromListingFn = prev
	}()

	state := &AppState{}
	reducer := NewStateReducer()
	if err := reducer.changeDirectory(state, tmpDir); err != nil {
		t.Fatalf("changeDirectory failed: %v", err)
	}

	names := fileNames(state.Files)
	if containsName(names, "skip-me") {
		t.Fatalf("skip-me should be omitted from directory listing, got %v", names)
	}
	if !containsName(names, "keep-me") {
		t.Fatalf("keep-me should remain, got %v", names)
	}
}

func TestGeneratePreview_TextFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "rdir-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create a text file
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "Line 1\nLine 2\nLine 3"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	state := &AppState{
		CurrentPath:   tmpDir,
		SelectedIndex: 0,
		ScreenHeight:  24,
		ScreenWidth:   80,
	}

	// Load directory to populate Files
	reducer := NewStateReducer()
	if err := reducer.changeDirectory(state, tmpDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Generate preview
	err = reducer.generatePreview(state)
	if err != nil {
		t.Fatalf("Failed to generate preview: %v", err)
	}

	if state.PreviewData == nil {
		t.Fatal("PreviewData should not be nil")
	}

	// Verify text content
	if state.PreviewData.IsDir {
		t.Error("Preview should be for file, not directory")
	}

	if state.PreviewData.LineCount != 3 {
		t.Errorf("Expected 3 lines, got %d", state.PreviewData.LineCount)
	}

	if len(state.PreviewData.TextLines) < 3 {
		t.Errorf("Expected at least 3 text lines, got %d", len(state.PreviewData.TextLines))
	}

	if state.PreviewData.TextLines[0] != "Line 1" {
		t.Errorf("Expected 'Line 1', got %q", state.PreviewData.TextLines[0])
	}
}

func TestGeneratePreview_Directory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "rdir-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create subdirectory with files
	subdir := filepath.Join(tmpDir, "mydir")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "file1.txt"), []byte("content1"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "file2.txt"), []byte("content2"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	state := &AppState{
		CurrentPath:   tmpDir,
		SelectedIndex: 0,
		ScreenHeight:  24,
		ScreenWidth:   80,
	}

	// Load directory
	reducer := NewStateReducer()
	if err := reducer.changeDirectory(state, tmpDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Generate preview
	err = reducer.generatePreview(state)
	if err != nil {
		t.Fatalf("Failed to generate preview: %v", err)
	}

	if state.PreviewData == nil {
		t.Fatal("PreviewData should not be nil")
	}

	// Verify it's a directory preview
	if !state.PreviewData.IsDir {
		t.Error("Preview should be for directory")
	}

	// Should have contents
	if len(state.PreviewData.DirEntries) == 0 {
		t.Error("Directory preview should show contents")
	}
}

func TestGeneratePreview_BinaryFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "rdir-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create a binary file (with many non-printable chars)
	binaryFile := filepath.Join(tmpDir, "binary.bin")
	binaryContent := make([]byte, 512)
	for i := 0; i < 512; i++ {
		binaryContent[i] = byte(i % 10) // Mostly non-printable
	}
	if err := os.WriteFile(binaryFile, binaryContent, 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	state := &AppState{
		CurrentPath:   tmpDir,
		SelectedIndex: 0,
		ScreenHeight:  24,
		ScreenWidth:   80,
	}

	// Load directory
	reducer := NewStateReducer()
	if err := reducer.changeDirectory(state, tmpDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Generate preview
	err = reducer.generatePreview(state)
	if err != nil {
		t.Fatalf("Failed to generate preview: %v", err)
	}

	if state.PreviewData == nil {
		t.Fatal("PreviewData should not be nil")
	}

	// Binary file should not have text lines
	if len(state.PreviewData.TextLines) > 0 {
		t.Error("Binary file should not have text preview")
	}

	if len(state.PreviewData.BinaryInfo.Lines) == 0 {
		t.Error("Binary file should have binary preview lines")
	}
}

func TestIsTextFile_PlainText(t *testing.T) {
	content := []byte("Hello world\nThis is plain text\n")
	if !fsutil.IsTextFile("plain.txt", content) {
		t.Error("Plain text should be detected as text")
	}
}

func TestIsTextFile_BinaryData(t *testing.T) {
	// Create binary data with >30% non-printable characters
	binaryData := make([]byte, 512)
	// Fill with mostly non-printable chars (low bytes < 9, or in range 14-31, 127)
	for i := 0; i < 512; i++ {
		binaryData[i] = byte(i % 10) // 0-9: only 9 is printable
	}
	if fsutil.IsTextFile("binary.bin", binaryData) {
		t.Error("Binary data should not be detected as text")
	}
}

func TestIsTextFile_EmptyFile(t *testing.T) {
	content := []byte("")
	if !fsutil.IsTextFile("empty.txt", content) {
		t.Error("Empty file should be detected as text")
	}
}

func TestIsTextFile_UTF8Text(t *testing.T) {
	content := []byte("Hello 世界\nЭто текст\n")
	if !fsutil.IsTextFile("utf8.txt", content) {
		t.Error("UTF-8 text should be detected as text")
	}
}

func TestIsTextFile_BinaryExtension(t *testing.T) {
	content := []byte("Pretend this is image data but still ASCII")
	if fsutil.IsTextFile("photo.png", content) {
		t.Error("Known binary extension should be treated as binary")
	}
}

func TestIsTextFile_NullByte(t *testing.T) {
	content := []byte("abc\x00def")
	if fsutil.IsTextFile("null.bin", content) {
		t.Error("Content with NUL byte should be treated as binary")
	}
}

func TestIsTextFile_Latin1(t *testing.T) {
	content := []byte{0x48, 0x65, 0x6C, 0x6C, 0x6F, 0xA0, 0xC4, 0xD6, 0xDC}
	if !fsutil.IsTextFile("latin1.txt", content) {
		t.Error("Latin-1 text should be treated as text")
	}
}

// ===== FILE SYSTEM INTEGRATION TESTS =====

func TestFullNavigationFlow_WithRealFilesystem(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "rdir-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create directory structure
	//  tmpDir/
	//    ├── dir1/
	//    │   ├── file1.txt
	//    │   └── file2.txt
	//    └── dir2/
	//        └── file3.txt

	dir1 := filepath.Join(tmpDir, "dir1")
	dir2 := filepath.Join(tmpDir, "dir2")
	if err := os.Mkdir(dir1, 0755); err != nil {
		t.Fatalf("Failed to create dir1: %v", err)
	}
	if err := os.Mkdir(dir2, 0755); err != nil {
		t.Fatalf("Failed to create dir2: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir1, "file1.txt"), []byte("content1"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir1, "file2.txt"), []byte("content2"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir2, "file3.txt"), []byte("content3"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Start in tmpDir
	state := &AppState{
		CurrentPath:   tmpDir,
		History:       []string{tmpDir},
		HistoryIndex:  0,
		SelectedIndex: 0,
		ScreenHeight:  24,
		ScreenWidth:   80,
	}

	reducer := NewStateReducer()
	if err := reducer.changeDirectory(state, tmpDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}
	state.updateParentEntries()

	// Should see 2 directories
	if len(state.Files) != 2 {
		t.Errorf("Expected 2 dirs in root, got %d", len(state.Files))
	}

	// Navigate into dir1
	if _, err := reducer.Reduce(state, EnterDirectoryAction{}); err != nil {
		t.Fatalf("Failed to enter directory: %v", err)
	}

	// Should now be in dir1
	if filepath.Base(state.CurrentPath) != "dir1" {
		t.Errorf("Expected to be in dir1, but in %s", state.CurrentPath)
	}

	// Should see 2 files
	if len(state.Files) != 2 {
		t.Errorf("Expected 2 files in dir1, got %d", len(state.Files))
	}

	// Go back up
	if _, err := reducer.Reduce(state, GoUpAction{}); err != nil {
		t.Fatalf("Failed to go up: %v", err)
	}

	// Should be back in tmpDir
	if state.CurrentPath != tmpDir {
		t.Errorf("Expected to be back in tmpDir, but in %s", state.CurrentPath)
	}

	// Should have dir1 selected (smart selection)
	if !state.Files[state.SelectedIndex].IsDir || state.Files[state.SelectedIndex].Name != "dir1" {
		t.Errorf("Expected dir1 to be selected, got %s", state.Files[state.SelectedIndex].Name)
	}
}

func TestFilterWithRealFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "rdir-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create test files
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte("package main"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("# README"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	state := &AppState{
		CurrentPath:  tmpDir,
		ScreenHeight: 24,
		ScreenWidth:  80,
	}

	reducer := NewStateReducer()
	if err := reducer.changeDirectory(state, tmpDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Start filtering for ".go" files
	if _, err := reducer.Reduce(state, FilterStartAction{}); err != nil {
		t.Fatalf("Failed to start filter: %v", err)
	}
	if _, err := reducer.Reduce(state, FilterCharAction{Char: '.'}); err != nil {
		t.Fatalf("Failed to filter: %v", err)
	}
	if _, err := reducer.Reduce(state, FilterCharAction{Char: 'g'}); err != nil {
		t.Fatalf("Failed to filter: %v", err)
	}
	if _, err := reducer.Reduce(state, FilterCharAction{Char: 'o'}); err != nil {
		t.Fatalf("Failed to filter: %v", err)
	}

	// Should have 2 matches
	displayFiles := state.getDisplayFiles()
	if len(displayFiles) != 2 {
		t.Errorf("Expected 2 .go files, got %d", len(displayFiles))
	}

	// Verify they are go files
	names := map[string]bool{}
	for _, f := range displayFiles {
		names[f.Name] = true
	}

	if !names["main.go"] || !names["test.go"] {
		t.Error("Filtered results should contain both .go files")
	}
}

func TestGoHomeActionNavigatesToHomeDirectory(t *testing.T) {
	tmpRoot, err := os.MkdirTemp("", "rdir-home-")
	if err != nil {
		t.Fatalf("Failed to create temp root: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpRoot)
	}()

	homeDir := filepath.Join(tmpRoot, "home")
	workDir := filepath.Join(tmpRoot, "work")

	for _, dir := range []string{homeDir, workDir} {
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatalf("Failed to create %s: %v", dir, err)
		}
	}

	// Populate directories so sorting/selection behave predictably.
	if err := os.WriteFile(filepath.Join(workDir, "work.txt"), []byte("data"), 0644); err != nil {
		t.Fatalf("Failed to seed work dir: %v", err)
	}
	if err := os.Mkdir(filepath.Join(homeDir, "alpha"), 0755); err != nil {
		t.Fatalf("Failed to seed home dir alpha: %v", err)
	}
	if err := os.Mkdir(filepath.Join(homeDir, "beta"), 0755); err != nil {
		t.Fatalf("Failed to seed home dir beta: %v", err)
	}

	state := &AppState{
		CurrentPath:  workDir,
		History:      []string{workDir},
		HistoryIndex: 0,
		ScreenHeight: 24,
		ScreenWidth:  80,
	}

	reducer := NewStateReducer()
	if err := reducer.changeDirectory(state, workDir); err != nil {
		t.Fatalf("Failed to load work dir: %v", err)
	}

	originalHomeFn := userHomeDirFn
	userHomeDirFn = func() (string, error) {
		return homeDir, nil
	}
	defer func() {
		userHomeDirFn = originalHomeFn
	}()

	// Pretend the user left the cursor on the second entry at home.
	reducer.selectionHistory[homeDir] = 1

	if _, err := reducer.Reduce(state, GoHomeAction{}); err != nil {
		t.Fatalf("GoHomeAction failed: %v", err)
	}

	if state.CurrentPath != homeDir {
		t.Fatalf("Expected to be in %s, got %s", homeDir, state.CurrentPath)
	}

	if state.SelectedIndex != 1 {
		t.Fatalf("Expected selection index 1 to be restored, got %d", state.SelectedIndex)
	}

	if len(state.History) != 2 || state.History[len(state.History)-1] != homeDir {
		t.Fatalf("History should end with home directory, got %v", state.History)
	}

	if state.HistoryIndex != len(state.History)-1 {
		t.Fatalf("HistoryIndex should point to latest entry, got %d", state.HistoryIndex)
	}
}

func extractParentNames(entries []FileEntry) []string {
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name
	}
	return names
}

func fileNames(entries []FileEntry) []string {
	return extractParentNames(entries)
}

func containsName(names []string, target string) bool {
	for _, name := range names {
		if name == target {
			return true
		}
	}
	return false
}

func ensureHidden(t *testing.T, path string) {
	t.Helper()
	if err := markHiddenForTest(path); err != nil {
		t.Fatalf("failed to mark %s hidden: %v", path, err)
	}
}
