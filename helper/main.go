package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const (
	hostsMarker = "# portsmith-dynamic-forward"
)

var (
	hostsPath = "/etc/hosts"
)

func checkRoot() {
	if os.Geteuid() != 0 {
		fmt.Fprintf(os.Stderr, "Error: portsmith-helper must be run as root\n")
		os.Exit(1)
	}
}

func validateIP(ip string) error {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return fmt.Errorf("invalid IP address: %s", ip)
	}
	// Only allow loopback addresses for security
	if !parsed.IsLoopback() {
		return fmt.Errorf("only loopback addresses allowed: %s", ip)
	}
	return nil
}

func validateHostname(hostname string) error {
	// Basic validation - no spaces, reasonable length
	if strings.ContainsAny(hostname, " \t\n\r") {
		return fmt.Errorf("invalid hostname (contains whitespace): %s", hostname)
	}
	if len(hostname) > 253 {
		return fmt.Errorf("hostname too long: %s", hostname)
	}
	if len(hostname) == 0 {
		return fmt.Errorf("hostname cannot be empty")
	}
	return nil
}

func addAlias(ip string) error {
	if err := validateIP(ip); err != nil {
		return err
	}

	if runtime.GOOS != "darwin" {
		return fmt.Errorf("unsupported OS: %s (only macOS is supported)", runtime.GOOS)
	}
	return addAliasDarwin(ip)
}

func addAliasDarwin(ip string) error {
	// Check if already exists
	cmd := exec.Command("ifconfig", "lo0")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check lo0: %v", err)
	}

	if strings.Contains(string(output), ip) {
		fmt.Printf("Loopback alias %s already exists\n", ip)
		return nil
	}

	// Add alias
	cmd = exec.Command("ifconfig", "lo0", "alias", ip, "up")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add loopback alias: %v", err)
	}

	fmt.Printf("Added loopback alias: %s\n", ip)
	return nil
}


func removeAlias(ip string) error {
	if err := validateIP(ip); err != nil {
		return err
	}

	if runtime.GOOS != "darwin" {
		return fmt.Errorf("unsupported OS: %s (only macOS is supported)", runtime.GOOS)
	}
	return removeAliasDarwin(ip)
}

func removeAliasDarwin(ip string) error {
	cmd := exec.Command("ifconfig", "lo0", "-alias", ip)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove loopback alias: %v", err)
	}
	fmt.Printf("Removed loopback alias: %s\n", ip)
	return nil
}


func addHost(ip, hostname string) error {
	if err := validateIP(ip); err != nil {
		return err
	}
	if err := validateHostname(hostname); err != nil {
		return err
	}

	// Read current hosts file
	content, err := os.ReadFile(hostsPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %v", hostsPath, err)
	}

	lines := strings.Split(string(content), "\n")

	// Check if entry already exists
	searchEntry := fmt.Sprintf("%s %s", ip, hostname)
	for _, line := range lines {
		if strings.Contains(line, hostsMarker) &&
			strings.Contains(line, hostname) &&
			strings.Contains(line, ip) {
			fmt.Printf("/etc/hosts entry already exists: %s -> %s\n", hostname, ip)
			return nil
		}
	}

	// Add new entry
	newContent := string(content)
	if !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}
	newContent += fmt.Sprintf("%s %s\n", searchEntry, hostsMarker)

	if err := os.WriteFile(hostsPath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %v", hostsPath, err)
	}

	fmt.Printf("Added /etc/hosts entry: %s -> %s\n", hostname, ip)
	return nil
}

func removeHost(ip, hostname string) error {
	if err := validateIP(ip); err != nil {
		return err
	}
	if err := validateHostname(hostname); err != nil {
		return err
	}

	content, err := os.ReadFile(hostsPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %v", hostsPath, err)
	}

	lines := strings.Split(string(content), "\n")
	newLines := make([]string, 0, len(lines))

	for _, line := range lines {
		// Remove lines that match our marker and this specific hostname + IP
		if strings.Contains(line, hostsMarker) &&
			strings.Contains(line, hostname) &&
			strings.Contains(line, ip) {
			continue
		}
		newLines = append(newLines, line)
	}

	newContent := strings.Join(newLines, "\n")
	if err := os.WriteFile(hostsPath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %v", hostsPath, err)
	}

	fmt.Printf("Removed /etc/hosts entry: %s -> %s\n", hostname, ip)
	return nil
}

func removeHosts() error {
	content, err := os.ReadFile(hostsPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %v", hostsPath, err)
	}

	lines := strings.Split(string(content), "\n")
	newLines := make([]string, 0, len(lines))

	removed := 0
	for _, line := range lines {
		if strings.Contains(line, hostsMarker) {
			removed++
			continue
		}
		newLines = append(newLines, line)
	}

	if removed == 0 {
		fmt.Println("No portsmith /etc/hosts entries to remove")
		return nil
	}

	newContent := strings.Join(newLines, "\n")
	if err := os.WriteFile(hostsPath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %v", hostsPath, err)
	}

	fmt.Printf("Removed %d /etc/hosts entries\n", removed)
	return nil
}

func removeAliases() error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("unsupported OS: %s (only macOS is supported)", runtime.GOOS)
	}
	return removeAliasesDarwin()
}

func removeAliasesDarwin() error {
	// Get current lo0 configuration
	cmd := exec.Command("ifconfig", "lo0")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check lo0: %v", err)
	}

	lines := strings.Split(string(output), "\n")
	removed := 0

	// Parse ifconfig output and find inet addresses
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
		// Skip localhost
		if ip == "127.0.0.1" {
			continue
		}

		// Only remove 127.0.0.x aliases
		if strings.HasPrefix(ip, "127.0.0.") {
			cmd := exec.Command("ifconfig", "lo0", "-alias", ip)
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to remove alias %s: %v\n", ip, err)
				continue
			}
			fmt.Printf("Removed loopback alias: %s\n", ip)
			removed++
		}
	}

	if removed == 0 {
		fmt.Println("No loopback aliases to remove")
	} else {
		fmt.Printf("Removed %d loopback aliases\n", removed)
	}
	return nil
}


func printUsage() {
	fmt.Fprintf(os.Stderr, `portsmith-helper - Privileged operations helper for portsmith

Usage:
  portsmith-helper add-alias <ip>              Add loopback alias
  portsmith-helper remove-alias <ip>           Remove specific loopback alias
  portsmith-helper remove-aliases              Remove all 127.0.0.x aliases (except 127.0.0.1)
  portsmith-helper add-host <ip> <hostname>    Add /etc/hosts entry
  portsmith-helper remove-host <ip> <hostname> Remove specific /etc/hosts entry
  portsmith-helper remove-hosts                Remove all portsmith /etc/hosts entries

All IP addresses must be loopback addresses (127.0.0.0/8 or ::1).
This program must be run as root.
`)
}

func main() {
	checkRoot()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	var err error

	switch command {
	case "add-alias":
		if len(os.Args) != 3 {
			fmt.Fprintf(os.Stderr, "Error: add-alias requires IP argument\n")
			os.Exit(1)
		}
		err = addAlias(os.Args[2])

	case "remove-alias":
		if len(os.Args) != 3 {
			fmt.Fprintf(os.Stderr, "Error: remove-alias requires IP argument\n")
			os.Exit(1)
		}
		err = removeAlias(os.Args[2])

	case "add-host":
		if len(os.Args) != 4 {
			fmt.Fprintf(os.Stderr, "Error: add-host requires IP and hostname arguments\n")
			os.Exit(1)
		}
		err = addHost(os.Args[2], os.Args[3])

	case "remove-host":
		if len(os.Args) != 4 {
			fmt.Fprintf(os.Stderr, "Error: remove-host requires IP and hostname arguments\n")
			os.Exit(1)
		}
		err = removeHost(os.Args[2], os.Args[3])

	case "remove-hosts":
		err = removeHosts()

	case "remove-aliases":
		err = removeAliases()

	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
