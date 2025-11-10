package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	DefaultConfigPath = "config.yaml"
	GlobalConfigPath  = "~/.config/portsmith/config.yaml"
	DefaultKeyPath    = "~/.ssh/id_rsa"
	SSHDefaultPort    = 22
)

// HostConfig represents configuration for a single forwarding target
type HostConfig struct {
	LocalIP    string        `yaml:"local_ip"`
	Hostnames  []string      `yaml:"hostnames"`
	RemoteHost string        `yaml:"remote_host"`
	JumpHost   string        `yaml:"jump_host"`
	JumpPort   int           `yaml:"jump_port"`
	KeyPath    string        `yaml:"key_path"`
	Ports      []interface{} `yaml:"ports"` // Supports both ints (80) and strings ("100-105")
}

// Config represents the top-level configuration
type Config struct {
	Hosts []HostConfig `yaml:"hosts"`
}

// ForwardConfig contains all parameters needed for a single forward connection
type ForwardConfig struct {
	LocalIP    string
	RemoteHost string
	Port       int // Port to forward to on remote host
	ListenPort int // Port to listen on locally (may differ if using pf redirect)
	JumpHost   string
	JumpPort   int
	KeyPath    string
}

// NewForwardConfig creates a ForwardConfig from a HostConfig and port
func NewForwardConfig(host HostConfig, port int) ForwardConfig {
	listenPort := port
	// If port is privileged, use a high port and we'll set up pf redirect
	if port < 1024 {
		listenPort = 10000 + port
	}

	return ForwardConfig{
		LocalIP:    host.LocalIP,
		RemoteHost: host.RemoteHost,
		Port:       port,
		ListenPort: listenPort,
		JumpHost:   host.JumpHost,
		JumpPort:   host.JumpPort,
		KeyPath:    host.KeyPath,
	}
}

// NeedsPFRedirect returns true if this config requires a pf redirect
func (fc ForwardConfig) NeedsPFRedirect() bool {
	return fc.Port != fc.ListenPort
}

// LoadConfig reads and parses a YAML configuration file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults if not specified
	for i := range config.Hosts {
		if config.Hosts[i].JumpPort == 0 {
			config.Hosts[i].JumpPort = SSHDefaultPort
		}
		if config.Hosts[i].KeyPath == "" {
			config.Hosts[i].KeyPath = DefaultKeyPath
		}
	}

	return &config, nil
}

// FindConfigPath searches for a config file using a fallback chain:
// 1. Current directory: ./config.yaml
// 2. Global config: ~/.config/portsmith/config.yaml
// Returns the first path that exists, or an error if none are found.
func FindConfigPath() (string, error) {
	// Check current directory first
	if _, err := os.Stat(DefaultConfigPath); err == nil {
		return DefaultConfigPath, nil
	}

	// Check global config location
	globalPath, err := ExpandKeyPath(GlobalConfigPath)
	if err != nil {
		return "", fmt.Errorf("failed to expand global config path: %w", err)
	}

	if _, err := os.Stat(globalPath); err == nil {
		return globalPath, nil
	}

	// No config found
	return "", fmt.Errorf("no config file found. Searched:\n  - %s (current directory)\n  - %s (global config)", DefaultConfigPath, GlobalConfigPath)
}

// ExpandPorts converts port specifications into a flat list.
// Supports both individual ports (80) and ranges ("100-105").
// Returns an empty list if no ports are specified.
func ExpandPorts(config HostConfig) ([]int, error) {
	portsMap := make(map[int]bool)

	for _, portSpec := range config.Ports {
		switch v := portSpec.(type) {
		case int:
			// Individual port
			portsMap[v] = true
		case string:
			// Port range in format "start-end"
			var start, end int
			n, err := fmt.Sscanf(v, "%d-%d", &start, &end)
			if err != nil || n != 2 {
				return nil, fmt.Errorf("invalid port range format %q, expected format: \"start-end\"", v)
			}
			if start > end {
				return nil, fmt.Errorf("invalid port range %q: start (%d) must be <= end (%d)", v, start, end)
			}
			for p := start; p <= end; p++ {
				portsMap[p] = true
			}
		default:
			return nil, fmt.Errorf("invalid port specification: must be int or string range, got %T", v)
		}
	}

	// Convert map to slice
	ports := make([]int, 0, len(portsMap))
	for port := range portsMap {
		ports = append(ports, port)
	}

	return ports, nil
}
