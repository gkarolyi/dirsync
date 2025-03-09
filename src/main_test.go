package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// TestConfigLoading tests the loading and parsing of the config file
func TestConfigLoading(t *testing.T) {
	// Use the test_config.json created in setup
	configFile, err := os.ReadFile("test_config.json")
	if err != nil {
		t.Fatalf("Error reading config: %v", err)
	}

	var testConfig Config
	if err := json.Unmarshal(configFile, &testConfig); err != nil {
		t.Fatalf("Error parsing config: %v", err)
	}

	// Verify the config was loaded correctly
	if testConfig.SyncInterval != 5 {
		t.Errorf("Expected SyncInterval %d, got %d", 5, testConfig.SyncInterval)
	}

	if len(testConfig.SyncPairs) != 1 {
		t.Errorf("Expected %d SyncPairs, got %d", 1, len(testConfig.SyncPairs))
	} else {
		expectedPair := testSourceDir + ":" + testDestDir
		if testConfig.SyncPairs[0] != expectedPair {
			t.Errorf("Expected SyncPair %s, got %s", expectedPair, testConfig.SyncPairs[0])
		}
	}

	if testConfig.Port != ":8090" {
		t.Errorf("Expected Port %s, got %s", ":8090", testConfig.Port)
	}
}

// TestSyncInitialization tests the initialization of the Sync struct
func TestSyncInitialization(t *testing.T) {
	// Load the test config
	configFile, err := os.ReadFile("test_config.json")
	if err != nil {
		t.Fatalf("Error reading config: %v", err)
	}

	if err := json.Unmarshal(configFile, &config); err != nil {
		t.Fatalf("Error parsing config: %v", err)
	}

	// Initialize a test sync
	sourcePath := testSourceDir
	destPath := testDestDir
	interval := config.SyncInterval

	testSync := NewSync(sourcePath, destPath, interval)

	// Verify sync initialization
	if testSync.IsSyncing != false {
		t.Errorf("Expected IsSyncing to be false, got %v", testSync.IsSyncing)
	}

	if testSync.SourcePath != sourcePath {
		t.Errorf("Expected SourcePath %s, got %s", sourcePath, testSync.SourcePath)
	}

	if testSync.DestinationPath != destPath {
		t.Errorf("Expected DestinationPath %s, got %s", destPath, testSync.DestinationPath)
	}

	expectedID := sourcePath + ":" + destPath
	if testSync.ID != expectedID {
		t.Errorf("Expected ID %s, got %s", expectedID, testSync.ID)
	}

	// Allow a small time difference due to execution time
	timeDiff := testSync.NextSyncTime.Sub(time.Now())
	// The NextSyncTime is now set to time.Now() for immediate first sync
	if timeDiff < -5*time.Second || timeDiff > 5*time.Second {
		t.Errorf("Expected NextSyncTime to be close to current time, but difference is %v", timeDiff)
	}
}

// TestHandleStatus tests the status HTTP handler
func TestHandleStatus(t *testing.T) {
	// Set up test sync manager
	testSyncManager := NewSyncManager()
	syncManager = testSyncManager

	// Add a test sync
	testTime := time.Now()
	testSync := &Sync{
		ID:              testSourceDir + ":" + testDestDir,
		SourcePath:      testSourceDir,
		DestinationPath: testDestDir,
		IsSyncing:       true,
		LastSync:        testTime,
		NextSyncTime:    testTime.Add(60 * time.Second),
		Output:          "test output",
		LastError:       "test error",
	}

	testSyncManager.Syncs = append(testSyncManager.Syncs, testSync)

	// Create a request to pass to our handler
	req, err := http.NewRequest("GET", "/status", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create a ResponseRecorder to record the response
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(handleStatus)

	// Call the handler
	handler.ServeHTTP(rr, req)

	// Check the status code
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	// Check the content type
	expectedContentType := "application/json"
	if contentType := rr.Header().Get("Content-Type"); contentType != expectedContentType {
		t.Errorf("handler returned wrong content type: got %v want %v", contentType, expectedContentType)
	}

	// Check the response body
	var responseStatuses []map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&responseStatuses); err != nil {
		t.Fatalf("Failed to decode response body: %v", err)
	}

	if len(responseStatuses) != 1 {
		t.Fatalf("Expected 1 sync status, got %d", len(responseStatuses))
	}

	responseStatus := responseStatuses[0]

	if isSyncing, ok := responseStatus["is_syncing"].(bool); !ok || isSyncing != testSync.IsSyncing {
		t.Errorf("Expected IsSyncing %v, got %v", testSync.IsSyncing, responseStatus["is_syncing"])
	}

	if id, ok := responseStatus["id"].(string); !ok || id != testSync.ID {
		t.Errorf("Expected ID %s, got %s", testSync.ID, responseStatus["id"])
	}

	if lastError, ok := responseStatus["last_error"].(string); !ok || lastError != testSync.LastError {
		t.Errorf("Expected LastError %s, got %s", testSync.LastError, responseStatus["last_error"])
	}
}

// TestHandleSyncNow tests the manual sync trigger endpoint
func TestHandleSyncNow(t *testing.T) {
	// Set up test sync manager
	testSyncManager := NewSyncManager()
	syncManager = testSyncManager

	// Add a test sync
	initialTime := time.Now().Add(60 * time.Second) // Next sync in 60 seconds
	testSync := &Sync{
		ID:              testSourceDir + ":" + testDestDir,
		SourcePath:      testSourceDir,
		DestinationPath: testDestDir,
		IsSyncing:       false,
		LastSync:        time.Now().Add(-60 * time.Second), // Last sync was 60 seconds ago
		NextSyncTime:    initialTime,
		Output:          "",
		LastError:       "",
	}

	testSyncManager.Syncs = append(testSyncManager.Syncs, testSync)

	// Create a POST request to the sync now endpoint
	req, err := http.NewRequest("POST", "/api/sync/now", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create a ResponseRecorder to record the response
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(handleSyncNow)

	// Call the handler
	handler.ServeHTTP(rr, req)

	// Check the status code
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	// Check the content type
	expectedContentType := "application/json"
	if contentType := rr.Header().Get("Content-Type"); contentType != expectedContentType {
		t.Errorf("handler returned wrong content type: got %v want %v", contentType, expectedContentType)
	}

	// Check the response body
	var response map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response body: %v", err)
	}

	// Verify the response contains success: true
	if success, ok := response["success"].(bool); !ok || !success {
		t.Errorf("Expected success: true in response, got %v", response)
	}

	// Verify the next sync time was updated to now (or very close to now)
	if time.Since(testSync.NextSyncTime) > 2*time.Second {
		t.Errorf("Expected NextSyncTime to be updated to now, but it's %v in the past", time.Since(testSync.NextSyncTime))
	}

	// Test with wrong HTTP method
	req, err = http.NewRequest("GET", "/api/sync/now", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Should return method not allowed
	if status := rr.Code; status != http.StatusMethodNotAllowed {
		t.Errorf("handler should return method not allowed for GET, got %v", status)
	}
}

// TestHandleSyncDetails tests the sync details endpoint
func TestHandleSyncDetails(t *testing.T) {
	// Set up test sync manager
	testSyncManager := NewSyncManager()
	syncManager = testSyncManager

	// Add a test sync
	testSync := &Sync{
		ID:              testSourceDir + ":" + testDestDir,
		SourcePath:      testSourceDir,
		DestinationPath: testDestDir,
		IsSyncing:       false,
		LastSync:        time.Now(),
		NextSyncTime:    time.Now().Add(60 * time.Second),
		Output:          "test output content",
		LastError:       "",
	}

	testSyncManager.Syncs = append(testSyncManager.Syncs, testSync)

	// Create a request to the sync details endpoint
	req, err := http.NewRequest("GET", "/api/sync/details?id="+testSync.ID, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create a ResponseRecorder to record the response
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(handleSyncDetails)

	// Call the handler
	handler.ServeHTTP(rr, req)

	// Check the status code
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	// Check the content type
	expectedContentType := "application/json"
	if contentType := rr.Header().Get("Content-Type"); contentType != expectedContentType {
		t.Errorf("handler returned wrong content type: got %v want %v", contentType, expectedContentType)
	}

	// Check the response body
	var responseStatus map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&responseStatus); err != nil {
		t.Fatalf("Failed to decode response body: %v", err)
	}

	if output, ok := responseStatus["output"].(string); !ok || output != testSync.Output {
		t.Errorf("Expected Output %s, got %s", testSync.Output, responseStatus["output"])
	}

	// Test with missing ID
	req, err = http.NewRequest("GET", "/api/sync/details", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Should return bad request
	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler should return bad request for missing ID, got %v", status)
	}

	// Test with non-existent ID
	req, err = http.NewRequest("GET", "/api/sync/details?id=nonexistent", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Should return not found
	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("handler should return not found for non-existent ID, got %v", status)
	}
}

// TestIntegration performs an integration test of the entire application flow
func TestIntegration(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Set up a test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			handleStatus(w, r)
		} else if r.URL.Path == "/api/sync/details" {
			handleSyncDetails(w, r)
		}
	}))
	defer ts.Close()

	// Load the config
	configFile, err := os.ReadFile("test_config.json")
	if err != nil {
		t.Fatalf("Error reading config: %v", err)
	}

	if err := json.Unmarshal(configFile, &config); err != nil {
		t.Fatalf("Error parsing config: %v", err)
	}

	// Initialize sync manager
	testSyncManager := NewSyncManager()
	syncManager = testSyncManager

	// Add a test sync
	testSync := NewSync(testSourceDir, testDestDir, config.SyncInterval)
	testSyncManager.Syncs = append(testSyncManager.Syncs, testSync)

	// Make a request to the status endpoint
	resp, err := http.Get(ts.URL + "/status")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check the response
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	var responseStatuses []map[string]interface{}
	if err := json.Unmarshal(body, &responseStatuses); err != nil {
		t.Fatalf("Failed to unmarshal response body: %v", err)
	}

	if len(responseStatuses) != 1 {
		t.Fatalf("Expected 1 sync status, got %d", len(responseStatuses))
	}

	responseStatus := responseStatuses[0]

	if id, ok := responseStatus["id"].(string); !ok || id != testSync.ID {
		t.Errorf("Expected ID %s, got %s", testSync.ID, responseStatus["id"])
	}
}
