# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

mcpinspect is a CLI tool to inspect MCP (Model Context Protocol) servers configured for Claude Code. It reads the Claude configuration file (~/.claude.json) and displays information about configured MCP servers and their tools.

## Build Commands

```bash
go build .           # Build the binary
./mcpinspect         # List all configured MCP servers
./mcpinspect <name>  # Inspect a specific server's tools
```

## Architecture

- **main.go**: CLI entry point (cobra), server listing, tool inspection
- **config.go**: Claude config file parsing and types
- **transport.go**: Streamable HTTP transport for MCP protocol (JSON-RPC over HTTP)
- **sse.go**: Traditional SSE transport (GET /sse for stream, POST to endpoint)
- **auth.go**: OAuth token retrieval from macOS keychain

## Key Dependencies

- `github.com/metoro-io/mcp-golang` - MCP client library
- `github.com/spf13/cobra` - CLI framework
