package main

import (
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"mockProxy/internal/cdn"
	"mockProxy/internal/proxy"
	"mockProxy/internal/server"
)

func main() {
	log.Println("Starting CDN vs Proxy Demo...")

	// Clean up output directories on startup
	cleanupOutputDirectories()

	// Initialize CDN service
	cdnService := cdn.NewService()
	go cdnService.Start()

	// Initialize Proxy service
	proxyService := proxy.NewService()
	go proxyService.Start()

	// Initialize HTTP server
	httpServer := server.NewServer(cdnService, proxyService)
	go httpServer.Start()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	cdnService.Stop()
	proxyService.Stop()
	httpServer.Stop()
}

// cleanupOutputDirectories removes all files from origin, cdn_output, and proxy_output directories
func cleanupOutputDirectories() {
	directories := []string{"origin", "cdn_output", "proxy_output"}
	
	for _, dir := range directories {
		// Check if directory exists
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			// Create directory if it doesn't exist
			if err := os.MkdirAll(dir, 0755); err != nil {
				log.Printf("Failed to create directory %s: %v", dir, err)
				continue
			}
			log.Printf("Created directory: %s", dir)
		} else {
			// Directory exists, clean it up
			entries, err := os.ReadDir(dir)
			if err != nil {
				log.Printf("Failed to read directory %s: %v", dir, err)
				continue
			}
			
			// Remove all files in the directory
			for _, entry := range entries {
				if !entry.IsDir() {
					filePath := filepath.Join(dir, entry.Name())
					if err := os.Remove(filePath); err != nil {
						log.Printf("Failed to remove file %s: %v", filePath, err)
					} else {
						log.Printf("Removed file: %s", filePath)
					}
				}
			}
			log.Printf("Cleaned up directory: %s", dir)
		}
	}
}
