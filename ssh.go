package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/user"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/term"
)

// SSHClientPool manages SSH client connections with connection pooling
type SSHClientPool struct {
	clients     map[string]*ssh.Client
	mu          sync.Mutex
	authMethods map[string][]ssh.AuthMethod
	authMu      sync.Mutex
}

// NewSSHClientPool creates a new SSH client pool
func NewSSHClientPool() *SSHClientPool {
	return &SSHClientPool{
		clients:     make(map[string]*ssh.Client),
		authMethods: make(map[string][]ssh.AuthMethod),
	}
}

// LoadAuthMethods loads SSH authentication methods for the given key path and optional identity agent
func (pool *SSHClientPool) LoadAuthMethods(keyPath, identityAgent string) error {
	pool.authMu.Lock()
	defer pool.authMu.Unlock()

	cacheKey := keyPath
	if identityAgent != "" {
		cacheKey = keyPath + "|" + identityAgent
	}

	if _, exists := pool.authMethods[cacheKey]; exists {
		return nil
	}

	authMethods, err := loadSSHAuthMethods(keyPath, identityAgent)
	if err != nil {
		return fmt.Errorf("failed to load SSH auth methods: %w", err)
	}

	pool.authMethods[cacheKey] = authMethods
	return nil
}

// LoadAuthMethodsWithRetry attempts to load SSH auth methods with unlimited retries
// This is used when waiting for an SSH agent to become available (e.g., at startup)
func (pool *SSHClientPool) LoadAuthMethodsWithRetry(keyPath, identityAgent string, retryInterval time.Duration) error {
	attempt := 0

	for {
		attempt++
		err := pool.LoadAuthMethods(keyPath, identityAgent)
		if err == nil {
			if attempt > 1 {
				log.Printf("Successfully loaded SSH auth methods for %s after %d attempts", keyPath, attempt)
			}
			return nil
		}

		// Check if error is agent-related (socket not available yet)
		errStr := err.Error()
		isAgentSocketError := strings.Contains(errStr, "connection refused") ||
			strings.Contains(errStr, "no such file")

		if !isAgentSocketError {
			// Not an agent availability issue, fail immediately
			return fmt.Errorf("failed to load SSH auth methods: %w", err)
		}

		if attempt == 1 {
			log.Printf("Waiting for SSH agent to become available (will retry every %s)...", retryInterval)
		} else if attempt%6 == 0 {
			// Log every 30 seconds (6 attempts * 5s interval)
			log.Printf("Still waiting for SSH agent... (%d attempts so far)", attempt)
		}

		time.Sleep(retryInterval)
	}
}

// GetClient returns an SSH client for the given jump host, creating one if needed
func (pool *SSHClientPool) GetClient(jumpHost string, jumpPort int, keyPath, identityAgent string) (*ssh.Client, error) {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	clientKey := fmt.Sprintf("%s:%d", jumpHost, jumpPort)

	if client, exists := pool.clients[clientKey]; exists {
		return client, nil
	}

	cacheKey := keyPath
	if identityAgent != "" {
		cacheKey = keyPath + "|" + identityAgent
	}

	pool.authMu.Lock()
	authMethods, exists := pool.authMethods[cacheKey]
	pool.authMu.Unlock()

	// Lazy load auth methods if not cached
	if !exists || len(authMethods) == 0 {
		log.Printf("Auth methods not loaded for %s, loading now...", keyPath)

		// Unlock the main mutex while we load auth methods to avoid blocking other operations
		pool.mu.Unlock()

		// Try to load with unlimited retries (5 seconds between attempts)
		// This will wait indefinitely for the SSH agent to become available
		err := pool.LoadAuthMethodsWithRetry(keyPath, identityAgent, 5*time.Second)

		// Re-lock before continuing
		pool.mu.Lock()

		if err != nil {
			return nil, fmt.Errorf("failed to load SSH auth methods: %w", err)
		}

		// Retrieve the newly loaded auth methods
		pool.authMu.Lock()
		authMethods = pool.authMethods[cacheKey]
		pool.authMu.Unlock()

		if len(authMethods) == 0 {
			return nil, fmt.Errorf("no authentication methods available for key %s after loading", keyPath)
		}
	}

	// Get current user
	currentUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}

	log.Printf("Connecting as user %s with %d auth method(s)", currentUser.Username, len(authMethods))

	sshConfig := &ssh.ClientConfig{
		User:            currentUser.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	// Build jump host address with port
	jumpAddr := fmt.Sprintf("%s:%d", jumpHost, jumpPort)

	// Retry SSH connection with exponential backoff for agent errors
	// This handles cases where the agent responds but fails during handshake
	// (e.g., 1Password waiting for Touch ID unlock, agent initialization)
	var client *ssh.Client
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		client, err = ssh.Dial("tcp", jumpAddr, sshConfig)
		if err == nil {
			break
		}

		// Check if error is agent-related during handshake
		errStr := err.Error()
		isAgentError := strings.Contains(errStr, "agent:") ||
			strings.Contains(errStr, "broken pipe") ||
			strings.Contains(errStr, "EOF")

		if !isAgentError || attempt == maxRetries {
			return nil, fmt.Errorf("failed to dial jump host %s (attempt %d/%d): %w", jumpAddr, attempt, maxRetries, err)
		}

		delay := time.Duration(attempt*3) * time.Second
		log.Printf("SSH connection failed (attempt %d/%d): %v. Agent may need unlock. Retrying in %s...",
			attempt, maxRetries, err, delay)

		// Unlock to allow other operations while we wait
		pool.mu.Unlock()
		time.Sleep(delay)
		pool.mu.Lock()
	}

	pool.clients[clientKey] = client
	log.Printf("SSH connection established to %s as %s", jumpAddr, currentUser.Username)

	return client, nil
}

// RemoveClient removes a stale client from the pool
func (pool *SSHClientPool) RemoveClient(jumpHost string, jumpPort int) {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	clientKey := fmt.Sprintf("%s:%d", jumpHost, jumpPort)
	if client, exists := pool.clients[clientKey]; exists {
		client.Close()
		delete(pool.clients, clientKey)
		log.Printf("Removed stale SSH connection to %s", clientKey)
	}
}

// Close closes all SSH clients in the pool
func (pool *SSHClientPool) Close() {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	for jumpAddr, client := range pool.clients {
		log.Printf("Closing connection to %s", jumpAddr)
		client.Close()
	}
}

// ExpandKeyPath expands ~ in key paths to the home directory
func ExpandKeyPath(keyPath string) (string, error) {
	if strings.HasPrefix(keyPath, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		return homeDir + keyPath[1:], nil
	}
	if keyPath == "~" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		return homeDir, nil
	}
	return keyPath, nil
}

// loadSSHAuthMethods loads SSH authentication methods from agent or key file
func loadSSHAuthMethods(keyPath, identityAgent string) ([]ssh.AuthMethod, error) {
	authMethods := make([]ssh.AuthMethod, 0)

	// Priority: identity_agent config > SSH_AUTH_SOCK env > key file
	agentSocket := ""
	if identityAgent != "" {
		expandedAgent, err := ExpandKeyPath(identityAgent)
		if err != nil {
			log.Printf("Failed to expand identity agent path %s: %v", identityAgent, err)
		} else {
			agentSocket = expandedAgent
			log.Printf("Using configured identity agent: %s", agentSocket)
		}
	}

	if agentSocket == "" {
		if sshAuthSock := os.Getenv("SSH_AUTH_SOCK"); sshAuthSock != "" {
			agentSocket = sshAuthSock
			log.Printf("Using SSH_AUTH_SOCK agent")
		}
	}

	if agentSocket != "" {
		agentConn, err := net.Dial("unix", agentSocket)
		if err == nil {
			agentClient := agent.NewClient(agentConn)
			signers, err := agentClient.Signers()
			if err == nil && len(signers) > 0 {
				authMethods = append(authMethods, ssh.PublicKeys(signers...))
				log.Printf("SSH agent connected with %d key(s)", len(signers))
				authMethods = append(authMethods, ssh.KeyboardInteractive(keyboardInteractiveChallenge))
				return authMethods, nil
			}
			agentConn.Close()
		} else {
			log.Printf("Failed to connect to SSH agent at %s: %v", agentSocket, err)
		}
	}

	// Fall back to key file
	log.Printf("SSH agent has no keys, loading from key file...")
	expandedKeyPath, err := ExpandKeyPath(keyPath)
	if err != nil {
		return nil, err
	}
	keyPath = expandedKeyPath
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("could not read key file %s: %w", keyPath, err)
	}

	// Try to parse the key without passphrase first
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		// Check if it's a passphrase-protected key
		if _, ok := err.(*ssh.PassphraseMissingError); ok {
			fmt.Printf("Enter passphrase for %s: ", keyPath)
			passphrase, err := term.ReadPassword(int(syscall.Stdin))
			fmt.Println() // Print newline after password input
			if err != nil {
				return nil, fmt.Errorf("failed to read passphrase: %w", err)
			}
			signer, err = ssh.ParsePrivateKeyWithPassphrase(key, passphrase)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt key with passphrase: %w", err)
			}
		} else {
			return nil, fmt.Errorf("could not parse key file %s: %w", keyPath, err)
		}
	}

	authMethods = append(authMethods, ssh.PublicKeys(signer))
	// Add keyboard-interactive for 2FA support
	authMethods = append(authMethods, ssh.KeyboardInteractive(keyboardInteractiveChallenge))
	log.Printf("Loaded SSH key from %s", keyPath)
	return authMethods, nil
}

// keyboardInteractiveChallenge handles keyboard-interactive authentication challenges
func keyboardInteractiveChallenge(user, instruction string, questions []string, echos []bool) ([]string, error) {
	if len(questions) == 0 {
		return []string{}, nil
	}

	answers := make([]string, len(questions))
	for i, question := range questions {
		fmt.Printf("%s", question)
		if echos[i] {
			// Echo enabled - read normally
			var answer string
			fmt.Scanln(&answer)
			answers[i] = answer
		} else {
			// Echo disabled - read password
			password, err := term.ReadPassword(int(syscall.Stdin))
			fmt.Println()
			if err != nil {
				return nil, fmt.Errorf("failed to read password: %w", err)
			}
			answers[i] = string(password)
		}
	}
	return answers, nil
}
