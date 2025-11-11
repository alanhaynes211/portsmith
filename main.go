package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

func main() {
	helperPath := "/usr/local/bin/portsmith-helper"
	if _, err := os.Stat(helperPath); err != nil {
		helperPath = "bin/portsmith-helper"
	}

	// Setup logging to file before any log statements
	setupSystrayLogging()

	configPath, err := FindConfigPath()
	if err != nil {
		log.Fatalf("Failed to find config: %v", err)
	}

	log.Printf("Loading configuration from: %s", configPath)

	config, err := LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	forwarder, err := NewDynamicForwarder(configPath, config.Hosts, helperPath)
	if err != nil {
		log.Fatalf("Failed to initialize forwarder: %v", err)
	}

	runSystrayMode(forwarder)
}

// runSystrayMode runs portsmith with system tray UI
func runSystrayMode(forwarder *DynamicForwarder) {
	fmt.Println("Portsmith starting in system tray...")
	fmt.Println("Logs: ~/Library/Logs/Portsmith/portsmith.log")
	log.Println("Starting Portsmith in systray mode...")
	app := NewSystrayApp(forwarder)
	app.Run()
}

// setupSystrayLogging redirects logs to a file
func setupSystrayLogging() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to get home directory: %v", err)
	}

	logDir := filepath.Join(homeDir, "Library", "Logs", "Portsmith")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Fatalf("Failed to create log directory: %v", err)
	}

	logFile := filepath.Join(logDir, "portsmith.log")
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}

	log.SetOutput(f)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
