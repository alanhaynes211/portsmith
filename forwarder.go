package main

import (
	"fmt"
	"log"
	"strings"
)

// DynamicForwarder orchestrates the dynamic port forwarding
type DynamicForwarder struct {
	configs    []HostConfig
	netSetup   *NetworkSetup
	sshPool    *SSHClientPool
	connHandle *ConnectionHandler
	cleanup    []func() error
}

// NewDynamicForwarder creates a new dynamic forwarder
func NewDynamicForwarder(configs []HostConfig, helperPath string) (*DynamicForwarder, error) {
	// Create network setup
	netSetup, err := NewNetworkSetup(helperPath)
	if err != nil {
		return nil, err
	}

	// Create SSH client pool
	sshPool := NewSSHClientPool()

	// Load SSH auth methods upfront for each unique key path
	keyPaths := make(map[string]bool)
	for _, config := range configs {
		if config.KeyPath != "" {
			keyPaths[config.KeyPath] = true
		}
	}

	for keyPath := range keyPaths {
		log.Printf("Loading SSH authentication for %s...", keyPath)
		if err := sshPool.LoadAuthMethods(keyPath); err != nil {
			return nil, err
		}
	}

	// Create connection handler
	connHandler := NewConnectionHandler(sshPool)

	return &DynamicForwarder{
		configs:    configs,
		netSetup:   netSetup,
		sshPool:    sshPool,
		connHandle: connHandler,
		cleanup:    make([]func() error, 0),
	}, nil
}

// setupNetwork configures all network settings
func (df *DynamicForwarder) setupNetwork() error {
	cleanup, err := df.netSetup.SetupNetwork(df.configs)
	if err != nil {
		return err
	}
	df.cleanup = append(df.cleanup, cleanup...)
	return nil
}

// Start begins the port forwarding
func (df *DynamicForwarder) Start() error {
	// Clean up any stale resources from previous runs
	log.Printf("Cleaning up stale resources from previous runs...")
	if err := df.netSetup.CleanupAll(); err != nil {
		log.Printf("Warning: Initial cleanup failed: %v", err)
	}
	log.Printf("Stale resource cleanup complete")

	// Setup network interfaces and hosts
	if err := df.setupNetwork(); err != nil {
		return err
	}

	for _, config := range df.configs {
		displayName := config.RemoteHost
		if len(config.Hostnames) > 0 {
			displayName = fmt.Sprintf("%s (%s)", strings.Join(config.Hostnames, ", "), config.RemoteHost)
		}

		ports, err := ExpandPorts(config)
		if err != nil {
			return fmt.Errorf("failed to expand ports for %s: %w", displayName, err)
		}

		if len(ports) == 0 {
			log.Printf("Warning: %s has no ports configured - skipping", displayName)
			continue
		}

		log.Printf("Setting up %s -> %s (%d ports)",
			config.LocalIP, displayName, len(ports))

		for _, port := range ports {
			cfg := NewForwardConfig(config, port)

			// Set up pf redirect if this is a privileged port
			if cfg.NeedsPFRedirect() {
				cleanup, err := df.netSetup.SetupPFRedirect(cfg.LocalIP, cfg.Port, cfg.ListenPort)
				if err != nil {
					return fmt.Errorf("failed to setup pf redirect for %s:%d: %w", cfg.LocalIP, cfg.Port, err)
				}
				df.cleanup = append(df.cleanup, cleanup)
			}

			go df.connHandle.ListenOnPort(cfg)
		}
	}

	select {}
}

// Close shuts down the forwarder and cleans up resources
func (df *DynamicForwarder) Close() error {
	df.sshPool.Close()

	// Run cleanup functions in reverse order
	for i := len(df.cleanup) - 1; i >= 0; i-- {
		if err := df.cleanup[i](); err != nil {
			log.Printf("Cleanup error: %v", err)
		}
	}

	return nil
}
