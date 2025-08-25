package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"mockProxy/internal/cdn"
	"mockProxy/internal/proxy"
	"mockProxy/internal/server"
)

func main() {
	log.Println("Starting CDN vs Proxy Demo...")

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
