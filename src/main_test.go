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

// TestStatusInitialization tests the initialization of the status struct
func TestStatusInitialization(t *testing.T) {
	// Load the test config
	configFile, err := os.ReadFile("test_config.json")
	if err != nil {
		t.Fatalf("Error reading config: %v", err)
	}

	if err := json.Unmarshal(configFile, &config); err != nil {
		t.Fatalf("Error parsing config: %v", err)
	}

	// Initialize status with the test config
	now := time.Now()
	testStatus := Status{
		IsSyncing:    false,
		LastSync:     now,
		NextSyncTime: now.Add(time.Duration(config.SyncInterval) * time.Second),
	}

	// Verify status initialization
	if testStatus.IsSyncing != false {
		t.Errorf("Expected IsSyncing to be false, got %v", testStatus.IsSyncing)
	}

	// Allow a small time difference due to execution time
	timeDiff := testStatus.NextSyncTime.Sub(testStatus.LastSync)
	expectedDiff := time.Duration(config.SyncInterval) * time.Second
	if timeDiff < expectedDiff-time.Second || timeDiff > expectedDiff+time.Second {
		t.Errorf("Expected time difference of %v, got %v", expectedDiff, timeDiff)
	}
}

// TestHandleStatus tests the status HTTP handler
func TestHandleStatus(t *testing.T) {
	// Set up test status
	testTime := time.Now()
	status = Status{
		IsSyncing:    true,
		LastSync:     testTime,
		NextSyncTime: testTime.Add(60 * time.Second),
		CurrentPair:  testSourceDir + ":" + testDestDir,
		LastError:    "test error",
	}

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
	var responseStatus Status
	if err := json.NewDecoder(rr.Body).Decode(&responseStatus); err != nil {
		t.Fatalf("Failed to decode response body: %v", err)
	}

	if responseStatus.IsSyncing != status.IsSyncing {
		t.Errorf("Expected IsSyncing %v, got %v", status.IsSyncing, responseStatus.IsSyncing)
	}

	if responseStatus.CurrentPair != status.CurrentPair {
		t.Errorf("Expected CurrentPair %s, got %s", status.CurrentPair, responseStatus.CurrentPair)
	}

	if responseStatus.LastError != status.LastError {
		t.Errorf("Expected LastError %s, got %s", status.LastError, responseStatus.LastError)
	}
}

// TestHandleSyncNow tests the manual sync trigger endpoint
func TestHandleSyncNow(t *testing.T) {
	// Set up initial status
	initialTime := time.Now().Add(60 * time.Second) // Next sync in 60 seconds
	status = Status{
		IsSyncing:    false,
		LastSync:     time.Now().Add(-60 * time.Second), // Last sync was 60 seconds ago
		NextSyncTime: initialTime,
		CurrentPair:  "",
		LastError:    "",
	}

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
	if time.Since(status.NextSyncTime) > 2*time.Second {
		t.Errorf("Expected NextSyncTime to be updated to now, but it's %v in the past", time.Since(status.NextSyncTime))
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

	// Initialize status
	status = Status{
		IsSyncing:    false,
		LastSync:     time.Now(),
		NextSyncTime: time.Now().Add(time.Duration(config.SyncInterval) * time.Second),
	}

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

	var responseStatus Status
	if err := json.Unmarshal(body, &responseStatus); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if responseStatus.IsSyncing != status.IsSyncing {
		t.Errorf("Expected IsSyncing %v, got %v", status.IsSyncing, responseStatus.IsSyncing)
	}
}
