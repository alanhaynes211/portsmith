package main

import (
	"os"
	"testing"
)

func TestExpandPorts(t *testing.T) {
	tests := []struct {
		name      string
		config    HostConfig
		expected  int
		shouldErr bool
	}{
		{
			name: "explicit ports",
			config: HostConfig{
				Ports: []interface{}{80, 443, 8080},
			},
			expected: 3,
		},
		{
			name: "port range string",
			config: HostConfig{
				Ports: []interface{}{"8000-8005"},
			},
			expected: 6, // 8000-8005 inclusive
		},
		{
			name:     "no ports specified",
			config:   HostConfig{},
			expected: 0, // No defaults - returns empty
		},
		{
			name: "mixed ports and ranges",
			config: HostConfig{
				Ports: []interface{}{80, 443, "9000-9002"},
			},
			expected: 5, // 80, 443, 9000, 9001, 9002
		},
		{
			name: "duplicate ports",
			config: HostConfig{
				Ports: []interface{}{80, 80, 443, "80-81"},
			},
			expected: 3, // 80, 81, 443 (deduped)
		},
		{
			name: "invalid range format",
			config: HostConfig{
				Ports: []interface{}{"invalid"},
			},
			shouldErr: true,
		},
		{
			name: "reversed range",
			config: HostConfig{
				Ports: []interface{}{"100-50"},
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ports, err := ExpandPorts(tt.config)
			if tt.shouldErr {
				if err == nil {
					t.Errorf("ExpandPorts() expected error, got none")
				}
				return
			}
			if err != nil {
				t.Errorf("ExpandPorts() unexpected error: %v", err)
				return
			}
			if len(ports) != tt.expected {
				t.Errorf("ExpandPorts() returned %d ports, expected %d", len(ports), tt.expected)
			}
		})
	}
}

func TestNewForwardConfig(t *testing.T) {
	host := HostConfig{
		LocalIP:    "127.0.0.2",
		RemoteHost: "remote.example.com",
		JumpHost:   "jump.example.com",
		JumpPort:   2222,
		KeyPath:    "~/.ssh/id_rsa",
	}

	cfg := NewForwardConfig(host, 8080)

	if cfg.LocalIP != host.LocalIP {
		t.Errorf("LocalIP = %s, want %s", cfg.LocalIP, host.LocalIP)
	}
	if cfg.RemoteHost != host.RemoteHost {
		t.Errorf("RemoteHost = %s, want %s", cfg.RemoteHost, host.RemoteHost)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want %d", cfg.Port, 8080)
	}
	if cfg.JumpHost != host.JumpHost {
		t.Errorf("JumpHost = %s, want %s", cfg.JumpHost, host.JumpHost)
	}
	if cfg.JumpPort != host.JumpPort {
		t.Errorf("JumpPort = %d, want %d", cfg.JumpPort, host.JumpPort)
	}
	if cfg.KeyPath != host.KeyPath {
		t.Errorf("KeyPath = %s, want %s", cfg.KeyPath, host.KeyPath)
	}
}

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	tmpFile, err := os.CreateTemp("", "portsmith-test-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	configContent := `hosts:
  - local_ip: 127.0.0.2
    hostnames:
      - test.local
    remote_host: remote.example.com
    jump_host: jump.example.com
    key_path: ~/.ssh/id_rsa
    ports: [80, 443]
  - local_ip: 127.0.0.3
    remote_host: another.example.com
    jump_host: jump.example.com
    jump_port: 2222
    key_path: ~/.ssh/id_rsa
`

	if _, err := tmpFile.Write([]byte(configContent)); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpFile.Close()

	config, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if len(config.Hosts) != 2 {
		t.Errorf("Expected 2 hosts, got %d", len(config.Hosts))
	}

	// Test first host
	host1 := config.Hosts[0]
	if host1.LocalIP != "127.0.0.2" {
		t.Errorf("Host1 LocalIP = %s, want 127.0.0.2", host1.LocalIP)
	}
	if len(host1.Hostnames) != 1 || host1.Hostnames[0] != "test.local" {
		t.Errorf("Host1 Hostnames = %v, want [test.local]", host1.Hostnames)
	}
	if host1.JumpPort != SSHDefaultPort {
		t.Errorf("Host1 JumpPort = %d, want %d (default)", host1.JumpPort, SSHDefaultPort)
	}

	// Test second host
	host2 := config.Hosts[1]
	if host2.JumpPort != 2222 {
		t.Errorf("Host2 JumpPort = %d, want 2222", host2.JumpPort)
	}
}

func TestLoadConfigInvalidFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/file.yaml")
	if err == nil {
		t.Error("LoadConfig() should return error for nonexistent file")
	}
}

func TestLoadConfigInvalidYAML(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "portsmith-test-invalid-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write invalid YAML
	tmpFile.WriteString("invalid: yaml: content: [")
	tmpFile.Close()

	_, err = LoadConfig(tmpFile.Name())
	if err == nil {
		t.Error("LoadConfig() should return error for invalid YAML")
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	// Create a config with minimal settings to test defaults
	tmpFile, err := os.CreateTemp("", "portsmith-test-defaults-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	configContent := `hosts:
  - local_ip: 127.0.0.2
    remote_host: remote.example.com
    jump_host: jump.example.com
    # Omit jump_port and key_path to test defaults
`

	if _, err := tmpFile.Write([]byte(configContent)); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpFile.Close()

	config, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if len(config.Hosts) != 1 {
		t.Fatalf("Expected 1 host, got %d", len(config.Hosts))
	}

	host := config.Hosts[0]

	// Test default JumpPort
	if host.JumpPort != SSHDefaultPort {
		t.Errorf("JumpPort = %d, want %d (default)", host.JumpPort, SSHDefaultPort)
	}

	// Test default KeyPath
	if host.KeyPath != DefaultKeyPath {
		t.Errorf("KeyPath = %s, want %s (default)", host.KeyPath, DefaultKeyPath)
	}
}
