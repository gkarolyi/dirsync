package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Sync represents a single directory synchronization task
type Sync struct {
	ID              string    `json:"id"`
	SourcePath      string    `json:"source_path"`
	DestinationPath string    `json:"destination_path"`
	IsSyncing       bool      `json:"is_syncing"`
	LastSync        time.Time `json:"last_sync"`
	NextSyncTime    time.Time `json:"next_sync_time"`
	Output          string    `json:"output"`
	LastError       string    `json:"last_error"`
	mu              sync.RWMutex
}

// NewSync creates a new Sync instance
func NewSync(sourcePath, destPath string, interval int) *Sync {
	id := fmt.Sprintf("%s:%s", sourcePath, destPath)
	return &Sync{
		ID:              id,
		SourcePath:      sourcePath,
		DestinationPath: destPath,
		IsSyncing:       false,
		LastSync:        time.Time{},
		NextSyncTime:    time.Now(),
		Output:          "",
		LastError:       "",
	}
}

// Start begins the sync process in a goroutine
func (s *Sync) Start(interval int) {
	go func() {
		for {
			s.mu.RLock()
			nextSync := s.NextSyncTime
			s.mu.RUnlock()

			// Calculate time until next sync
			waitTime := time.Until(nextSync)
			log.Printf("[%s] Next sync in %v", s.ID, waitTime)

			// Wait until next sync time
			time.Sleep(waitTime)

			// Perform the sync
			s.SyncDirectories()

			// Update next sync time
			s.mu.Lock()
			s.NextSyncTime = time.Now().Add(time.Duration(interval) * time.Second)
			s.mu.Unlock()
		}
	}()
}

// TriggerSync triggers an immediate sync
func (s *Sync) TriggerSync() {
	s.mu.Lock()
	s.NextSyncTime = time.Now()
	s.mu.Unlock()
}

// GetStatus returns the current status of the sync
func (s *Sync) GetStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"id":               s.ID,
		"source_path":      s.SourcePath,
		"destination_path": s.DestinationPath,
		"is_syncing":       s.IsSyncing,
		"last_sync":        s.LastSync,
		"next_sync_time":   s.NextSyncTime,
		"output":           s.Output,
		"last_error":       s.LastError,
	}
}

// SyncDirectories synchronizes files from source to destination using rsync
func (s *Sync) SyncDirectories() error {
	// Update status
	s.mu.Lock()
	s.IsSyncing = true
	s.Output = fmt.Sprintf("Starting sync from %s to %s\n", s.SourcePath, s.DestinationPath)
	s.LastError = ""
	s.mu.Unlock()

	log.Printf("[%s] Starting sync from %s to %s using rsync", s.ID, s.SourcePath, s.DestinationPath)

	// Make sure paths exist
	if _, err := os.Stat(s.SourcePath); os.IsNotExist(err) {
		errMsg := fmt.Sprintf("Source path does not exist: %s", s.SourcePath)
		log.Println(errMsg)
		s.setError(errMsg)
		return err
	}

	// Check if source directory is empty
	empty, err := isDirEmpty(s.SourcePath)
	if err != nil {
		errMsg := fmt.Sprintf("Error checking if source directory is empty: %s", err)
		log.Println(errMsg)
		s.setError(errMsg)
		return err
	}

	if empty {
		log.Printf("[%s] Source directory %s is empty, nothing to sync", s.ID, s.SourcePath)
		// Update status
		s.mu.Lock()
		s.IsSyncing = false
		s.LastSync = time.Now()
		s.Output += fmt.Sprintf("\nSource directory %s is empty, nothing to sync", s.SourcePath)
		s.mu.Unlock()
		return nil
	}

	// Create destination if it doesn't exist
	if _, err := os.Stat(s.DestinationPath); os.IsNotExist(err) {
		log.Printf("[%s] Creating destination directory: %s", s.ID, s.DestinationPath)
		if err := os.MkdirAll(s.DestinationPath, 0755); err != nil {
			errMsg := fmt.Sprintf("Failed to create destination directory: %s", err)
			log.Println(errMsg)
			s.setError(errMsg)
			return err
		}

		// Update output
		s.mu.Lock()
		s.Output += fmt.Sprintf("\nCreated destination directory: %s", s.DestinationPath)
		s.mu.Unlock()
	}

	// Check if rsync is available
	_, err = exec.LookPath("rsync")
	if err != nil {
		errMsg := "rsync command not found, falling back to file copy method"
		log.Println(errMsg)
		s.mu.Lock()
		s.Output += "\n" + errMsg
		s.mu.Unlock()
		// Fall back to file copy method
		return s.syncWithFileCopy()
	}

	// Ensure source path ends with a slash to copy contents only
	sourcePath := s.SourcePath
	if !strings.HasSuffix(sourcePath, "/") {
		sourcePath = sourcePath + "/"
	}

	// Prepare rsync command with verbose output
	// -a: archive mode (preserves permissions, timestamps, etc.)
	// -v: verbose
	// -z: compress during transfer
	// -P: show progress
	// Note: --delete flag is NOT used to ensure we don't delete files in destination
	cmd := exec.Command("rsync", "-avzP", sourcePath, s.DestinationPath)

	// Create pipes for stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		errMsg := fmt.Sprintf("Failed to create stdout pipe: %s", err)
		log.Println(errMsg)
		s.setError(errMsg)
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		errMsg := fmt.Sprintf("Failed to create stderr pipe: %s", err)
		log.Println(errMsg)
		s.setError(errMsg)
		return err
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		errMsg := fmt.Sprintf("Failed to start rsync: %s", err)
		log.Println(errMsg)
		s.setError(errMsg)
		return err
	}

	// Create a buffer to store the output
	var outputBuffer strings.Builder
	outputBuffer.WriteString(s.Output) // Include existing output

	// Create a channel to signal when reading is done
	done := make(chan bool)

	// Read stdout in a goroutine
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			outputBuffer.WriteString(line + "\n")

			// Update status with current output
			s.mu.Lock()
			s.Output = outputBuffer.String()
			s.mu.Unlock()

			log.Println("[" + s.ID + "] rsync: " + line)
		}
		done <- true
	}()

	// Read stderr in a goroutine
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			outputBuffer.WriteString("ERROR: " + line + "\n")
			log.Println("[" + s.ID + "] rsync error: " + line)

			// Update status with current output including errors
			s.mu.Lock()
			s.Output = outputBuffer.String()
			s.mu.Unlock()
		}
		done <- true
	}()

	// Wait for both stdout and stderr to be fully read
	<-done
	<-done

	// Wait for the command to finish
	err = cmd.Wait()

	// Get the complete output
	output := outputBuffer.String()

	if err != nil {
		errMsg := fmt.Sprintf("rsync error: %v", err)
		log.Println(errMsg)
		s.setError(errMsg)
		return err
	}

	log.Printf("[%s] rsync completed successfully", s.ID)

	// Update status
	s.mu.Lock()
	s.IsSyncing = false
	s.LastSync = time.Now()
	s.Output = output + "\nSync completed successfully"
	s.mu.Unlock()

	return nil
}

// isDirEmpty checks if a directory is empty
func isDirEmpty(dirPath string) (bool, error) {
	f, err := os.Open(dirPath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	// Read just one entry
	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return true, nil // Directory is empty
	}
	return false, err // Either not empty or error
}

// syncWithFileCopy is a fallback method if rsync is not available
func (s *Sync) syncWithFileCopy() error {
	log.Printf("[%s] Using file copy method for %s to %s", s.ID, s.SourcePath, s.DestinationPath)

	// Get current output
	s.mu.Lock()
	currentOutput := s.Output
	s.mu.Unlock()

	// Create a buffer to store the output
	var outputBuffer strings.Builder
	outputBuffer.WriteString(currentOutput)
	outputBuffer.WriteString(fmt.Sprintf("\nUsing file copy method for %s to %s\n", s.SourcePath, s.DestinationPath))

	// Check if source directory is empty
	empty, err := isDirEmpty(s.SourcePath)
	if err != nil {
		errMsg := fmt.Sprintf("Error checking if source directory is empty: %s", err)
		log.Println(errMsg)
		s.setError(errMsg)
		return err
	}

	if empty {
		log.Printf("[%s] Source directory %s is empty, nothing to sync", s.ID, s.SourcePath)
		// Update status
		s.mu.Lock()
		s.IsSyncing = false
		s.LastSync = time.Now()
		s.Output = outputBuffer.String() + fmt.Sprintf("\nSource directory %s is empty, nothing to sync", s.SourcePath)
		s.mu.Unlock()
		return nil
	}

	// Walk through the source directory
	fileCount := 0

	err = filepath.Walk(s.SourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("[%s] Error accessing path %s: %v", s.ID, path, err)
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(s.SourcePath, path)
		if err != nil {
			log.Printf("[%s] Error getting relative path for %s: %v", s.ID, path, err)
			return err
		}

		// Skip the root directory
		if relPath == "." {
			return nil
		}

		// Construct destination path
		destFilePath := filepath.Join(s.DestinationPath, relPath)

		// If it's a directory, create it in destination
		if info.IsDir() {
			log.Printf("[%s] Creating directory: %s", s.ID, destFilePath)
			outputBuffer.WriteString(fmt.Sprintf("Creating directory: %s\n", relPath))

			// Update output periodically
			s.mu.Lock()
			s.Output = outputBuffer.String()
			s.mu.Unlock()

			return os.MkdirAll(destFilePath, info.Mode())
		}

		// It's a file, so copy it
		log.Printf("[%s] Copying file: %s to %s", s.ID, path, destFilePath)
		outputBuffer.WriteString(fmt.Sprintf("Copying file: %s\n", relPath))

		// Update output periodically
		s.mu.Lock()
		s.Output = outputBuffer.String()
		s.mu.Unlock()

		fileCount++
		return copyFile(path, destFilePath, info.Mode())
	})

	// Update status
	s.mu.Lock()
	s.IsSyncing = false
	s.LastSync = time.Now()
	if err != nil {
		s.LastError = err.Error()
		s.Output = outputBuffer.String() + fmt.Sprintf("\nError: %s", err.Error())
		log.Printf("[%s] Sync error: %v", s.ID, err)
	} else {
		s.Output = outputBuffer.String() + fmt.Sprintf("\nCompleted: %d files copied", fileCount)
		log.Printf("[%s] Sync completed successfully. Copied %d files.", s.ID, fileCount)
	}
	s.mu.Unlock()

	return err
}

// copyFile copies a file from src to dest
func copyFile(src, dest string, mode os.FileMode) error {
	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Create destination file
	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destFile.Close()

	// Copy the content
	if _, err := io.Copy(destFile, srcFile); err != nil {
		return err
	}

	// Set the same permissions
	return os.Chmod(dest, mode)
}

// setError updates the status with an error message
func (s *Sync) setError(errMsg string) {
	s.mu.Lock()
	s.IsSyncing = false
	s.LastError = errMsg
	s.Output += "\nError: " + errMsg
	s.mu.Unlock()
}

// SyncManager manages multiple Sync instances
type SyncManager struct {
	Syncs []*Sync
	mu    sync.RWMutex
}

// NewSyncManager creates a new SyncManager
func NewSyncManager() *SyncManager {
	return &SyncManager{
		Syncs: make([]*Sync, 0),
	}
}

// AddSync adds a new Sync to the manager
func (sm *SyncManager) AddSync(sourcePath, destPath string, interval int) *Sync {
	sync := NewSync(sourcePath, destPath, interval)

	sm.mu.Lock()
	sm.Syncs = append(sm.Syncs, sync)
	sm.mu.Unlock()

	return sync
}

// GetAllStatus returns the status of all syncs
func (sm *SyncManager) GetAllStatus() []map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	statuses := make([]map[string]interface{}, len(sm.Syncs))
	for i, sync := range sm.Syncs {
		statuses[i] = sync.GetStatus()
	}

	return statuses
}

// GetSyncByID returns a sync by its ID
func (sm *SyncManager) GetSyncByID(id string) *Sync {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for _, sync := range sm.Syncs {
		if sync.ID == id {
			return sync
		}
	}

	return nil
}

// TriggerAllSyncs triggers all syncs
func (sm *SyncManager) TriggerAllSyncs() {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for _, sync := range sm.Syncs {
		sync.TriggerSync()
	}
}

// StartSyncProcess starts the synchronization process for all pairs
func StartSyncProcess(syncManager *SyncManager, config *Config) {
	log.Println("Starting sync process")

	// Create a sync for each pair
	for _, pair := range config.SyncPairs {
		parts := strings.Split(pair, ":")
		if len(parts) != 2 {
			log.Printf("Invalid sync pair format: %s", pair)
			continue
		}

		sourcePath := parts[0]
		destPath := parts[1]

		// Create and start a new sync
		sync := syncManager.AddSync(sourcePath, destPath, config.SyncInterval)
		sync.Start(config.SyncInterval)
	}
}
