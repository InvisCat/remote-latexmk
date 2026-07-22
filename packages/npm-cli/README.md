# remote-latexmk npm launcher

This package runs the existing native remote-latexmk client. It does not
contain TeX Live and it does not download executable code from a lifecycle
script. npm selects a platform package through `optionalDependencies`.

```sh
npm exec --yes --ignore-scripts --package=remote-latexmk@VERSION -- \
  rlatexmk version
npm exec --yes --ignore-scripts --package=remote-latexmk@VERSION -- \
  rlatexmk mcp serve --stdio --root-from-client
```

Codex Desktop users can install the versioned Plugin without Codex CLI:

```sh
npx --yes --ignore-scripts remote-latexmk@VERSION plugin install codex
```

Run the same command with a newer `VERSION` to update the managed personal
marketplace source. Codex keeps its installed Plugin in a separate versioned
cache, so finish on the opened Plugin page by selecting **Install** or
**Update**, restart Codex if it is running, and start a new task. The command
does not edit that private cache. The saved server URL and API token remain
unchanged.

Codex CLI and Claude Code users can install the same Plugin from the
repository marketplace. It bundles the Skills and this npm-backed MCP command.
See the main Quick Start.

Save the server URL and token once on the client. The token prompt disables
terminal echo. The command verifies the server and token before storing the
login outside paper directories:

```sh
npx --yes --ignore-scripts remote-latexmk@VERSION auth login --server https://latex.example.edu
```

Paste the remote-latexmk API token when prompted. Do not put it in the command
itself.

For OpenCode, custom Agent environments, or a fixed-root setup, the legacy
installer can install the Skills and a local MCP entry for detected coding
agents:

```sh
npm exec --yes --ignore-scripts --package=remote-latexmk@VERSION -- \
  rlatexmk agent install \
  --project-root /absolute/paper \
  --server https://latex.example.edu \
  --token-file /absolute/path/to/latexmk-token
```

Use `--dry-run` to inspect planned file and configuration changes. Raw bearer
tokens are deliberately not accepted by the Agent installer.
