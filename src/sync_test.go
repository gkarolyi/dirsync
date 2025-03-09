package main

import (
	"encoding/json"
	"os"
	"path/filepath"
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
}
