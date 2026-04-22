# Claude Desktop & CLI

DocScout-MCP can be added to Claude Desktop or used via the `claude` CLI tool using the standard `stdio` transport.

## Using the Claude CLI

The easiest way to use DocScout-MCP with the Claude CLI is by using the `mcp add` command to configure a local `stdio` server.

### Direct Binary

If you have built the binary (e.g., `docscout-mcp`):

```bash
claude mcp add --transport stdio \
  --env GITHUB_TOKEN=github_pat_... \
  --env GITHUB_ORG=my-org \
  --env SCAN_INTERVAL=30m \
  docscout-mcp \
  -- /path/to/docscout-mcp
```

### Using Go Run (Development)

To run directly from source:

```bash
claude mcp add --transport stdio docscout-mcp \
  --env GITHUB_TOKEN="github_pat_..." \
  --env GITHUB_ORG="<org/username>" \
  --env SCAN_INTERVAL="10m" \
  -- go run .
```

_Note: The `--` separates the `claude mcp add` arguments from the command that executes the server._

---

## Claude Desktop Configuration

If you prefer configuring Claude Desktop manually, modify your `claude_desktop_config.json` file.

**macOS:** `~/Library/Application Support/Claude/claude_desktop_config.json`
**Windows:** `%APPDATA%\Claude\claude_desktop_config.json`

Add the following to your `mcpServers` block:

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

### Using Docker in Claude Desktop

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

Once added, Claude will analyze the available MCP tools (`list_repos`, `search_docs`, `search_nodes`, `read_graph`, etc.) and automatically invoke them when you ask questions about your GitHub organization's documentation or architecture graph.
