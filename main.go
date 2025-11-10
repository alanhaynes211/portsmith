package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// Find helper path - try installed location first, then local
	helperPath := "/usr/local/bin/portsmith-helper"
	if _, err := os.Stat(helperPath); err != nil {
		// Not found in /usr/local/bin, try local bin
		helperPath = "bin/portsmith-helper"
	}

	// Determine config path
	var configPath string
	if len(os.Args) > 1 {
		// User provided explicit path
		configPath = os.Args[1]
	} else {
		// Use config discovery (current dir or global)
		foundPath, err := FindConfigPath()
		if err != nil {
			log.Fatalf("Failed to find config: %v", err)
		}
		configPath = foundPath
	}

	if len(os.Args) > 2 {
		helperPath = os.Args[2]
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	forwarder, err := NewDynamicForwarder(config.Hosts, helperPath)
	if err != nil {
		log.Fatalf("Failed to initialize forwarder: %v", err)
	}

	// Handle signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("\nShutting down gracefully...")
		forwarder.Close()
		os.Exit(0)
	}()

	log.Printf("Loading configuration from: %s", configPath)
	log.Println("Starting dynamic SSH forwarder...")
	if err := forwarder.Start(); err != nil {
		log.Fatal(err)
	}
}
