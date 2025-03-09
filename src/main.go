package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Config holds our JSON configuration
type Config struct {
	SyncInterval int      `json:"sync_interval"`
	SyncPairs    []string `json:"sync_pairs"`
	Port         string   `json:"port"`
}

var (
	config      Config
	baseDir     string
	syncManager *SyncManager
)

func main() {
	// Configure logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting DirSync application")

	// Determine the base directory
	// First try the current directory
	baseDir = "."

	// Load config
	configPath := filepath.Join(baseDir, "config.json")
	log.Printf("Loading configuration from %s", configPath)

	configFile, err := os.ReadFile(configPath)
	if err != nil {
		log.Printf("Error reading config from %s: %v", configPath, err)

		// Try to find config in parent directory
		baseDir = ".."
		configPath = filepath.Join(baseDir, "config.json")
		log.Printf("Trying to load config from %s", configPath)

		configFile, err = os.ReadFile(configPath)
		if err != nil {
			log.Fatalf("Error reading config from %s: %v", configPath, err)
		}

		log.Printf("Found config in parent directory, using %s as base directory", baseDir)
	}

	// Parse the config
	if err := json.Unmarshal(configFile, &config); err != nil {
		log.Fatalf("Error parsing config: %v", err)
	}

	// Adjust sync pairs paths if needed
	for i, pair := range config.SyncPairs {
		parts := strings.Split(pair, ":")
		if len(parts) == 2 {
			// If paths are relative and we're in the src directory,
			// make them relative to the base directory
			if baseDir == ".." {
				if !filepath.IsAbs(parts[0]) && !strings.HasPrefix(parts[0], "..") {
					parts[0] = filepath.Join(baseDir, parts[0])
				}
				if !filepath.IsAbs(parts[1]) && !strings.HasPrefix(parts[1], "..") {
					parts[1] = filepath.Join(baseDir, parts[1])
				}
				config.SyncPairs[i] = parts[0] + ":" + parts[1]
			}
		}
	}

	// Log the loaded configuration
	log.Printf("Loaded configuration: Sync interval: %d seconds, Sync pairs: %v, Port: %s",
		config.SyncInterval, config.SyncPairs, config.Port)

	// Initialize sync manager
	syncManager = NewSyncManager()

	// Start sync process in a goroutine
	go StartSyncProcess(syncManager, &config)

	// Set up routes
	staticDir := filepath.Join(baseDir, "static")
	log.Printf("Serving static files from: %s", staticDir)

	// Check if static directory exists
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		log.Fatalf("Static directory not found: %s", staticDir)
	}

	http.Handle("/", http.FileServer(http.Dir(staticDir)))
	http.HandleFunc("/status", handleStatus)
	http.HandleFunc("/api/sync/now", handleSyncNow)
	http.HandleFunc("/api/sync/details", handleSyncDetails)

	// Start server
	port := config.Port
	if port == "" {
		port = ":8080"
	}
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	log.Printf("Starting server on http://localhost%s", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	statuses := syncManager.GetAllStatus()

	if err := json.NewEncoder(w).Encode(statuses); err != nil {
		log.Printf("Error encoding status: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// handleSyncNow triggers an immediate sync
func handleSyncNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	log.Println("Manual sync triggered")

	// Trigger all syncs
	syncManager.TriggerAllSyncs()

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"success": true, "message": "Sync triggered"}`)
}

// handleSyncDetails returns details for a specific sync
func handleSyncDetails(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing sync ID", http.StatusBadRequest)
		return
	}

	sync := syncManager.GetSyncByID(id)
	if sync == nil {
		http.Error(w, "Sync not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(sync.GetStatus()); err != nil {
		log.Printf("Error encoding sync details: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}
