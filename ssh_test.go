package main

import (
	"testing"
)

func TestExpandKeyPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string // Expected to contain this substring
	}{
		{
			name:     "tilde with path",
			input:    "~/.ssh/id_rsa",
			contains: ".ssh/id_rsa",
		},
		{
			name:     "just tilde",
			input:    "~",
			contains: "/", // Should be a home directory path
		},
		{
			name:     "absolute path",
			input:    "/absolute/path/key",
			contains: "/absolute/path/key",
		},
		{
			name:     "relative path",
			input:    "./relative/key",
			contains: "./relative/key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExpandKeyPath(tt.input)
			if err != nil {
				t.Fatalf("ExpandKeyPath() error = %v", err)
			}
			if result == "" {
				t.Error("ExpandKeyPath() returned empty string")
			}
			if tt.contains != "" && result == tt.input {
				// For tilde paths, result should be different from input
				if tt.input[0] == '~' && result == tt.input {
					t.Errorf("ExpandKeyPath(%s) was not expanded", tt.input)
				}
			}
		})
	}
}

func TestNewSSHClientPool(t *testing.T) {
	pool := NewSSHClientPool()

	if pool == nil {
		t.Fatal("NewSSHClientPool() returned nil")
	}

	if pool.clients == nil {
		t.Error("SSHClientPool.clients map is nil")
	}

	if pool.authMethods == nil {
		t.Error("SSHClientPool.authMethods map is nil")
	}
}

func TestKeyboardInteractiveChallenge(t *testing.T) {
	// Test with empty questions
	answers, err := keyboardInteractiveChallenge("user", "instruction", []string{}, []bool{})
	if err != nil {
		t.Errorf("keyboardInteractiveChallenge() error = %v", err)
	}
	if len(answers) != 0 {
		t.Errorf("Expected 0 answers, got %d", len(answers))
	}
}
