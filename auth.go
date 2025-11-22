package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

// MCPCredentials represents the keychain credentials structure
type MCPCredentials struct {
	MCPOAuth map[string]MCPOAuthEntry `json:"mcpOAuth"`
}

type MCPOAuthEntry struct {
	ServerName   string `json:"serverName"`
	ServerURL    string `json:"serverUrl"`
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    int64  `json:"expiresAt"`
}

func getMCPOAuthToken(serverName, serverURL string) (string, error) {
	// Read from macOS keychain
	cmd := exec.Command("security", "find-generic-password", "-s", "Claude Code-credentials", "-w")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	var creds MCPCredentials
	if err := json.Unmarshal(output, &creds); err != nil {
		return "", err
	}

	// Look for matching server
	for key, entry := range creds.MCPOAuth {
		if entry.ServerName == serverName || entry.ServerURL == serverURL ||
		   (len(key) > len(serverName) && key[:len(serverName)] == serverName) {
			return entry.AccessToken, nil
		}
	}

	return "", fmt.Errorf("no token found for server %s", serverName)
}
