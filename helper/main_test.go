package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateIP(t *testing.T) {
	tests := []struct {
		name      string
		ip        string
		shouldErr bool
	}{
		{
			name:      "valid loopback",
			ip:        "127.0.0.2",
			shouldErr: false,
		},
		{
			name:      "localhost",
			ip:        "127.0.0.1",
			shouldErr: false,
		},
		{
			name:      "high loopback",
			ip:        "127.0.0.255",
			shouldErr: false,
		},
		{
			name:      "non-loopback",
			ip:        "192.168.1.1",
			shouldErr: true,
		},
		{
			name:      "invalid IP",
			ip:        "not-an-ip",
			shouldErr: true,
		},
		{
			name:      "empty string",
			ip:        "",
			shouldErr: true,
		},
		{
			name:      "public IP",
			ip:        "8.8.8.8",
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateIP(tt.ip)
			if tt.shouldErr && err == nil {
				t.Errorf("validateIP(%q) expected error, got none", tt.ip)
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("validateIP(%q) unexpected error: %v", tt.ip, err)
			}
		})
	}
}

func TestValidateHostname(t *testing.T) {
	tests := []struct {
		name      string
		hostname  string
		shouldErr bool
	}{
		{
			name:      "valid hostname",
			hostname:  "example.local",
			shouldErr: false,
		},
		{
			name:      "simple name",
			hostname:  "myapp",
			shouldErr: false,
		},
		{
			name:      "with dashes",
			hostname:  "my-app.local",
			shouldErr: false,
		},
		{
			name:      "with underscores",
			hostname:  "my_app.local",
			shouldErr: false,
		},
		{
			name:      "contains space",
			hostname:  "my app",
			shouldErr: true,
		},
		{
			name:      "contains tab",
			hostname:  "my\tapp",
			shouldErr: true,
		},
		{
			name:      "contains newline",
			hostname:  "my\napp",
			shouldErr: true,
		},
		{
			name:      "empty string",
			hostname:  "",
			shouldErr: true,
		},
		{
			name:      "too long",
			hostname:  strings.Repeat("a", 254),
			shouldErr: true,
		},
		{
			name:      "max valid length",
			hostname:  strings.Repeat("a", 253),
			shouldErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHostname(tt.hostname)
			if tt.shouldErr && err == nil {
				t.Errorf("validateHostname(%q) expected error, got none", tt.hostname)
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("validateHostname(%q) unexpected error: %v", tt.hostname, err)
			}
		})
	}
}

func TestRemoveHosts(t *testing.T) {
	// Create a temporary hosts file
	tmpDir := t.TempDir()
	tmpHosts := filepath.Join(tmpDir, "hosts")

	// Write test content
	content := `127.0.0.1 localhost
127.0.0.2 test.local # portsmith-dynamic-forward
192.168.1.1 regular.host
127.0.0.3 another.local # portsmith-dynamic-forward
127.0.0.4 normal.host
`
	if err := os.WriteFile(tmpHosts, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test hosts file: %v", err)
	}

	// Override hostsPath for testing
	originalHostsPath := hostsPath
	hostsPath = tmpHosts
	defer func() { hostsPath = originalHostsPath }()

	// Run removeHosts
	if err := removeHosts(); err != nil {
		t.Fatalf("removeHosts() error: %v", err)
	}

	// Read result
	result, err := os.ReadFile(tmpHosts)
	if err != nil {
		t.Fatalf("Failed to read result: %v", err)
	}

	resultStr := string(result)

	// Should keep entries without marker
	if !strings.Contains(resultStr, "127.0.0.1 localhost") {
		t.Error("removeHosts() removed localhost entry")
	}
	if !strings.Contains(resultStr, "192.168.1.1 regular.host") {
		t.Error("removeHosts() removed regular host entry")
	}
	if !strings.Contains(resultStr, "127.0.0.4 normal.host") {
		t.Error("removeHosts() removed normal host entry")
	}

	// Should remove entries with marker
	if strings.Contains(resultStr, "test.local") {
		t.Error("removeHosts() did not remove test.local")
	}
	if strings.Contains(resultStr, "another.local") {
		t.Error("removeHosts() did not remove another.local")
	}
}

func TestRemoveHostsEmpty(t *testing.T) {
	// Create a temporary hosts file with no portsmith entries
	tmpDir := t.TempDir()
	tmpHosts := filepath.Join(tmpDir, "hosts")

	content := `127.0.0.1 localhost
192.168.1.1 regular.host
`
	if err := os.WriteFile(tmpHosts, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test hosts file: %v", err)
	}

	originalHostsPath := hostsPath
	hostsPath = tmpHosts
	defer func() { hostsPath = originalHostsPath }()

	// Should not error when no entries to remove
	if err := removeHosts(); err != nil {
		t.Fatalf("removeHosts() error on empty: %v", err)
	}

	// File should be unchanged
	result, _ := os.ReadFile(tmpHosts)
	if string(result) != content {
		t.Error("removeHosts() modified file when no portsmith entries present")
	}
}

func TestAddHost(t *testing.T) {
	tmpDir := t.TempDir()
	tmpHosts := filepath.Join(tmpDir, "hosts")

	// Initial content
	content := "127.0.0.1 localhost\n"
	if err := os.WriteFile(tmpHosts, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test hosts file: %v", err)
	}

	originalHostsPath := hostsPath
	hostsPath = tmpHosts
	defer func() { hostsPath = originalHostsPath }()

	// Add a host entry
	if err := addHost("127.0.0.2", "test.local"); err != nil {
		t.Fatalf("addHost() error: %v", err)
	}

	// Read result
	result, _ := os.ReadFile(tmpHosts)
	resultStr := string(result)

	// Should contain the new entry with marker
	if !strings.Contains(resultStr, "127.0.0.2 test.local") {
		t.Error("addHost() did not add entry")
	}
	if !strings.Contains(resultStr, hostsMarker) {
		t.Error("addHost() did not add marker")
	}

	// Adding same entry again should not duplicate
	if err := addHost("127.0.0.2", "test.local"); err != nil {
		t.Fatalf("addHost() error on duplicate: %v", err)
	}

	result2, _ := os.ReadFile(tmpHosts)
	count := strings.Count(string(result2), "127.0.0.2 test.local")
	if count != 1 {
		t.Errorf("addHost() created duplicate entries, found %d occurrences", count)
	}
}

func TestAddHostInvalidInputs(t *testing.T) {
	tmpDir := t.TempDir()
	tmpHosts := filepath.Join(tmpDir, "hosts")
	os.WriteFile(tmpHosts, []byte(""), 0644)

	originalHostsPath := hostsPath
	hostsPath = tmpHosts
	defer func() { hostsPath = originalHostsPath }()

	tests := []struct {
		name     string
		ip       string
		hostname string
	}{
		{
			name:     "invalid IP",
			ip:       "not-an-ip",
			hostname: "test.local",
		},
		{
			name:     "non-loopback IP",
			ip:       "192.168.1.1",
			hostname: "test.local",
		},
		{
			name:     "invalid hostname with space",
			ip:       "127.0.0.2",
			hostname: "test local",
		},
		{
			name:     "empty hostname",
			ip:       "127.0.0.2",
			hostname: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := addHost(tt.ip, tt.hostname)
			if err == nil {
				t.Errorf("addHost(%q, %q) expected error, got none", tt.ip, tt.hostname)
			}
		})
	}
}

func TestRemoveHost(t *testing.T) {
	tmpDir := t.TempDir()
	tmpHosts := filepath.Join(tmpDir, "hosts")

	content := `127.0.0.1 localhost
127.0.0.2 test.local # portsmith-dynamic-forward
127.0.0.3 another.local # portsmith-dynamic-forward
`
	if err := os.WriteFile(tmpHosts, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test hosts file: %v", err)
	}

	originalHostsPath := hostsPath
	hostsPath = tmpHosts
	defer func() { hostsPath = originalHostsPath }()

	// Remove one specific entry
	if err := removeHost("127.0.0.2", "test.local"); err != nil {
		t.Fatalf("removeHost() error: %v", err)
	}

	result, _ := os.ReadFile(tmpHosts)
	resultStr := string(result)

	// Should remove only the specified entry
	if strings.Contains(resultStr, "test.local") {
		t.Error("removeHost() did not remove test.local")
	}

	// Should keep other entries
	if !strings.Contains(resultStr, "localhost") {
		t.Error("removeHost() removed localhost")
	}
	if !strings.Contains(resultStr, "another.local") {
		t.Error("removeHost() removed another.local")
	}
}

func TestRemoveAliasesDarwinParsing(t *testing.T) {
	// Test parsing logic for ifconfig output
	ifconfigOutput := `lo0: flags=8049<UP,LOOPBACK,RUNNING,MULTICAST> mtu 16384
	options=1203<RXCSUM,TXCSUM,TXSTATUS,SW_TIMESTAMP>
	inet 127.0.0.1 netmask 0xff000000
	inet 127.0.0.2 netmask 0xff000000
	inet 127.0.0.3 netmask 0xff000000
	inet6 ::1 prefixlen 128
	inet6 fe80::1%lo0 prefixlen 64 scopeid 0x1
	nd6 options=201<PERFORMNUD,DAD>`

	lines := strings.Split(ifconfigOutput, "\n")
	var foundIPs []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "inet ") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		ip := fields[1]
		if ip == "127.0.0.1" {
			continue
		}

		if strings.HasPrefix(ip, "127.0.0.") {
			foundIPs = append(foundIPs, ip)
		}
	}

	expected := []string{"127.0.0.2", "127.0.0.3"}
	if len(foundIPs) != len(expected) {
		t.Errorf("Expected %d IPs, found %d", len(expected), len(foundIPs))
	}

	for i, ip := range expected {
		if foundIPs[i] != ip {
			t.Errorf("Expected IP %s at position %d, got %s", ip, i, foundIPs[i])
		}
	}
}

