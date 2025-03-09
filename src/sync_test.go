package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSyncDirectories tests the directory synchronization functionality
func TestSyncDirectories(t *testing.T) {
	// Use the test directories created in setup
	sourceDir := testSourceDir
	destDir := testDestDir

	// Get the list of test files from the source directory
	var testFiles []struct {
		path    string
		content string
	}

	// Walk through the source directory to find all files
	filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// Add to test files
		testFiles = append(testFiles, struct {
			path    string
			content string
		}{
			path:    relPath,
			content: string(content),
		})

		return nil
	})

	// Initialize status for testing
	status = Status{
		IsSyncing:    false,
		LastSync:     time.Now(),
		NextSyncTime: time.Now().Add(60 * time.Second),
	}

	// Load the test config
	configFile, err := os.ReadFile("test_config.json")
	if err != nil {
		t.Fatalf("Error reading config: %v", err)
	}

	if err := json.Unmarshal(configFile, &config); err != nil {
		t.Fatalf("Error parsing config: %v", err)
	}

	// Check if rsync is available
	_, err = exec.LookPath("rsync")
	rsyncAvailable := err == nil

	if rsyncAvailable {
		t.Log("rsync is available, testing with rsync")
	} else {
		t.Log("rsync is not available, testing with file copy fallback")
	}

	// Create a file in the destination that doesn't exist in the source
	destOnlyFile := filepath.Join(destDir, "dest_only.txt")
	destOnlyContent := "This file exists only in the destination"
	if err := os.WriteFile(destOnlyFile, []byte(destOnlyContent), 0644); err != nil {
		t.Fatalf("Failed to create destination-only file: %v", err)
	}

	// Run the sync
	if err := SyncDirectories(sourceDir, destDir); err != nil {
		t.Fatalf("SyncDirectories failed: %v", err)
	}

	// Verify status was updated correctly
	if status.IsSyncing {
		t.Error("Expected IsSyncing to be false after sync")
	}

	if status.LastError != "" {
		t.Errorf("Expected no error, got: %s", status.LastError)
	}

	// Check if CurrentPair was updated with completion message
	if status.CurrentPair == "" {
		t.Error("Expected CurrentPair to be updated, but it's empty")
	}

	// Verify files were copied correctly
	for _, tf := range testFiles {
		destPath := filepath.Join(destDir, tf.path)

		// Check if file exists
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			t.Errorf("File %s was not copied to destination", tf.path)
			continue
		}

		// Check content
		content, err := os.ReadFile(destPath)
		if err != nil {
			t.Errorf("Failed to read destination file %s: %v", destPath, err)
			continue
		}

		if string(content) != tf.content {
			t.Errorf("File %s content mismatch. Expected: %s, Got: %s", tf.path, tf.content, string(content))
		}
	}

	// Verify the destination-only file still exists and wasn't deleted
	if _, err := os.Stat(destOnlyFile); os.IsNotExist(err) {
		t.Errorf("Destination-only file was deleted during sync")
	} else {
		// Check content
		content, err := os.ReadFile(destOnlyFile)
		if err != nil {
			t.Errorf("Failed to read destination-only file: %v", err)
		} else if string(content) != destOnlyContent {
			t.Errorf("Destination-only file content was changed. Expected: %s, Got: %s",
				destOnlyContent, string(content))
		}
	}
}

// TestEmptySourceDirectory tests that syncing an empty source directory doesn't delete files in destination
func TestEmptySourceDirectory(t *testing.T) {
	// Create temporary empty source directory
	emptySourceDir, err := os.MkdirTemp("", "empty_source")
	if err != nil {
		t.Fatalf("Failed to create empty source directory: %v", err)
	}
	defer os.RemoveAll(emptySourceDir)

	// Create temporary destination directory with a file
	destDir, err := os.MkdirTemp("", "dest_with_files")
	if err != nil {
		t.Fatalf("Failed to create destination directory: %v", err)
	}
	defer os.RemoveAll(destDir)

	// Create a file in the destination
	destFile := filepath.Join(destDir, "existing_file.txt")
	destContent := "This file exists in the destination"
	if err := os.WriteFile(destFile, []byte(destContent), 0644); err != nil {
		t.Fatalf("Failed to create destination file: %v", err)
	}

	// Initialize status for testing
	status = Status{
		IsSyncing:    false,
		LastSync:     time.Now(),
		NextSyncTime: time.Now().Add(60 * time.Second),
	}

	// Run the sync with empty source directory
	if err := SyncDirectories(emptySourceDir, destDir); err != nil {
		t.Fatalf("SyncDirectories failed: %v", err)
	}

	// Verify status was updated correctly
	if status.IsSyncing {
		t.Error("Expected IsSyncing to be false after sync")
	}

	if status.LastError != "" {
		t.Errorf("Expected no error, got: %s", status.LastError)
	}

	// Verify the destination file still exists and wasn't deleted
	if _, err := os.Stat(destFile); os.IsNotExist(err) {
		t.Errorf("Destination file was deleted during sync with empty source")
	} else {
		// Check content
		content, err := os.ReadFile(destFile)
		if err != nil {
			t.Errorf("Failed to read destination file: %v", err)
		} else if string(content) != destContent {
			t.Errorf("Destination file content was changed. Expected: %s, Got: %s",
				destContent, string(content))
		}
	}
}

// TestCopyFile tests the file copying functionality
func TestCopyFile(t *testing.T) {
	// Create temporary source and destination files
	srcFile, err := os.CreateTemp("", "source-*.txt")
	if err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}
	defer os.Remove(srcFile.Name())

	destFile := srcFile.Name() + ".copy"
	defer os.Remove(destFile)

	// Write test content to source file
	testContent := "This is a test file for copying"
	if _, err := srcFile.WriteString(testContent); err != nil {
		t.Fatalf("Failed to write to source file: %v", err)
	}
	srcFile.Close()

	// Set permissions
	if err := os.Chmod(srcFile.Name(), 0644); err != nil {
		t.Fatalf("Failed to set permissions on source file: %v", err)
	}

	// Copy the file
	if err := copyFile(srcFile.Name(), destFile, 0644); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	// Verify the file was copied correctly
	content, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("Failed to read destination file: %v", err)
	}

	if string(content) != testContent {
		t.Errorf("File content mismatch. Expected: %s, Got: %s", testContent, string(content))
	}

	// Check permissions
	info, err := os.Stat(destFile)
	if err != nil {
		t.Fatalf("Failed to stat destination file: %v", err)
	}

	if info.Mode().Perm() != 0644 {
		t.Errorf("File permissions mismatch. Expected: %v, Got: %v", 0644, info.Mode().Perm())
	}
}

// TestSyncWithFileCopy tests the file copy fallback method
func TestSyncWithFileCopy(t *testing.T) {
	// Use the test directories created in setup
	sourceDir := testSourceDir
	destDir := testDestDir + "_filecopy"

	// Create the destination directory
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatalf("Failed to create destination directory: %v", err)
	}
	defer os.RemoveAll(destDir)

	// Create a file in the destination that doesn't exist in the source
	destOnlyFile := filepath.Join(destDir, "dest_only.txt")
	destOnlyContent := "This file exists only in the destination"
	if err := os.WriteFile(destOnlyFile, []byte(destOnlyContent), 0644); err != nil {
		t.Fatalf("Failed to create destination-only file: %v", err)
	}

	// Initialize status for testing
	status = Status{
		IsSyncing:    false,
		LastSync:     time.Now(),
		NextSyncTime: time.Now().Add(60 * time.Second),
	}

	// Run the sync with file copy method
	if err := syncWithFileCopy(sourceDir, destDir); err != nil {
		t.Fatalf("syncWithFileCopy failed: %v", err)
	}

	// Verify status was updated correctly
	if status.IsSyncing {
		t.Error("Expected IsSyncing to be false after sync")
	}

	if status.LastError != "" {
		t.Errorf("Expected no error, got: %s", status.LastError)
	}

	// Check if CurrentPair was updated with completion message
	if !strings.Contains(status.CurrentPair, "Completed:") {
		t.Errorf("Expected CurrentPair to contain completion message, got: %s", status.CurrentPair)
	}

	// Verify files were copied correctly by comparing directories
	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the root directory
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil || relPath == "." {
			return nil
		}

		// Get destination path
		destPath := filepath.Join(destDir, relPath)

		// Check if it exists in destination
		_, err = os.Stat(destPath)
		if os.IsNotExist(err) {
			t.Errorf("File/directory %s was not copied to destination", relPath)
		}

		return nil
	})

	if err != nil {
		t.Fatalf("Error walking source directory: %v", err)
	}

	// Verify the destination-only file still exists and wasn't deleted
	if _, err := os.Stat(destOnlyFile); os.IsNotExist(err) {
		t.Errorf("Destination-only file was deleted during sync")
	} else {
		// Check content
		content, err := os.ReadFile(destOnlyFile)
		if err != nil {
			t.Errorf("Failed to read destination-only file: %v", err)
		} else if string(content) != destOnlyContent {
			t.Errorf("Destination-only file content was changed. Expected: %s, Got: %s",
				destOnlyContent, string(content))
		}
	}
}

// TestIsDirEmpty tests the directory empty check functionality
func TestIsDirEmpty(t *testing.T) {
	// Create a temporary empty directory
	emptyDir, err := os.MkdirTemp("", "empty_dir")
	if err != nil {
		t.Fatalf("Failed to create empty directory: %v", err)
	}
	defer os.RemoveAll(emptyDir)

	// Test empty directory
	empty, err := isDirEmpty(emptyDir)
	if err != nil {
		t.Fatalf("isDirEmpty failed: %v", err)
	}
	if !empty {
		t.Errorf("Expected empty directory to be reported as empty")
	}

	// Create a file in the directory
	testFile := filepath.Join(emptyDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test non-empty directory
	empty, err = isDirEmpty(emptyDir)
	if err != nil {
		t.Fatalf("isDirEmpty failed: %v", err)
	}
	if empty {
		t.Errorf("Expected non-empty directory to be reported as not empty")
	}

	// Test non-existent directory
	_, err = isDirEmpty("/path/that/does/not/exist")
	if err == nil {
		t.Errorf("Expected error for non-existent directory")
	}
}

// TestSetError tests the error setting functionality
func TestSetError(t *testing.T) {
	// Initialize status
	status = Status{
		IsSyncing: true,
		LastError: "",
	}

	// Set an error
	testError := "Test error message"
	setError(testError)

	// Verify status was updated
	if status.IsSyncing {
		t.Error("Expected IsSyncing to be false after error")
	}

	if status.LastError != testError {
		t.Errorf("Expected error message %s, got %s", testError, status.LastError)
	}

	// Check if CurrentPair was updated with error message
	if !strings.Contains(status.CurrentPair, "Error:") {
		t.Errorf("Expected CurrentPair to contain error message, got: %s", status.CurrentPair)
	}
}

// TestStartSyncProcess tests the sync process functionality
// This is a more complex test that involves goroutines and timing
func TestStartSyncProcess(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping StartSyncProcess test in short mode")
	}

	// Use the test directories created in setup
	sourceDir := testSourceDir
	destDir := testDestDir

	// Create a new test file in source
	testFile := filepath.Join(sourceDir, "test_sync_process.txt")
	if err := os.WriteFile(testFile, []byte("Test content for sync process"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create a file in the destination that doesn't exist in the source
	destOnlyFile := filepath.Join(destDir, "dest_only_sync_process.txt")
	destOnlyContent := "This file exists only in the destination for sync process test"
	if err := os.WriteFile(destOnlyFile, []byte(destOnlyContent), 0644); err != nil {
		t.Fatalf("Failed to create destination-only file: %v", err)
	}

	// Set up config with a very short sync interval
	config = Config{
		SyncInterval: 1, // 1 second
		SyncPairs:    []string{sourceDir + ":" + destDir},
	}

	// Initialize status
	now := time.Now()
	status = Status{
		IsSyncing:    false,
		LastSync:     now,
		NextSyncTime: now.Add(time.Second), // Next sync in 1 second
	}

	// Start sync process in a goroutine
	done := make(chan bool)
	go func() {
		// We'll run this for a short time only
		time.Sleep(3 * time.Second)
		done <- true
	}()

	// Start the sync process
	go func() {
		StartSyncProcess()
	}()

	// Wait for the test to complete
	<-done

	// Verify the file was copied
	destFile := filepath.Join(destDir, "test_sync_process.txt")
	if _, err := os.Stat(destFile); os.IsNotExist(err) {
		t.Error("File was not copied by sync process")
	}

	// Verify the destination-only file still exists and wasn't deleted
	if _, err := os.Stat(destOnlyFile); os.IsNotExist(err) {
		t.Error("Destination-only file was deleted during sync process")
	} else {
		// Check content
		content, err := os.ReadFile(destOnlyFile)
		if err != nil {
			t.Errorf("Failed to read destination-only file: %v", err)
		} else if string(content) != destOnlyContent {
			t.Errorf("Destination-only file content was changed. Expected: %s, Got: %s",
				destOnlyContent, string(content))
		}
	}
}
