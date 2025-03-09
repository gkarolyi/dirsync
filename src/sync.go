package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SyncDirectories synchronizes files from source to destination
func SyncDirectories(sourcePath, destPath string) error {
	// Update status
	mu.Lock()
	status.IsSyncing = true
	status.CurrentPair = fmt.Sprintf("%s:%s", sourcePath, destPath)
	status.LastError = ""
	mu.Unlock()

	log.Printf("Starting sync from %s to %s", sourcePath, destPath)

	// Make sure paths exist
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		errMsg := fmt.Sprintf("Source path does not exist: %s", sourcePath)
		log.Println(errMsg)
		setError(errMsg)
		return err
	}

	// Create destination if it doesn't exist
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		log.Printf("Creating destination directory: %s", destPath)
		if err := os.MkdirAll(destPath, 0755); err != nil {
			errMsg := fmt.Sprintf("Failed to create destination directory: %s", err)
			log.Println(errMsg)
			setError(errMsg)
			return err
		}
	}

	// Walk through the source directory
	fileCount := 0
	err := filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("Error accessing path %s: %v", path, err)
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(sourcePath, path)
		if err != nil {
			log.Printf("Error getting relative path for %s: %v", path, err)
			return err
		}

		// Skip the root directory
		if relPath == "." {
			return nil
		}

		// Construct destination path
		destFilePath := filepath.Join(destPath, relPath)

		// If it's a directory, create it in destination
		if info.IsDir() {
			log.Printf("Creating directory: %s", destFilePath)
			return os.MkdirAll(destFilePath, info.Mode())
		}

		// It's a file, so copy it
		log.Printf("Copying file: %s to %s", path, destFilePath)
		fileCount++
		return copyFile(path, destFilePath, info.Mode())
	})

	// Update status
	mu.Lock()
	status.IsSyncing = false
	status.LastSync = time.Now()
	status.NextSyncTime = time.Now().Add(time.Duration(config.SyncInterval) * time.Second)
	if err != nil {
		status.LastError = err.Error()
		log.Printf("Sync error: %v", err)
	} else {
		log.Printf("Sync completed successfully. Copied %d files.", fileCount)
	}
	mu.Unlock()

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
func setError(errMsg string) {
	mu.Lock()
	status.IsSyncing = false
	status.LastError = errMsg
	mu.Unlock()
}

// syncWithRsync uses rsync to sync files from source to destination
// This is a placeholder for future implementation
func syncWithRsync(sourcePath, destPath string) error {
	// Update status
	mu.Lock()
	status.IsSyncing = true
	status.CurrentPair = fmt.Sprintf("%s:%s", sourcePath, destPath)
	status.LastError = ""
	mu.Unlock()

	log.Printf("Starting rsync from %s to %s", sourcePath, destPath)

	// Check if rsync is available
	_, err := exec.LookPath("rsync")
	if err != nil {
		errMsg := "rsync command not found"
		log.Println(errMsg)
		setError(errMsg)
		return err
	}

	// Prepare rsync command
	// -a: archive mode (preserves permissions, timestamps, etc.)
	// -v: verbose
	// -z: compress during transfer
	// --delete: delete files in destination that don't exist in source
	cmd := exec.Command("rsync", "-avz", "--delete", sourcePath+"/", destPath)

	// Capture output
	output, err := cmd.CombinedOutput()
	if err != nil {
		errMsg := fmt.Sprintf("rsync error: %v - %s", err, string(output))
		log.Println(errMsg)
		setError(errMsg)
		return err
	}

	log.Printf("rsync output: %s", string(output))

	// Update status
	mu.Lock()
	status.IsSyncing = false
	status.LastSync = time.Now()
	status.NextSyncTime = time.Now().Add(time.Duration(config.SyncInterval) * time.Second)
	mu.Unlock()

	return nil
}

// StartSyncProcess starts the synchronization process for all pairs
func StartSyncProcess() {
	log.Println("Starting sync process")

	for {
		mu.RLock()
		nextSync := status.NextSyncTime
		mu.RUnlock()

		// Calculate time until next sync
		waitTime := time.Until(nextSync)
		log.Printf("Next sync in %v", waitTime)

		// Wait until next sync time
		time.Sleep(waitTime)

		log.Println("Starting sync cycle")

		// Sync all pairs
		for _, pair := range config.SyncPairs {
			parts := strings.Split(pair, ":")
			if len(parts) != 2 {
				errMsg := fmt.Sprintf("Invalid sync pair format: %s", pair)
				log.Println(errMsg)
				setError(errMsg)
				continue
			}

			sourcePath := parts[0]
			destPath := parts[1]

			// For now, use the file-based sync method
			// In the future, this could be switched to rsync
			if err := SyncDirectories(sourcePath, destPath); err != nil {
				// Error is already set in SyncDirectories
				continue
			}

			// Example of how to use rsync in the future:
			// if err := syncWithRsync(sourcePath, destPath); err != nil {
			//     continue
			// }
		}

		log.Println("Sync cycle completed")

		// Update next sync time
		mu.Lock()
		status.NextSyncTime = time.Now().Add(time.Duration(config.SyncInterval) * time.Second)
		mu.Unlock()
	}
}
