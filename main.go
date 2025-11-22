package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	mcp "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/stdio"
	"github.com/spf13/cobra"
)

var configPath string

func main() {
	rootCmd := &cobra.Command{
		Use:   "mcpinspect [server-name]",
		Short: "Inspect MCP servers configured in Claude",
		Long: `mcpinspect is a tool to inspect MCP servers configured for Claude Code.
It reads the Claude configuration file and displays information about configured MCP servers.

Without arguments, it lists all MCP servers across all projects.
With a server name argument, it shows detailed information about that specific server.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			config, err := loadConfig(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if len(args) == 0 {
				return listServers(config)
			}
			return inspectServer(config, args[0])
		},
	}

	// Get default config path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting home directory: %v\n", err)
		os.Exit(1)
	}
	defaultConfig := filepath.Join(homeDir, ".claude.json")

	rootCmd.Flags().StringVarP(&configPath, "config", "c", defaultConfig, "path to Claude config file")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// ServerInfo holds aggregated server information
type ServerInfo struct {
	Name     string
	Type     string
	URL      string
	Command  string
	Args     []string
	Projects []string
}

func listServers(config *ClaudeConfig) error {
	// Aggregate servers across all projects
	servers := make(map[string]*ServerInfo)

	for projectPath, project := range config.Projects {
		for name, server := range project.MCPServers {
			if existing, ok := servers[name]; ok {
				existing.Projects = append(existing.Projects, projectPath)
			} else {
				info := &ServerInfo{
					Name:     name,
					Type:     server.Type,
					URL:      server.URL,
					Command:  server.Command,
					Args:     server.Args,
					Projects: []string{projectPath},
				}
				servers[name] = info
			}
		}
	}

	if len(servers) == 0 {
		fmt.Println("No MCP servers configured.")
		return nil
	}

	// Sort server names for consistent output
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)

	// Print table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tURL\tCOMMAND\tARGS")

	for _, name := range names {
		info := servers[name]
		url := info.URL
		if url == "" {
			url = "[N/A]"
		}
		command := info.Command
		if command == "" {
			command = "[N/A]"
		}
		args := "[N/A]"
		if len(info.Args) > 0 {
			args = strings.Join(info.Args, " ")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", info.Name, info.Type, url, command, args)
	}

	w.Flush()
	return nil
}

func inspectServer(config *ClaudeConfig, serverName string) error {
	var foundServer *MCPServer

	for _, project := range config.Projects {
		if server, ok := project.MCPServers[serverName]; ok {
			foundServer = &server
			break
		}
	}

	if foundServer == nil {
		return fmt.Errorf("server '%s' not found", serverName)
	}

	// Connect to the server and get capabilities
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, cleanup, err := connectToServer(ctx, foundServer, serverName)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	// Initialize
	initResp, err := client.Initialize(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize: %w", err)
	}

	// List tools
	tools, err := client.ListTools(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to list tools: %w", err)
	}

	// Sort tools alphabetically
	sort.Slice(tools.Tools, func(i, j int) bool {
		return tools.Tools[i].Name < tools.Tools[j].Name
	})

	// Print tools table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tDESCRIPTION")

	for _, tool := range tools.Tools {
		desc := ""
		if tool.Description != nil {
			desc = *tool.Description
		}
		fmt.Fprintf(w, "%s\t%s\n", tool.Name, desc)
	}

	w.Flush()

	// Print summary
	fmt.Println()
	serverInfo := initResp.ServerInfo.Name
	if initResp.ServerInfo.Version != "" {
		serverInfo += " v" + initResp.ServerInfo.Version
	}
	fmt.Printf("%d tools | %s | %s\n", len(tools.Tools), foundServer.Type, serverInfo)

	return nil
}

func connectToServer(ctx context.Context, server *MCPServer, serverName string) (*mcp.Client, func(), error) {
	switch server.Type {
	case "stdio":
		return connectStdio(ctx, server)
	case "http":
		return connectHTTP(ctx, server, serverName)
	case "sse":
		return connectSSE(ctx, server, serverName)
	default:
		return nil, nil, fmt.Errorf("unsupported server type: %s", server.Type)
	}
}

func connectStdio(ctx context.Context, server *MCPServer) (*mcp.Client, func(), error) {
	cmd := exec.CommandContext(ctx, server.Command, server.Args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("failed to start command: %w", err)
	}

	innerTransport := stdio.NewStdioServerTransportWithIO(stdout, stdin)
	transport := NewCleaningStdioTransport(innerTransport)
	client := mcp.NewClient(transport)

	cleanup := func() {
		stdin.Close()
		cmd.Process.Kill()
		cmd.Wait()
	}

	return client, cleanup, nil
}

func connectHTTP(ctx context.Context, server *MCPServer, serverName string) (*mcp.Client, func(), error) {
	transport := NewSSEClientTransport(server.URL)

	// Try to get OAuth token from keychain
	token, err := getMCPOAuthToken(serverName, server.URL)
	if err == nil && token != "" {
		transport.WithHeader("Authorization", "Bearer "+token)
	}

	client := mcp.NewClient(transport)
	return client, nil, nil
}

func connectSSE(ctx context.Context, server *MCPServer, serverName string) (*mcp.Client, func(), error) {
	transport := NewTraditionalSSETransport(server.URL)

	// Try to get OAuth token from keychain
	token, err := getMCPOAuthToken(serverName, server.URL)
	if err == nil && token != "" {
		transport.WithHeader("Authorization", "Bearer "+token)
	}

	// Start the SSE connection (GET /sse and wait for endpoint)
	if err := transport.Start(ctx); err != nil {
		return nil, nil, fmt.Errorf("failed to start SSE transport: %w", err)
	}

	client := mcp.NewClient(transport)

	cleanup := func() {
		transport.Close()
	}

	return client, cleanup, nil
}
