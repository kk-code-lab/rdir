package state

import (
	"os"
	"path/filepath"
	"testing"
)

// ===== SYMLINK TESTS =====
// These tests verify symlink detection and handling

func TestLoadDirectory_SymlinkToDirectory(t *testing.T) {
	// Create temporary directories
	tmpDir, err := os.MkdirTemp("", "rdir-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create a target directory
	targetDir := filepath.Join(tmpDir, "target")
	if err := os.Mkdir(targetDir, 0755); err != nil {
		t.Fatalf("Failed to create target dir: %v", err)
	}

	// Create a symlink to the target directory
	symlinkPath := filepath.Join(tmpDir, "linkdir")
	if err := os.Symlink(targetDir, symlinkPath); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	// Load directory and check symlink detection
	state := &AppState{
		CurrentPath:  tmpDir,
		ScreenHeight: 24,
		ScreenWidth:  80,
	}

	reducer := NewStateReducer()
	err = reducer.changeDirectory(state, tmpDir)
	if err != nil {
		t.Fatalf("Failed to load directory: %v", err)
	}

	// Find the symlink entry
	var symlinkEntry *FileEntry
	for i := range state.Files {
		if state.Files[i].Name == "linkdir" {
			symlinkEntry = &state.Files[i]
			break
		}
	}

	if symlinkEntry == nil {
		t.Fatal("Symlink not found in Files")
	}

	// Verify symlink is detected and marked as directory
	if !symlinkEntry.IsSymlink {
		t.Error("IsSymlink should be true for symlink to directory")
	}
	if !symlinkEntry.IsDir {
		t.Error("IsDir should be true for symlink to directory (target is directory)")
	}
	if symlinkEntry.Name != "linkdir" {
		t.Errorf("Name should be 'linkdir', got %s", symlinkEntry.Name)
	}
}

func TestLoadDirectory_SymlinkToFile(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "rdir-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create a target file
	targetFile := filepath.Join(tmpDir, "target.txt")
	if err := os.WriteFile(targetFile, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create target file: %v", err)
	}

	// Create a symlink to the target file
	symlinkPath := filepath.Join(tmpDir, "linkfile")
	if err := os.Symlink(targetFile, symlinkPath); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	// Load directory and check symlink detection
	state := &AppState{
		CurrentPath:  tmpDir,
		ScreenHeight: 24,
		ScreenWidth:  80,
	}

	reducer := NewStateReducer()
	err = reducer.changeDirectory(state, tmpDir)
	if err != nil {
		t.Fatalf("Failed to load directory: %v", err)
	}

	// Find the symlink entry
	var symlinkEntry *FileEntry
	for i := range state.Files {
		if state.Files[i].Name == "linkfile" {
			symlinkEntry = &state.Files[i]
			break
		}
	}

	if symlinkEntry == nil {
		t.Fatal("Symlink not found in Files")
	}

	// Verify symlink is detected and marked as file
	if !symlinkEntry.IsSymlink {
		t.Error("IsSymlink should be true for symlink to file")
	}
	if symlinkEntry.IsDir {
		t.Error("IsDir should be false for symlink to file (target is file)")
	}
	if symlinkEntry.Name != "linkfile" {
		t.Errorf("Name should be 'linkfile', got %s", symlinkEntry.Name)
	}
}

func TestLoadDirectory_BrokenSymlink(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "rdir-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create a symlink to non-existent target
	symlinkPath := filepath.Join(tmpDir, "brokenlink")
	nonexistentTarget := filepath.Join(tmpDir, "nonexistent")
	if err := os.Symlink(nonexistentTarget, symlinkPath); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	// Load directory and check broken symlink handling
	state := &AppState{
		CurrentPath:  tmpDir,
		ScreenHeight: 24,
		ScreenWidth:  80,
	}

	reducer := NewStateReducer()
	err = reducer.changeDirectory(state, tmpDir)
	if err != nil {
		t.Fatalf("Failed to load directory: %v", err)
	}

	// Find the broken symlink entry
	var symlinkEntry *FileEntry
	for i := range state.Files {
		if state.Files[i].Name == "brokenlink" {
			symlinkEntry = &state.Files[i]
			break
		}
	}

	if symlinkEntry == nil {
		t.Fatal("Broken symlink not found in Files")
	}

	// Verify broken symlink is detected
	if !symlinkEntry.IsSymlink {
		t.Error("IsSymlink should be true for broken symlink")
	}
	// For broken symlink, IsDir is based on lstat (which shows symlink, not target)
	// os.Stat fails, so isDir remains false from e.IsDir()
	if symlinkEntry.IsDir {
		t.Error("IsDir should be false for broken symlink (target is inaccessible)")
	}
}

func TestLoadDirectory_SymlinkWithHiddenFiles(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "rdir-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create a hidden target directory
	targetDir := filepath.Join(tmpDir, ".hidden")
	if err := os.Mkdir(targetDir, 0755); err != nil {
		t.Fatalf("Failed to create hidden target dir: %v", err)
	}
	ensureHidden(t, targetDir)

	// Create symlink to hidden directory
	symlinkPath := filepath.Join(tmpDir, "hidden_link")
	if err := os.Symlink(targetDir, symlinkPath); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	// Load directory
	state := &AppState{
		CurrentPath:     tmpDir,
		ScreenHeight:    24,
		ScreenWidth:     80,
		HideHiddenFiles: true,
	}

	reducer := NewStateReducer()
	err = reducer.changeDirectory(state, tmpDir)
	if err != nil {
		t.Fatalf("Failed to load directory: %v", err)
	}

	// Symlink itself is not hidden (name doesn't start with .)
	// It should be in display files
	displayFiles := state.getDisplayFiles()
	var foundSymlink bool
	for _, f := range displayFiles {
		if f.Name == "hidden_link" {
			foundSymlink = true
			if !f.IsSymlink {
				t.Error("Symlink should be marked as IsSymlink=true")
			}
			if !f.IsDir {
				t.Error("Symlink should be marked as IsDir=true (target is directory)")
			}
			break
		}
	}

	if !foundSymlink {
		t.Error("Symlink to hidden directory should be visible (symlink name is not hidden)")
	}
}

func TestEnterSymlinkDirectory(t *testing.T) {
	// Create temporary directories
	tmpDir, err := os.MkdirTemp("", "rdir-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create target directory with a file
	targetDir := filepath.Join(tmpDir, "target")
	if err := os.Mkdir(targetDir, 0755); err != nil {
		t.Fatalf("Failed to create target dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "testfile.txt"), []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create symlink to directory
	symlinkPath := filepath.Join(tmpDir, "linkdir")
	if err := os.Symlink(targetDir, symlinkPath); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	// Load initial directory
	state := &AppState{
		CurrentPath:  tmpDir,
		ScreenHeight: 24,
		ScreenWidth:  80,
	}

	reducer := NewStateReducer()
	err = reducer.changeDirectory(state, tmpDir)
	if err != nil {
		t.Fatalf("Failed to load directory: %v", err)
	}

	// Find symlink and select it
	for i, f := range state.Files {
		if f.Name == "linkdir" {
			state.SelectedIndex = i
			break
		}
	}

	// Get path to selected file and enter it
	selectedFile := state.Files[state.SelectedIndex]
	if !selectedFile.IsDir {
		t.Fatal("Selected file should be a directory")
	}

	// Build path to symlink
	symlinkFullPath := filepath.Join(state.CurrentPath, selectedFile.Name)

	// Load the symlink directory - should load target's contents
	err = reducer.changeDirectory(state, symlinkFullPath)
	if err != nil {
		t.Fatalf("Failed to enter symlink directory: %v", err)
	}

	// Verify we're inside the symlink (current path is the symlink path)
	if state.CurrentPath != symlinkFullPath {
		t.Errorf("CurrentPath should be symlink path %s, got %s", symlinkFullPath, state.CurrentPath)
	}

	// Verify contents are from target directory
	if len(state.Files) != 1 {
		t.Errorf("Expected 1 file in target directory, got %d", len(state.Files))
	}
	if state.Files[0].Name != "testfile.txt" {
		t.Errorf("Expected testfile.txt, got %s", state.Files[0].Name)
	}
}

func TestEnterSymlinkDirectory_FollowsTarget(t *testing.T) {
	// Create temporary directories
	tmpDir, err := os.MkdirTemp("", "rdir-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create target directory with nested structure
	targetDir := filepath.Join(tmpDir, "target")
	if err := os.Mkdir(targetDir, 0755); err != nil {
		t.Fatalf("Failed to create target dir: %v", err)
	}
	nestedDir := filepath.Join(targetDir, "nested")
	if err := os.Mkdir(nestedDir, 0755); err != nil {
		t.Fatalf("Failed to create nested dir: %v", err)
	}

	// Create symlink
	symlinkPath := filepath.Join(tmpDir, "linkdir")
	if err := os.Symlink(targetDir, symlinkPath); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	// Load initial directory
	state := &AppState{
		CurrentPath:  tmpDir,
		ScreenHeight: 24,
		ScreenWidth:  80,
	}

	reducer := NewStateReducer()
	err = reducer.changeDirectory(state, tmpDir)
	if err != nil {
		t.Fatalf("Failed to load directory: %v", err)
	}

	// Enter symlink directory
	err = reducer.changeDirectory(state, symlinkPath)
	if err != nil {
		t.Fatalf("Failed to enter symlink directory: %v", err)
	}

	// Verify we can see the nested directory inside symlink
	found := false
	for _, f := range state.Files {
		if f.Name == "nested" && f.IsDir {
			found = true
			break
		}
	}

	if !found {
		t.Error("Should be able to navigate inside symlink directory and see nested directories")
	}
}

func TestGetSymlinkTarget(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "rdir-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create a target directory
	targetDir := filepath.Join(tmpDir, "target")
	if err := os.Mkdir(targetDir, 0755); err != nil {
		t.Fatalf("Failed to create target dir: %v", err)
	}

	// Create a symlink to the target directory
	symlinkPath := filepath.Join(tmpDir, "linkdir")
	if err := os.Symlink(targetDir, symlinkPath); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	// Load directory and select symlink
	state := &AppState{
		CurrentPath:  tmpDir,
		ScreenHeight: 24,
		ScreenWidth:  80,
	}

	reducer := NewStateReducer()
	err = reducer.changeDirectory(state, tmpDir)
	if err != nil {
		t.Fatalf("Failed to load directory: %v", err)
	}

	// Find and select symlink
	for i, f := range state.Files {
		if f.Name == "linkdir" {
			state.SelectedIndex = i
			break
		}
	}

	// Get symlink target
	target := state.getSymlinkTarget()

	if target == "" {
		t.Error("getSymlinkTarget should return non-empty string for symlink")
	}
	if target != targetDir {
		t.Errorf("Expected target '%s', got '%s'", targetDir, target)
	}
}

func TestGetSymlinkTarget_NotSymlink(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "rdir-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Create a regular file
	filePath := filepath.Join(tmpDir, "regular.txt")
	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// Load directory and select file
	state := &AppState{
		CurrentPath:  tmpDir,
		ScreenHeight: 24,
		ScreenWidth:  80,
	}

	reducer := NewStateReducer()
	err = reducer.changeDirectory(state, tmpDir)
	if err != nil {
		t.Fatalf("Failed to load directory: %v", err)
	}

	// Find and select file
	for i, f := range state.Files {
		if f.Name == "regular.txt" {
			state.SelectedIndex = i
			break
		}
	}

	// Get symlink target - should return empty string
	target := state.getSymlinkTarget()

	if target != "" {
		t.Errorf("getSymlinkTarget should return empty string for non-symlink, got '%s'", target)
	}
}
