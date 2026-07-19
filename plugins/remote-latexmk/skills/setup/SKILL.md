---
name: setup
description: Connect the remote-latexmk Codex or Claude Code plugin to a private server without putting credentials in a paper repository. Use when the plugin is first installed, the server URL or token file changes, or doctor reports missing client configuration.
---

# Set Up Remote LaTeX

Store the server URL and credential file path in the user's remote-latexmk
configuration. Never write credentials to `.latexmk.json`, the Agent workspace,
shell history, or command arguments.

Use this launcher for every command in this workflow:

```sh
npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.1
```

## Workflow

1. Ask for the private server URL, the path of an existing token file, and an optional CA certificate file. Accept only an `http://` or `https://` URL. Prefer HTTPS or a private VPN/SSH tunnel.
2. Do not ask the user to paste the token. Do not read, print, summarize, copy, or edit the token file. The file must contain one non-empty line and, on Unix, have mode `0600`.
3. Preview the non-secret configuration without changing files:

   ```sh
   npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.1 setup --json --server SERVER_URL --token-file TOKEN_FILE
   ```

   Add `--ca-file CA_FILE` only when the server uses a private CA.
4. Show the resolved server URL and file paths from the preview. They are not secrets, but do not show token contents. Ask for explicit confirmation before applying the configuration.
5. Apply exactly the previewed values after confirmation:

   ```sh
   npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.1 setup --yes --json --server SERVER_URL --token-file TOKEN_FILE
   ```

   Add the same `--ca-file CA_FILE` used in the preview when applicable.
6. Run `npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.1 doctor`. If the server is unavailable, report the connection error. Do not weaken authentication, TLS verification, upload policy, or path boundaries to make the check pass.

The setup command writes the user configuration outside the paper workspace. It
stores only the token file path, never the token value. A later setup replaces
the server and file paths only after another preview and confirmation.
