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
	stateDir    = "/var/run/portsmith"
	aliasesFile = "/var/run/portsmith/aliases"
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

func ensureStateDir() error {
	return os.MkdirAll(stateDir, 0755)
}

func addAliasToState(ip string) error {
	if err := ensureStateDir(); err != nil {
		return err
	}

	aliases, err := loadAliases()
	if err != nil {
		return err
	}

	for _, existing := range aliases {
		if existing == ip {
			return nil
		}
	}

	f, err := os.OpenFile(aliasesFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open state file: %v", err)
	}
	defer f.Close()

	if _, err := f.WriteString(ip + "\n"); err != nil {
		return fmt.Errorf("failed to write to state file: %v", err)
	}

	return nil
}

func removeAliasFromState(ip string) error {
	aliases, err := loadAliases()
	if err != nil {
		return err
	}

	var newAliases []string
	for _, existing := range aliases {
		if existing != ip {
			newAliases = append(newAliases, existing)
		}
	}

	content := strings.Join(newAliases, "\n")
	if len(newAliases) > 0 {
		content += "\n"
	}

	return os.WriteFile(aliasesFile, []byte(content), 0644)
}

func loadAliases() ([]string, error) {
	content, err := os.ReadFile(aliasesFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read state file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	var aliases []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			aliases = append(aliases, line)
		}
	}

	return aliases, nil
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
		if err := addAliasToState(ip); err != nil {
			return fmt.Errorf("failed to track alias in state: %v", err)
		}
		return nil
	}

	// Add alias
	cmd = exec.Command("ifconfig", "lo0", "alias", ip, "up")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add loopback alias: %v", err)
	}

	if err := addAliasToState(ip); err != nil {
		return fmt.Errorf("failed to track alias in state: %v", err)
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

	if err := removeAliasFromState(ip); err != nil {
		return fmt.Errorf("failed to remove alias from state: %v", err)
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
	aliases, err := loadAliases()
	if err != nil {
		return fmt.Errorf("failed to load aliases state: %v", err)
	}

	if len(aliases) == 0 {
		fmt.Println("No portsmith aliases to remove")
		return nil
	}

	removed := 0
	for _, ip := range aliases {
		cmd := exec.Command("ifconfig", "lo0", "-alias", ip)
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to remove alias %s: %v\n", ip, err)
			continue
		}
		fmt.Printf("Removed loopback alias: %s\n", ip)
		removed++
	}

	// Clear the state file
	if err := os.WriteFile(aliasesFile, []byte{}, 0644); err != nil {
		return fmt.Errorf("failed to clear state file: %v", err)
	}

	fmt.Printf("Removed %d portsmith aliases\n", removed)
	return nil
}

func addPFRedirect(ip string, fromPort, toPort int) error {
	if err := validateIP(ip); err != nil {
		return err
	}

	if fromPort < 1 || fromPort > 65535 || toPort < 1 || toPort > 65535 {
		return fmt.Errorf("invalid port range: from=%d to=%d", fromPort, toPort)
	}

	if runtime.GOOS != "darwin" {
		return fmt.Errorf("unsupported OS: %s (only macOS is supported)", runtime.GOOS)
	}

	anchorFile := "/etc/pf.anchors/portsmith"

	// Create the pf redirect rule
	rule := fmt.Sprintf("rdr pass on lo0 inet proto tcp from any to %s port %d -> %s port %d\n", ip, fromPort, ip, toPort)

	// Read existing rules from anchor file
	var existingContent string
	if content, err := os.ReadFile(anchorFile); err == nil {
		existingContent = string(content)
	}

	// Check if rule already exists
	if strings.Contains(existingContent, strings.TrimSpace(rule)) {
		fmt.Printf("pf redirect already exists: %s:%d -> %s:%d\n", ip, fromPort, ip, toPort)
		return nil
	}

	// Append new rule
	newContent := existingContent + rule

	// Write to anchor file
	if err := os.WriteFile(anchorFile, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write anchor file: %v", err)
	}

	// Check if anchor is referenced in pf.conf
	pfConfContent, err := os.ReadFile("/etc/pf.conf")
	if err != nil {
		return fmt.Errorf("failed to read /etc/pf.conf: %v", err)
	}

	needsReload := false
	if !strings.Contains(string(pfConfContent), "rdr-anchor \"portsmith\"") {
		// Add anchor reference to pf.conf
		lines := strings.Split(string(pfConfContent), "\n")
		var newLines []string
		anchorAdded := false

		for _, line := range lines {
			newLines = append(newLines, line)
			// Add rdr-anchor after the com.apple rdr-anchor
			if strings.Contains(line, "rdr-anchor \"com.apple/*\"") && !anchorAdded {
				newLines = append(newLines, "rdr-anchor \"portsmith\"")
				anchorAdded = true
			}
		}

		if !anchorAdded {
			return fmt.Errorf("could not find appropriate location in /etc/pf.conf to add anchor")
		}

		newPfConf := strings.Join(newLines, "\n")
		if err := os.WriteFile("/etc/pf.conf", []byte(newPfConf), 0644); err != nil {
			return fmt.Errorf("failed to update /etc/pf.conf: %v", err)
		}
		needsReload = true
	}

	// Load the anchor
	cmd := exec.Command("pfctl", "-a", "portsmith", "-f", anchorFile)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to load pf anchor: %v", err)
	}

	// If we updated pf.conf, reload it to activate the anchor
	if needsReload {
		cmd = exec.Command("pfctl", "-f", "/etc/pf.conf")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to reload pf.conf: %v", err)
		}
	}

	fmt.Printf("Added pf redirect: %s:%d -> %s:%d\n", ip, fromPort, ip, toPort)
	return nil
}

func removePFRedirect(ip string, fromPort, toPort int) error {
	if err := validateIP(ip); err != nil {
		return err
	}

	if runtime.GOOS != "darwin" {
		return fmt.Errorf("unsupported OS: %s (only macOS is supported)", runtime.GOOS)
	}

	anchorFile := "/etc/pf.anchors/portsmith"

	// Read existing rules
	content, err := os.ReadFile(anchorFile)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("No pf redirects to remove for %s:%d\n", ip, fromPort)
			return nil
		}
		return fmt.Errorf("failed to read anchor file: %v", err)
	}

	// Filter out the rule we want to remove
	targetRule := fmt.Sprintf("rdr pass on lo0 inet proto tcp from any to %s port %d -> %s port %d", ip, fromPort, ip, toPort)
	lines := strings.Split(string(content), "\n")
	var newLines []string

	for _, line := range lines {
		if !strings.Contains(line, targetRule) && strings.TrimSpace(line) != "" {
			newLines = append(newLines, line)
		}
	}

	// Write back the filtered rules
	newContent := strings.Join(newLines, "\n")
	if len(newLines) > 0 {
		newContent += "\n"
	}

	if err := os.WriteFile(anchorFile, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write anchor file: %v", err)
	}

	// Reload the anchor
	cmd := exec.Command("pfctl", "-a", "portsmith", "-f", anchorFile)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to reload pf anchor: %v", err)
	}

	fmt.Printf("Removed pf redirect: %s:%d -> %s:%d\n", ip, fromPort, ip, toPort)
	return nil
}

func removePFRedirects() error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("unsupported OS: %s (only macOS is supported)", runtime.GOOS)
	}

	anchorFile := "/etc/pf.anchors/portsmith"

	// Check if file exists
	if _, err := os.Stat(anchorFile); os.IsNotExist(err) {
		fmt.Println("No pf redirects to remove")
		return nil
	}

	// Clear the anchor file
	if err := os.WriteFile(anchorFile, []byte{}, 0644); err != nil {
		return fmt.Errorf("failed to clear anchor file: %v", err)
	}

	// Flush the anchor
	cmd := exec.Command("pfctl", "-a", "portsmith", "-F", "nat")
	if err := cmd.Run(); err != nil {
		// Not fatal if flush fails when there are no rules
		fmt.Fprintf(os.Stderr, "Warning: failed to flush pf anchor: %v\n", err)
	}

	fmt.Println("Removed all portsmith pf redirects")
	return nil
}


func printUsage() {
	fmt.Fprintf(os.Stderr, `portsmith-helper - Privileged operations helper for portsmith

Usage:
  portsmith-helper add-alias <ip>                      Add loopback alias
  portsmith-helper remove-alias <ip>                   Remove specific loopback alias
  portsmith-helper remove-aliases                      Remove all portsmith-managed aliases
  portsmith-helper add-host <ip> <hostname>            Add /etc/hosts entry
  portsmith-helper remove-host <ip> <hostname>         Remove specific /etc/hosts entry
  portsmith-helper remove-hosts                        Remove all portsmith /etc/hosts entries
  portsmith-helper add-pf-redirect <ip> <from> <to>    Add pf port redirect
  portsmith-helper remove-pf-redirect <ip> <from> <to> Remove specific pf redirect
  portsmith-helper remove-pf-redirects                 Remove all portsmith pf redirects

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

	case "add-pf-redirect":
		if len(os.Args) != 5 {
			fmt.Fprintf(os.Stderr, "Error: add-pf-redirect requires IP, from-port, and to-port arguments\n")
			os.Exit(1)
		}
		var fromPort, toPort int
		if _, e := fmt.Sscanf(os.Args[3], "%d", &fromPort); e != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid from-port: %s\n", os.Args[3])
			os.Exit(1)
		}
		if _, e := fmt.Sscanf(os.Args[4], "%d", &toPort); e != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid to-port: %s\n", os.Args[4])
			os.Exit(1)
		}
		err = addPFRedirect(os.Args[2], fromPort, toPort)

	case "remove-pf-redirect":
		if len(os.Args) != 5 {
			fmt.Fprintf(os.Stderr, "Error: remove-pf-redirect requires IP, from-port, and to-port arguments\n")
			os.Exit(1)
		}
		var fromPort, toPort int
		if _, e := fmt.Sscanf(os.Args[3], "%d", &fromPort); e != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid from-port: %s\n", os.Args[3])
			os.Exit(1)
		}
		if _, e := fmt.Sscanf(os.Args[4], "%d", &toPort); e != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid to-port: %s\n", os.Args[4])
			os.Exit(1)
		}
		err = removePFRedirect(os.Args[2], fromPort, toPort)

	case "remove-pf-redirects":
		err = removePFRedirects()

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
