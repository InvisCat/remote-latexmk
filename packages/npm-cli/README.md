# remote-latexmk npm launcher

This package runs the existing native remote-latexmk client. It does not
contain TeX Live and it does not download executable code from a lifecycle
script. npm selects a platform package through `optionalDependencies`.

```sh
npm exec --yes --ignore-scripts --package=remote-latexmk@VERSION -- \
  remote-latexmk version
npm exec --yes --ignore-scripts --package=remote-latexmk@VERSION -- \
  remote-latexmk mcp serve --stdio --project-root /absolute/paper
```

Install the Skills and a local MCP entry for detected coding agents:

```sh
npm exec --yes --ignore-scripts --package=remote-latexmk@VERSION -- \
  remote-latexmk agent install \
  --project-root /absolute/paper \
  --server https://latex.example.edu \
  --token-file /absolute/path/to/latexmk-token
```

Use `--dry-run` to inspect planned file and configuration changes. Raw bearer
tokens are deliberately not accepted by the Agent installer.
