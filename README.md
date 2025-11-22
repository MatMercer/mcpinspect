# mcpinspect

A tool to inspect MCP servers.

While configuring Claude Code commands, I noticed there was no easy way to specify allowed MCP tools for the project command, this tool makes it easy to list configured MCP servers.

**Warning:** Be careful when allowing MCP tools that perform actions (create, update, delete). Review tool descriptions before granting access.

## Supported OS and tools

* MacOS
  * Claude Code

## Commands

```
# Lists mcp servers (reads /Users/mmers/.claude.json$.projects[*].mcpServers* by default)
$ mcpinspect
NAME           TYPE   URL                         COMMAND
linear-server  http   https://mcp.linear.app/mcp  [N/A]
...

$ mcpinspect -c (or --config) custom claude location

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

## Credits

@ocervell - GO release skeleton

