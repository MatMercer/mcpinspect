package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// ClaudeConfig represents the structure of .claude.json
type ClaudeConfig struct {
	Projects map[string]ProjectConfig `json:"projects"`
}

// ProjectConfig represents a project's configuration
type ProjectConfig struct {
	MCPServers map[string]MCPServer `json:"mcpServers"`
}

// MCPServer represents an MCP server configuration
type MCPServer struct {
	Type    string   `json:"type"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	URL     string   `json:"url,omitempty"`
}

func loadConfig(path string) (*ClaudeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config ClaudeConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}
