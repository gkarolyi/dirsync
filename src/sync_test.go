package main

import (
	"os"
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

	// Clean the destination directory
	os.RemoveAll(destDir)
	os.MkdirAll(destDir, 0755)

	// Create a test sync
	testSync := NewSync(sourceDir, destDir, 60)

	// Perform the sync
	err := testSync.SyncDirectories()
	if err != nil {
		t.Fatalf("SyncDirectories failed: %v", err)
	}

	// Verify all files were copied
	for _, tf := range testFiles {
		destPath := filepath.Join(destDir, tf.path)
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			t.Errorf("File %s was not copied to destination", tf.path)
			continue
		}

		// Check file content
		content, err := os.ReadFile(destPath)
		if err != nil {
			t.Errorf("Failed to read destination file %s: %v", tf.path, err)
			continue
		}

		if string(content) != tf.content {
			t.Errorf("File %s content mismatch. Expected: %s, Got: %s", tf.path, tf.content, string(content))
		}
	}

	// Verify sync status was updated
	if testSync.IsSyncing {
		t.Errorf("IsSyncing should be false after sync, got true")
	}

	if testSync.LastSync.IsZero() {
		t.Errorf("LastSync should be set after sync, got zero time")
	}

	// Test with non-existent source directory
	nonExistentSync := NewSync("/non/existent/path", destDir, 60)
	err = nonExistentSync.SyncDirectories()
	if err == nil {
		t.Errorf("Expected error for non-existent source directory, got nil")
	}

	if nonExistentSync.LastError == "" {
		t.Errorf("Expected LastError to be set for non-existent source directory")
	}

	// Test with non-existent destination directory (should be created)
	newDestDir := filepath.Join(os.TempDir(), "dirsync_test_new_dest")
	os.RemoveAll(newDestDir) // Ensure it doesn't exist

	newDestSync := NewSync(sourceDir, newDestDir, 60)
	err = newDestSync.SyncDirectories()
	if err != nil {
		t.Fatalf("SyncDirectories failed with new destination: %v", err)
	}

	// Verify destination was created
	if _, err := os.Stat(newDestDir); os.IsNotExist(err) {
		t.Errorf("Destination directory was not created")
	} else {
		// Clean up
		os.RemoveAll(newDestDir)
	}
}

// TestEmptySourceDirectory tests syncing an empty source directory
func TestEmptySourceDirectory(t *testing.T) {
	// Create empty source directory
	emptySourceDir := filepath.Join(os.TempDir(), "dirsync_test_empty_source")
	os.RemoveAll(emptySourceDir)
	os.MkdirAll(emptySourceDir, 0755)
	defer os.RemoveAll(emptySourceDir)

	// Create destination directory
	emptyDestDir := filepath.Join(os.TempDir(), "dirsync_test_empty_dest")
	os.RemoveAll(emptyDestDir)
	os.MkdirAll(emptyDestDir, 0755)
	defer os.RemoveAll(emptyDestDir)

	// Create a test sync
	testSync := NewSync(emptySourceDir, emptyDestDir, 60)

	// Perform the sync
	err := testSync.SyncDirectories()
	if err != nil {
		t.Fatalf("SyncDirectories failed with empty source: %v", err)
	}

	// Verify sync status was updated
	if testSync.IsSyncing {
		t.Errorf("IsSyncing should be false after sync, got true")
	}

	if testSync.LastSync.IsZero() {
		t.Errorf("LastSync should be set after sync, got zero time")
	}

	// Create a file in the destination to test that it's not deleted
	testFile := filepath.Join(emptyDestDir, "test.txt")
	testContent := "This is a test file"
	err = os.WriteFile(testFile, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Sync again
	err = testSync.SyncDirectories()
	if err != nil {
		t.Fatalf("SyncDirectories failed on second sync: %v", err)
	}

	// Verify the file still exists in destination
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Errorf("Test file was deleted from destination")
	} else {
		// Check content
		content, err := os.ReadFile(testFile)
		if err != nil {
			t.Errorf("Failed to read test file: %v", err)
		} else if string(content) != testContent {
			t.Errorf("Test file content was modified. Expected: %s, Got: %s", testContent, string(content))
		}
	}
}

// TestCopyFile tests the copyFile function
func TestCopyFile(t *testing.T) {
	// Create a test file
	sourceFile := filepath.Join(os.TempDir(), "dirsync_test_copy_source.txt")
	destFile := filepath.Join(os.TempDir(), "dirsync_test_copy_dest.txt")

	// Clean up any existing files
	os.Remove(sourceFile)
	os.Remove(destFile)

	// Create source file with test content
	testContent := "This is a test file for copyFile function"
	err := os.WriteFile(sourceFile, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}
	defer os.Remove(sourceFile)

	// Get source file info
	sourceInfo, err := os.Stat(sourceFile)
	if err != nil {
		t.Fatalf("Failed to get source file info: %v", err)
	}

	// Copy the file
	err = copyFile(sourceFile, destFile, sourceInfo.Mode())
	if err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}
	defer os.Remove(destFile)

	// Verify the file was copied
	if _, err := os.Stat(destFile); os.IsNotExist(err) {
		t.Errorf("Destination file was not created")
	}

	// Check content
	content, err := os.ReadFile(destFile)
	if err != nil {
		t.Errorf("Failed to read destination file: %v", err)
	} else if string(content) != testContent {
		t.Errorf("File content mismatch. Expected: %s, Got: %s", testContent, string(content))
	}

	// Check permissions
	destInfo, err := os.Stat(destFile)
	if err != nil {
		t.Errorf("Failed to get destination file info: %v", err)
	} else if destInfo.Mode() != sourceInfo.Mode() {
		t.Errorf("File mode mismatch. Expected: %v, Got: %v", sourceInfo.Mode(), destInfo.Mode())
	}
}

// TestSyncWithFileCopy tests the syncWithFileCopy method
func TestSyncWithFileCopy(t *testing.T) {
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

	// Clean the destination directory
	os.RemoveAll(destDir)
	os.MkdirAll(destDir, 0755)

	// Create a test sync
	testSync := NewSync(sourceDir, destDir, 60)

	// Perform the sync with file copy
	err := testSync.syncWithFileCopy()
	if err != nil {
		t.Fatalf("syncWithFileCopy failed: %v", err)
	}

	// Verify all files were copied
	for _, tf := range testFiles {
		destPath := filepath.Join(destDir, tf.path)
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			t.Errorf("File %s was not copied to destination", tf.path)
			continue
		}

		// Check file content
		content, err := os.ReadFile(destPath)
		if err != nil {
			t.Errorf("Failed to read destination file %s: %v", tf.path, err)
			continue
		}

		if string(content) != tf.content {
			t.Errorf("File %s content mismatch. Expected: %s, Got: %s", tf.path, tf.content, string(content))
		}
	}

	// Verify sync status was updated
	if testSync.IsSyncing {
		t.Errorf("IsSyncing should be false after sync, got true")
	}

	if testSync.LastSync.IsZero() {
		t.Errorf("LastSync should be set after sync, got zero time")
	}
}

// TestIsDirEmpty tests the isDirEmpty function
func TestIsDirEmpty(t *testing.T) {
	// Create a test directory
	testDir := filepath.Join(os.TempDir(), "dirsync_test_empty_dir")
	os.RemoveAll(testDir)
	os.MkdirAll(testDir, 0755)
	defer os.RemoveAll(testDir)

	// Test empty directory
	empty, err := isDirEmpty(testDir)
	if err != nil {
		t.Fatalf("isDirEmpty failed: %v", err)
	}
	if !empty {
		t.Errorf("Expected empty directory, got non-empty")
	}

	// Add a file to the directory
	testFile := filepath.Join(testDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test non-empty directory
	empty, err = isDirEmpty(testDir)
	if err != nil {
		t.Fatalf("isDirEmpty failed: %v", err)
	}
	if empty {
		t.Errorf("Expected non-empty directory, got empty")
	}

	// Test non-existent directory
	_, err = isDirEmpty("/non/existent/path")
	if err == nil {
		t.Errorf("Expected error for non-existent directory, got nil")
	}
}

// TestSetError tests the setError method
func TestSetError(t *testing.T) {
	// Create a test sync
	testSync := NewSync(testSourceDir, testDestDir, 60)

	// Set initial state
	testSync.IsSyncing = true
	testSync.LastError = ""
	testSync.Output = "Initial output"

	// Call setError
	testErrorMsg := "Test error message"
	testSync.setError(testErrorMsg)

	// Verify error was set
	if testSync.IsSyncing {
		t.Errorf("IsSyncing should be false after setError, got true")
	}

	if testSync.LastError != testErrorMsg {
		t.Errorf("LastError not set correctly. Expected: %s, Got: %s", testErrorMsg, testSync.LastError)
	}

	if !strings.Contains(testSync.Output, testErrorMsg) {
		t.Errorf("Output should contain error message. Output: %s, Error: %s", testSync.Output, testErrorMsg)
	}
}

// TestSyncManager tests the SyncManager functionality
func TestSyncManager(t *testing.T) {
	// Create a test sync manager
	manager := NewSyncManager()

	// Verify it's initialized correctly
	if len(manager.Syncs) != 0 {
		t.Errorf("Expected empty Syncs slice, got %d items", len(manager.Syncs))
	}

	// Add a sync
	sync1 := manager.AddSync(testSourceDir, testDestDir, 60)

	// Verify sync was added
	if len(manager.Syncs) != 1 {
		t.Errorf("Expected 1 sync, got %d", len(manager.Syncs))
	}

	if sync1.ID != testSourceDir+":"+testDestDir {
		t.Errorf("Sync ID not set correctly. Expected: %s, Got: %s", testSourceDir+":"+testDestDir, sync1.ID)
	}

	// Add another sync
	sync2 := manager.AddSync("/another/source", "/another/dest", 30)

	// Verify second sync was added
	if len(manager.Syncs) != 2 {
		t.Errorf("Expected 2 syncs, got %d", len(manager.Syncs))
	}

	// Test GetSyncByID
	foundSync := manager.GetSyncByID(sync1.ID)
	if foundSync != sync1 {
		t.Errorf("GetSyncByID returned wrong sync")
	}

	// Test with non-existent ID
	foundSync = manager.GetSyncByID("non-existent")
	if foundSync != nil {
		t.Errorf("GetSyncByID should return nil for non-existent ID")
	}

	// Test GetAllStatus
	statuses := manager.GetAllStatus()
	if len(statuses) != 2 {
		t.Errorf("Expected 2 statuses, got %d", len(statuses))
	}

	// Test TriggerAllSyncs
	// Set initial next sync times
	initialTime1 := time.Now().Add(60 * time.Second)
	initialTime2 := time.Now().Add(30 * time.Second)
	sync1.NextSyncTime = initialTime1
	sync2.NextSyncTime = initialTime2

	// Trigger all syncs
	manager.TriggerAllSyncs()

	// Verify next sync times were updated
	if !sync1.NextSyncTime.Before(initialTime1) {
		t.Errorf("NextSyncTime for sync1 was not updated")
	}

	if !sync2.NextSyncTime.Before(initialTime2) {
		t.Errorf("NextSyncTime for sync2 was not updated")
	}
}
