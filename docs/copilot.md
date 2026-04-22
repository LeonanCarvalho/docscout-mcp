# GitHub Copilot

GitHub Copilot supports MCP servers through VS Code's built-in MCP configuration or via a repository-level `.github/copilot-mcp.json`.

## Per-Repository Setup

Create a `.github/copilot-mcp.json` file in your repository:

```json
{
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
```

## Global Setup (VS Code)

You can also configure it globally in VS Code. See [vscode.md](vscode.md) for the full VS Code settings approach.

### Using Docker

```json
{
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
```

## Usage

Once configured, GitHub Copilot Chat will have access to the `list_repos`, `search_docs`, and `get_file_content` tools to answer questions about your organization's documentation.
