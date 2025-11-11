package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"strings"
)

// DynamicForwarder orchestrates the dynamic port forwarding
type DynamicForwarder struct {
	configPath string
	configs    []HostConfig
	netSetup   *NetworkSetup
	sshPool    *SSHClientPool
	cleanup    []func() error
	running    bool
}

// NewDynamicForwarder creates a new dynamic forwarder
func NewDynamicForwarder(configPath string, configs []HostConfig, helperPath string) (*DynamicForwarder, error) {
	netSetup, err := NewNetworkSetup(helperPath)
	if err != nil {
		return nil, err
	}

	sshPool := NewSSHClientPool()

	// Note: We don't load SSH auth methods here anymore (lazy loading).
	// Auth methods will be loaded on-demand when the first connection is made.
	// This allows portsmith to start even if SSH agents aren't ready yet.

	return &DynamicForwarder{
		configPath: configPath,
		configs:    configs,
		netSetup:   netSetup,
		sshPool:    sshPool,
		cleanup:    make([]func() error, 0),
	}, nil
}

// reloadConfig re-reads the configuration file and updates internal state
func (df *DynamicForwarder) reloadConfig() error {
	log.Printf("Reloading configuration from: %s", df.configPath)
	config, err := LoadConfig(df.configPath)
	if err != nil {
		return fmt.Errorf("failed to reload config: %w", err)
	}

	df.configs = config.Hosts

	// Note: We don't load SSH auth methods here (lazy loading).
	// Auth methods will be loaded on-demand when connections are made.

	return nil
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
	if df.running {
		return fmt.Errorf("forwarder is already running")
	}

	if err := df.reloadConfig(); err != nil {
		return err
	}

	log.Printf("Cleaning up stale resources from previous runs...")
	if err := df.netSetup.Cleanup(); err != nil {
		log.Printf("Initial cleanup failed: %v", err)
	}
	log.Printf("Stale resource cleanup complete")

	if err := df.setupNetwork(); err != nil {
		return err
	}

	for _, cfg := range df.configs {
		displayName := cfg.RemoteHost
		if len(cfg.Hostnames) > 0 {
			displayName = fmt.Sprintf("%s (%s)", strings.Join(cfg.Hostnames, ", "), cfg.RemoteHost)
		}

		ports, err := ExpandPorts(cfg)
		if err != nil {
			return fmt.Errorf("failed to expand ports for %s: %w", displayName, err)
		}

		if len(ports) == 0 {
			log.Printf("%s has no ports configured - skipping", displayName)
			continue
		}

		log.Printf("Setting up %s -> %s (%d ports)",
			cfg.LocalIP, displayName, len(ports))

		for _, port := range ports {
			fwdCfg := NewForwardConfig(cfg, port)

			if fwdCfg.NeedsPFRedirect() {
				cleanup, err := df.netSetup.SetupPFRedirect(fwdCfg.LocalIP, fwdCfg.Port, fwdCfg.ListenPort)
				if err != nil {
					return fmt.Errorf("failed to setup pf redirect for %s:%d: %w", fwdCfg.LocalIP, fwdCfg.Port, err)
				}
				df.cleanup = append(df.cleanup, cleanup)
			}

			go df.listenAndForward(fwdCfg)
		}
	}

	df.running = true
	log.Printf("Port forwarding started")
	return nil
}

// Stop stops the port forwarding and cleans up
func (df *DynamicForwarder) Stop() error {
	if !df.running {
		return nil
	}

	log.Printf("Stopping port forwarding...")
	df.running = false
	return df.Close()
}

// IsRunning returns whether the forwarder is currently running
func (df *DynamicForwarder) IsRunning() bool {
	return df.running
}

// Close shuts down the forwarder and cleans up resources
func (df *DynamicForwarder) Close() error {
	df.sshPool.Close()

	for i := len(df.cleanup) - 1; i >= 0; i-- {
		if err := df.cleanup[i](); err != nil {
			log.Printf("Cleanup error: %v", err)
		}
	}

	return nil
}

// listenAndForward listens on a port and forwards connections
func (df *DynamicForwarder) listenAndForward(cfg ForwardConfig) {
	listenAddr := fmt.Sprintf("%s:%d", cfg.LocalIP, cfg.ListenPort)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Printf("Failed to listen on %s: %v", listenAddr, err)
		return
	}
	defer listener.Close()

	if cfg.NeedsPFRedirect() {
		log.Printf("Listening on %s (redirected from %s:%d)", listenAddr, cfg.LocalIP, cfg.Port)
	} else {
		log.Printf("Listening on %s", listenAddr)
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Accept error on %s: %v", listenAddr, err)
			return
		}

		go df.forwardConnection(conn, cfg)
	}
}

// forwardConnection forwards a single connection through SSH
func (df *DynamicForwarder) forwardConnection(localConn net.Conn, cfg ForwardConfig) {
	defer localConn.Close()

	sshClient, err := df.sshPool.GetClient(cfg.JumpHost, cfg.JumpPort, cfg.KeyPath, cfg.IdentityAgent)
	if err != nil {
		log.Printf("Failed to get SSH client: %v", err)
		return
	}

	remoteAddr := fmt.Sprintf("%s:%d", cfg.RemoteHost, cfg.Port)
	remoteConn, err := sshClient.Dial("tcp", remoteAddr)
	if err != nil {
		log.Printf("Connection failed, attempting reconnect: %v", err)
		df.sshPool.RemoveClient(cfg.JumpHost, cfg.JumpPort)

		sshClient, err = df.sshPool.GetClient(cfg.JumpHost, cfg.JumpPort, cfg.KeyPath, cfg.IdentityAgent)
		if err != nil {
			log.Printf("Failed to reconnect: %v", err)
			return
		}

		remoteConn, err = sshClient.Dial("tcp", remoteAddr)
		if err != nil {
			log.Printf("Failed to dial %s after reconnect: %v", remoteAddr, err)
			return
		}
	}
	defer remoteConn.Close()

	log.Printf("Forwarding: :%d -> %s", cfg.Port, remoteAddr)

	done := make(chan struct{}, 2)

	go func() {
		io.Copy(remoteConn, localConn)
		done <- struct{}{}
	}()

	go func() {
		io.Copy(localConn, remoteConn)
		done <- struct{}{}
	}()

	<-done
	log.Printf("Connection closed: :%d", cfg.Port)
}
