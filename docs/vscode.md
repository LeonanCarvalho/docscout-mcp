# VS Code (Copilot Chat MCP)

VS Code supports MCP servers natively through its Copilot Chat extension.

## Setup

1. Open VS Code Settings (`Ctrl+Shift+P` → "Preferences: Open User Settings (JSON)").
2. Add the following MCP server configuration:

```json
{
  "mcp": {
    "servers": {
      "docscout": {
        "type": "stdio",
        "command": "/path/to/docscout-mcp",
        "env": {
          "GITHUB_TOKEN": "github_pat_...",
          "GITHUB_ORG": "my-org",
          "SCAN_INTERVAL": "1h"
        }
      }
    }
  }
}
```

### Using Docker

```json
{
  "mcp": {
    "servers": {
      "docscout": {
        "type": "stdio",
        "command": "docker",
        "args": [
          "run",
          "-i",
          "--rm",
          "-e",
          "GITHUB_TOKEN=github_pat_...",
          "-e",
          "GITHUB_ORG=my-org",
          "-e",
          "SCAN_INTERVAL=1h",
          "ghcr.io/doc-scout/mcp-server:latest"
        ]
      }
    }
  }
}
```

## Usage

Once configured, open Copilot Chat and use the `@docscout` agent, or simply ask questions — the tools `list_repos`, `search_docs`, and `get_file_content` will be available automatically.
