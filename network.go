package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
)

// NetworkSetup handles privileged network operations via the helper binary
type NetworkSetup struct {
	helperPath string
}

// NewNetworkSetup creates a new network setup manager
func NewNetworkSetup(helperPath string) (*NetworkSetup, error) {
	// Verify helper exists and is executable
	if _, err := os.Stat(helperPath); err != nil {
		return nil, fmt.Errorf("helper not found at %s: %w", helperPath, err)
	}

	return &NetworkSetup{
		helperPath: helperPath,
	}, nil
}

// runHelper executes the helper with the given arguments
func (ns *NetworkSetup) runHelper(args ...string) error {
	cmd := exec.Command("sudo", append([]string{ns.helperPath}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// SetupLoopbackAlias creates a loopback alias for the given IP
func (ns *NetworkSetup) SetupLoopbackAlias(ip string) (func() error, error) {
	if err := ns.runHelper("add-alias", ip); err != nil {
		return nil, fmt.Errorf("failed to add loopback alias %s: %w", ip, err)
	}

	log.Printf("Created loopback alias %s", ip)

	// Return cleanup function
	cleanup := func() error {
		if err := ns.runHelper("remove-alias", ip); err != nil {
			return fmt.Errorf("failed to remove loopback alias %s: %w", ip, err)
		}
		log.Printf("Removed loopback alias %s", ip)
		return nil
	}

	return cleanup, nil
}

// AddHostsEntries adds /etc/hosts entries for the given hostnames
func (ns *NetworkSetup) AddHostsEntries(ip string, hostnames []string) (func() error, error) {
	if len(hostnames) == 0 {
		return func() error { return nil }, nil
	}

	for _, hostname := range hostnames {
		if err := ns.runHelper("add-host", ip, hostname); err != nil {
			return nil, fmt.Errorf("failed to add hosts entry %s -> %s: %w", hostname, ip, err)
		}
		log.Printf("Added /etc/hosts entry: %s -> %s", hostname, ip)
	}

	// Return cleanup function
	cleanup := func() error {
		for _, hostname := range hostnames {
			if err := ns.runHelper("remove-host", ip, hostname); err != nil {
				log.Printf("Warning: Failed to remove hosts entry %s -> %s: %v", hostname, ip, err)
			}
		}
		log.Printf("Removed /etc/hosts entries for %s", ip)
		return nil
	}

	return cleanup, nil
}

// SetupPFRedirect creates a pf redirect for privileged ports
func (ns *NetworkSetup) SetupPFRedirect(ip string, fromPort, toPort int) (func() error, error) {
	if err := ns.runHelper("add-pf-redirect", ip, fmt.Sprintf("%d", fromPort), fmt.Sprintf("%d", toPort)); err != nil {
		return nil, fmt.Errorf("failed to add pf redirect %s:%d -> %s:%d: %w", ip, fromPort, ip, toPort, err)
	}

	log.Printf("Created pf redirect: %s:%d -> %s:%d", ip, fromPort, ip, toPort)

	// Return cleanup function
	cleanup := func() error {
		if err := ns.runHelper("remove-pf-redirect", ip, fmt.Sprintf("%d", fromPort), fmt.Sprintf("%d", toPort)); err != nil {
			return fmt.Errorf("failed to remove pf redirect %s:%d -> %s:%d: %w", ip, fromPort, ip, toPort, err)
		}
		log.Printf("Removed pf redirect: %s:%d -> %s:%d", ip, fromPort, ip, toPort)
		return nil
	}

	return cleanup, nil
}

// Cleanup removes all portsmith resources via helper commands
// This performs bulk cleanup and is used for:
// - Cleaning up stale resources on startup
// - Manual cleanup via `just cleanup`
func (ns *NetworkSetup) Cleanup() error {
	// Clean up in reverse order: pf redirects, hosts, aliases
	if err := ns.runHelper("remove-pf-redirects"); err != nil {
		log.Printf("Warning: Failed to clean up pf redirects: %v", err)
	}

	if err := ns.runHelper("remove-hosts"); err != nil {
		log.Printf("Warning: Failed to clean up hosts entries: %v", err)
	}

	if err := ns.runHelper("remove-aliases"); err != nil {
		log.Printf("Warning: Failed to clean up loopback aliases: %v", err)
	}

	return nil
}

// SetupNetwork configures all network settings for the given host configs
func (ns *NetworkSetup) SetupNetwork(configs []HostConfig) ([]func() error, error) {
	cleanup := make([]func() error, 0)

	for _, config := range configs {
		// Setup loopback alias
		cleanupFn, err := ns.SetupLoopbackAlias(config.LocalIP)
		if err != nil {
			return cleanup, fmt.Errorf("failed to setup loopback for %s: %w", config.LocalIP, err)
		}
		cleanup = append(cleanup, cleanupFn)

		// Setup hosts entries
		cleanupFn, err = ns.AddHostsEntries(config.LocalIP, config.Hostnames)
		if err != nil {
			return cleanup, fmt.Errorf("failed to setup hosts entries for %s: %w", config.LocalIP, err)
		}
		cleanup = append(cleanup, cleanupFn)
	}

	return cleanup, nil
}
