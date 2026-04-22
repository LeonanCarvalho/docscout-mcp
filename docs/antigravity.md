# Antigravity (Google DeepMind)

Antigravity supports MCP servers through its configuration file.

## Setup

Edit your Antigravity settings file (typically located in your user directory):

**Windows:** `%USERPROFILE%\.gemini\settings.json`
**macOS/Linux:** `~/.gemini/settings.json`

Add an MCP server entry:

```json
{
  "mcpServers": {
    "docscout": {
      "command": "/path/to/docscout-mcp",
      "env": {
        "GITHUB_TOKEN": "github_pat_...",
        "GITHUB_ORG": "my-org",
        "SCAN_INTERVAL": "1h"
      }
    }
  }
}
```

### Using Docker

```json
{
  "mcpServers": {
    "docscout": {
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
```

## Usage

Once configured, the tools `list_repos`, `search_docs`, and `get_file_content` will be available in all Antigravity conversations.
