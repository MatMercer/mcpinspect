# mcpinspect

A tool to inspect MCP servers.

While configuring Claude Code commands, I noticed there was no easy way to specify allowed MCP tools for the project command, this tool makes it easy to list configured MCP servers.

**Warning:** Be careful when allowing MCP tools that perform actions (create, update, delete). Review tool descriptions before granting access.

## Supported OS and tools

* MacOS
  * Claude Code

## Installation

### macOS (Apple Silicon)

```bash
curl -L https://github.com/MatMercer/mcpinspect/releases/latest/download/mcpinspect_macOS_arm64.zip -o mcpinspect.zip
unzip mcpinspect.zip
chmod +x mcpinspect
sudo mv mcpinspect /usr/local/bin/

# zsh requires this for auto complete
rehash
```

## Usage

```
mcpinspect [server-name] [flags]

Flags:
  -c, --config string   path to Claude config file (default "~/.claude.json")
  -h, --help            help for mcpinspect
```

## Examples

### List all configured MCP servers

```
$ mcpinspect
NAME           TYPE   URL                         COMMAND
linear-server  http   https://mcp.linear.app/mcp  [N/A]
...
```

### Inspect a specific server's tools

```
$ mcpinspect linear-server
NAME                  DESCRIPTION
create_comment        Create a comment on a specific Linear issue
create_issue          Create a new Linear issue
create_issue_label    Create a new Linear issue label
create_project        Create a new project in Linear
get_document          Retrieve a Linear document by ID or slug
get_issue             Retrieve detailed information about an issue by ID
...

23 tools | http | Linear MCP v1.0.0
```

### Use a custom config file

```
$ mcpinspect -c /path/to/custom/claude.json
```

## Credits

@ocervell - GO release skeleton

