# Disclaimer

## No Warranty

DocScout-MCP is provided **"as is"**, without warranty of any kind, express or implied, including but not limited to the warranties of merchantability, fitness for a particular purpose, and non-infringement.

In no event shall the authors or copyright holders be liable for any claim, damages, or other liability — whether in an action of contract, tort, or otherwise — arising from, out of, or in connection with the software or the use or other dealings in the software.

## AI-Generated Output

This software exposes a knowledge graph to AI assistants (Claude, GitHub Copilot, Gemini CLI, ChatGPT, and others). **The accuracy of AI responses depends entirely on the data indexed from your repositories.** The authors make no guarantees regarding the correctness, completeness, or timeliness of any AI-generated output produced using this tool.

Do not rely on AI responses for critical infrastructure decisions without independent verification.

## GitHub API Usage

DocScout-MCP accesses GitHub repositories using a Personal Access Token (PAT) provided by the user. The authors are not responsible for:

- API rate limit overages or associated costs
- Unintended data exposure resulting from misconfigured token scopes
- Changes to GitHub's API that affect functionality

Always follow the principle of least privilege: grant only **read-only** access to `Contents` and `Metadata`.

## Security

While DocScout-MCP implements path-traversal protection and input sanitization, no software is entirely free of vulnerabilities. Users are responsible for:

- Securing their GitHub tokens and deployment environment
- Reviewing access controls before exposing the server over HTTP
- Reporting security issues responsibly via the project's security policy

See [SECURITY.md](security.md) for the full security policy and disclosure process.

## License

This software is licensed under the **GNU Affero General Public License v3.0 (AGPL-3.0)**. Any modifications or derivative works that are run as a network service must also be made available under the same license.

See [LICENSE](LICENSE) for the full license text.

---

Copyright &copy; 2026 Leonan Carvalho
