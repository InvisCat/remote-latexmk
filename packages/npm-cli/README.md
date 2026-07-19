# remote-latexmk npm launcher

This package runs the existing native remote-latexmk client. It does not
contain TeX Live and it does not download executable code from a lifecycle
script. npm selects a platform package through `optionalDependencies`.

```sh
npm exec --yes --ignore-scripts --package=remote-latexmk@VERSION -- \
  remote-latexmk version
npm exec --yes --ignore-scripts --package=remote-latexmk@VERSION -- \
  remote-latexmk mcp serve --stdio --root-from-client
```

Codex and Claude Code users should normally install the repository's native
Plugin. It bundles the Skills and this npm-backed MCP command. See the main
Quick Start.

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
  remote-latexmk agent install \
  --project-root /absolute/paper \
  --server https://latex.example.edu \
  --token-file /absolute/path/to/latexmk-token
```

Use `--dry-run` to inspect planned file and configuration changes. Raw bearer
tokens are deliberately not accepted by the Agent installer.
