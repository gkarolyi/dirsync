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
	"time"
)

// SyncDirectories synchronizes files from source to destination using rsync
func SyncDirectories(sourcePath, destPath string) error {
	// Update status
	mu.Lock()
	status.IsSyncing = true
	status.CurrentPair = fmt.Sprintf("%s:%s", sourcePath, destPath)
	status.SourcePath = sourcePath
	status.DestinationPath = destPath
	status.LastError = ""
	mu.Unlock()

	log.Printf("Starting sync from %s to %s using rsync", sourcePath, destPath)

	// Make sure paths exist
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		errMsg := fmt.Sprintf("Source path does not exist: %s", sourcePath)
		log.Println(errMsg)
		setError(errMsg)
		return err
	}

	// Check if source directory is empty
	empty, err := isDirEmpty(sourcePath)
	if err != nil {
		errMsg := fmt.Sprintf("Error checking if source directory is empty: %s", err)
		log.Println(errMsg)
		setError(errMsg)
		return err
	}

	if empty {
		log.Printf("Source directory %s is empty, nothing to sync", sourcePath)
		// Update status
		mu.Lock()
		status.IsSyncing = false
		status.LastSync = time.Now()
		status.NextSyncTime = time.Now().Add(time.Duration(config.SyncInterval) * time.Second)
		mu.Unlock()
		return nil
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

	// Check if rsync is available
	_, err = exec.LookPath("rsync")
	if err != nil {
		errMsg := "rsync command not found, falling back to file copy method"
		log.Println(errMsg)
		// Fall back to file copy method
		return syncWithFileCopy(sourcePath, destPath)
	}

	// Ensure source path ends with a slash to copy contents only
	if !strings.HasSuffix(sourcePath, "/") {
		sourcePath = sourcePath + "/"
	}

	// Prepare rsync command with verbose output
	// -a: archive mode (preserves permissions, timestamps, etc.)
	// -v: verbose
	// -z: compress during transfer
	// -P: show progress
	// Note: --delete flag is NOT used to ensure we don't delete files in destination
	cmd := exec.Command("rsync", "-avzP", sourcePath, destPath)

	// Create pipes for stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		errMsg := fmt.Sprintf("Failed to create stdout pipe: %s", err)
		log.Println(errMsg)
		setError(errMsg)
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		errMsg := fmt.Sprintf("Failed to create stderr pipe: %s", err)
		log.Println(errMsg)
		setError(errMsg)
		return err
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		errMsg := fmt.Sprintf("Failed to start rsync: %s", err)
		log.Println(errMsg)
		setError(errMsg)
		return err
	}

	// Create a buffer to store the output
	var outputBuffer strings.Builder

	// Create a channel to signal when reading is done
	done := make(chan bool)

	// Read stdout in a goroutine
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			outputBuffer.WriteString(line + "\n")

			// Update status with current output
			mu.Lock()
			status.CurrentPair = line
			mu.Unlock()

			log.Println("rsync: " + line)
		}
		done <- true
	}()

	// Read stderr in a goroutine
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			outputBuffer.WriteString("ERROR: " + line + "\n")
			log.Println("rsync error: " + line)
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
		errMsg := fmt.Sprintf("rsync error: %v - %s", err, output)
		log.Println(errMsg)
		setError(errMsg)
		return err
	}

	log.Printf("rsync completed successfully")

	// Update status
	mu.Lock()
	status.IsSyncing = false
	status.LastSync = time.Now()
	status.NextSyncTime = time.Now().Add(time.Duration(config.SyncInterval) * time.Second)
	status.CurrentPair = "Sync completed"
	mu.Unlock()

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
func syncWithFileCopy(sourcePath, destPath string) error {
	log.Printf("Using file copy method for %s to %s", sourcePath, destPath)

	// Update source and destination paths in status
	mu.Lock()
	status.SourcePath = sourcePath
	status.DestinationPath = destPath
	mu.Unlock()

	// Check if source directory is empty
	empty, err := isDirEmpty(sourcePath)
	if err != nil {
		errMsg := fmt.Sprintf("Error checking if source directory is empty: %s", err)
		log.Println(errMsg)
		setError(errMsg)
		return err
	}

	if empty {
		log.Printf("Source directory %s is empty, nothing to sync", sourcePath)
		// Update status
		mu.Lock()
		status.IsSyncing = false
		status.LastSync = time.Now()
		status.NextSyncTime = time.Now().Add(time.Duration(config.SyncInterval) * time.Second)
		mu.Unlock()
		return nil
	}

	// Walk through the source directory
	fileCount := 0
	err = filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
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

		// Update status with current file
		mu.Lock()
		status.CurrentPair = fmt.Sprintf("Copying: %s", relPath)
		mu.Unlock()

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
		status.CurrentPair = fmt.Sprintf("Error: %s", err.Error())
		log.Printf("Sync error: %v", err)
	} else {
		status.CurrentPair = fmt.Sprintf("Completed: %d files copied", fileCount)
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
	status.CurrentPair = fmt.Sprintf("Error: %s", errMsg)
	mu.Unlock()
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

			// Use rsync for synchronization
			if err := SyncDirectories(sourcePath, destPath); err != nil {
				// Error is already set in SyncDirectories
				continue
			}
		}

		log.Println("Sync cycle completed")

		// Update next sync time
		mu.Lock()
		status.NextSyncTime = time.Now().Add(time.Duration(config.SyncInterval) * time.Second)
		mu.Unlock()
	}
}
