package main

import (
	"os"
	"path/filepath"
	"testing"
)

var (
	// Test directories
	testSourceDir string
	testDestDir   string
)

// TestMain is used for test setup and teardown
func TestMain(m *testing.M) {
	// Setup test environment
	setup()

	// Run tests
	code := m.Run()

	// Cleanup
	teardown()

	// Exit with test result code
	os.Exit(code)
}

// setup prepares the test environment
func setup() {
	// Create test directories
	var err error
	testSourceDir, err = os.MkdirTemp("", "test_source")
	if err != nil {
		panic("Failed to create test source directory: " + err.Error())
	}

	testDestDir, err = os.MkdirTemp("", "test_dest")
	if err != nil {
		panic("Failed to create test destination directory: " + err.Error())
	}

	// Create test files
	createTestFiles()

	// Create test config
	createTestConfig()
}

// teardown cleans up the test environment
func teardown() {
	// Remove test directories
	os.RemoveAll(testSourceDir)
	os.RemoveAll(testDestDir)

	// Remove test config
	os.Remove("test_config.json")
}

// createTestFiles creates test files in the source directory
func createTestFiles() {
	// Create some test files
	testFiles := []struct {
		path    string
		content string
	}{
		{"file1.txt", "Test file 1 content"},
		{"file2.txt", "Test file 2 content"},
		{"subdir/file3.txt", "Test file in subdirectory"},
	}

	for _, tf := range testFiles {
		fullPath := filepath.Join(testSourceDir, tf.path)

		// Create directory if needed
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			panic("Failed to create directory " + dir + ": " + err.Error())
		}

		// Create file
		if err := os.WriteFile(fullPath, []byte(tf.content), 0644); err != nil {
			panic("Failed to create test file " + fullPath + ": " + err.Error())
		}
	}
}

// createTestConfig creates a test configuration file
func createTestConfig() {
	// Create a test config file with our temp directories
	configContent := `{
  "sync_interval": 5,
  "sync_pairs": ["` + testSourceDir + `:` + testDestDir + `"],
  "port": ":8090"
}`

	if err := os.WriteFile("test_config.json", []byte(configContent), 0644); err != nil {
		panic("Failed to create test config: " + err.Error())
	}
}
