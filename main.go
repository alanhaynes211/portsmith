package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
)

func main() {
	helperPath := "/usr/local/bin/portsmith-helper"
	if _, err := os.Stat(helperPath); err != nil {
		helperPath = "bin/portsmith-helper"
	}

	var configPath string
	cliMode := false
	daemonMode := false

	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch arg {
		case "--cli", "-c":
			cliMode = true
		case "--daemon":
			daemonMode = true
		default:
			configPath = arg
			if !cliMode {
				cliMode = true
			}
		}
	}

	if !cliMode && !daemonMode {
		daemonize()
		return
	}

	if daemonMode {
		setupDaemonLogging()
	}

	if configPath == "" {
		foundPath, err := FindConfigPath()
		if err != nil {
			log.Fatalf("Failed to find config: %v", err)
		}
		configPath = foundPath
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

	if cliMode {
		runCLIMode(forwarder)
	} else {
		runSystrayMode(forwarder)
	}
}

// runCLIMode runs portsmith in traditional CLI mode
func runCLIMode(forwarder *DynamicForwarder) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("\nShutting down gracefully...")
		forwarder.Stop()
		os.Exit(0)
	}()

	log.Println("Starting dynamic SSH forwarder (CLI mode)...")
	if err := forwarder.Start(); err != nil {
		log.Fatal(err)
	}

	select {}
}

// runSystrayMode runs portsmith with system tray UI
func runSystrayMode(forwarder *DynamicForwarder) {
	log.Println("Starting Portsmith in systray mode...")
	app := NewSystrayApp(forwarder)
	app.Run()
}

// daemonize forks the process and detaches from terminal
func daemonize() {
	executable, err := os.Executable()
	if err != nil {
		log.Fatalf("Failed to get executable path: %v", err)
	}

	args := []string{"--daemon"}
	args = append(args, os.Args[1:]...)

	cmd := exec.Command(executable, args...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to daemonize: %v", err)
	}

	fmt.Println("Portsmith started in background")
	fmt.Printf("PID: %d\n", cmd.Process.Pid)
	fmt.Println("Check logs at: ~/Library/Logs/Portsmith/portsmith.log")

	os.Exit(0)
}

// setupDaemonLogging redirects logs to a file
func setupDaemonLogging() {
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
