---
name: setup
description: Connect the remote-latexmk Codex or Claude Code plugin to a private server without putting credentials in a paper repository. Use when the plugin is first installed, the server URL or token file changes, or doctor reports missing client configuration.
---

# Set Up Remote LaTeX

Use the client's saved remote-latexmk login. Never ask the user to paste a
token into the Agent, and never write credentials to `.latexmk.json`, the
Agent workspace, shell history, or command arguments.

Use this launcher for every command in this workflow:

```sh
npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.1
```

## Workflow

1. Run `npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.1 doctor`. If it succeeds, the saved client login is ready; do not ask for more connection details.
2. If login is missing, ask only for the private server URL. Tell the user to run this command themselves in a local terminal:

   ```sh
   npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.1 auth login --server SERVER_URL
   ```

   The command uses a hidden token prompt. Do not run it through an Agent tool, ask the user for the token, or accept the token in chat. After the user finishes, run `doctor` again.
3. If the user explicitly wants to use an existing token file or private CA instead of interactive login, ask for the token-file path and optional CA-file path. Do not read, print, summarize, copy, or edit either file. Preview the non-secret configuration:

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

Interactive login writes a private token file and user configuration outside
the paper workspace. The user configuration stores only the token file path,
never the token value. The advanced setup command only records paths supplied
by the user after preview and confirmation.
