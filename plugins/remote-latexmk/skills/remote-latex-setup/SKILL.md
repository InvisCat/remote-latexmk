---
name: remote-latex-setup
description: Connect a remote-latexmk client or coding-agent Plugin to an existing private server. Use for first login, server URL or token changes, private CA configuration, or auth login and doctor errors. Do not use for server installation, paper compilation, or storage cleanup.
---

# Connect Remote LaTeX

Use the npm launcher `npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.1` for CLI fallbacks. Do not invoke the unrelated TeX Live `latexmk` command.

Never ask the user to paste a token into the Agent. Never write credentials to
`.latexmk.json`, the paper workspace, shell history, or command arguments.

## Workflow

1. Run `npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.1 doctor`. It checks service health and identity, protocol compatibility, configured authentication, and local project-cache Git policy.
2. If login is missing or the server changed, ask only for the server URL. Tell the user to run `npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.1 auth login --server SERVER_URL` themselves in a trusted interactive terminal.
3. Explain that `auth login` reads the remote-latexmk API token with terminal echo disabled. It verifies the server and token before saving them. A verification failure does not replace the saved login.
4. If login fails, ask for the exact error text but never the token. Read [references/login-errors.md](references/login-errors.md) and give the narrow matching correction. Do not weaken TLS, authentication, network policy, or path boundaries.
5. After a successful login, run `npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.1 doctor` once to check the remaining local policy. Do not ask for more connection details when it succeeds.

For an existing user-managed token file or private CA, preview the non-secret
configuration before applying it:

```sh
npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.1 setup --json --server SERVER_URL --token-file TOKEN_FILE
npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.1 setup --yes --json --server SERVER_URL --token-file TOKEN_FILE
```

Add the same `--ca-file CA_FILE` to both commands when a private CA is needed.
Do not read, print, summarize, copy, or edit the token or CA file. Show the
resolved non-secret paths and ask for explicit confirmation before apply.
