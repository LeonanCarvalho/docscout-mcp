# Gemini CLI

The Gemini CLI supports MCP servers through its settings configuration.

## Setup

Edit (or create) the Gemini CLI settings file:

**Windows:** `%USERPROFILE%\.gemini\settings.json`
**macOS/Linux:** `~/.gemini/settings.json`

```json
{
  "mcpServers": {
    "docscout": {
      "command": "/path/to/docscout-mcp",
      "env": {
        "GITHUB_TOKEN": "github_pat_...",
        "GITHUB_ORG": "my-org",
        "SCAN_INTERVAL": "30m"
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
        "SCAN_INTERVAL=30m",
        "ghcr.io/doc-scout/mcp-server:latest"
      ]
    }
  }
}
```

## Usage

Once configured, use Gemini CLI normally. The MCP tools will be available for the AI to invoke when answering questions about your organization's documentation.
