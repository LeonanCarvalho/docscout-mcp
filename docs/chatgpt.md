# ChatGPT Desktop

ChatGPT Desktop supports MCP servers for local tool integration.

## Setup

1. Open ChatGPT Desktop.
2. Go to **Settings** → **MCP Servers** (or **Beta Features** → **MCP**).
3. Click **"Add Server"** and configure:

| Field         | Value                   |
| ------------- | ----------------------- |
| **Name**      | `docscout`              |
| **Transport** | `stdio`                 |
| **Command**   | `/path/to/docscout-mcp` |

4. Add the following environment variables:

| Variable        | Value            |
| --------------- | ---------------- |
| `GITHUB_TOKEN`  | `github_pat_...` |
| `GITHUB_ORG`    | `my-org`         |
| `SCAN_INTERVAL` | `1h`             |

### Using Docker

Set the command to `docker` and add the arguments:

| Field         | Value                                                                                                                     |
| ------------- | ------------------------------------------------------------------------------------------------------------------------- |
| **Command**   | `docker`                                                                                                                  |
| **Arguments** | `run -i --rm -e GITHUB_TOKEN=github_pat_... -e GITHUB_ORG=my-org -e SCAN_INTERVAL=1h ghcr.io/doc-scout/mcp-server:latest` |

## Usage

Once configured, start a new conversation in ChatGPT Desktop. The AI will automatically have access to the `list_repos`, `search_docs`, and `get_file_content` tools to explore your organization's documentation.

> **Note:** MCP support in ChatGPT Desktop may be a beta feature. Ensure you have enabled it in Settings → Beta Features.
