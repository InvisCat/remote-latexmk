---
name: remote-latex-setup
description: Connect a remote-latexmk client or coding-agent Plugin to an existing private server. Use for first login, server URL or token changes, private CA configuration, or auth login and doctor errors. Do not use for server installation, paper compilation, or storage cleanup.
---

# Connect Remote LaTeX

Use the npm launcher `npx --yes --ignore-scripts remote-latexmk@0.4.1` for CLI fallbacks. Do not invoke the unrelated TeX Live `latexmk` command.

Never ask the user to paste a token into the Agent. Never write credentials to
`.latexmk.json`, the paper workspace, shell history, or command arguments.

## Workflow

1. If a saved login exists, run `npx --yes --ignore-scripts remote-latexmk@0.4.1 doctor`. It checks service health and identity, protocol compatibility, configured authentication, and local project-cache Git policy.
2. If login is missing or the server changed, ask only for the server address. Tell the user to run `npx --yes --ignore-scripts remote-latexmk@0.4.1 auth login --server SERVER` themselves in a trusted interactive terminal. A bare host and an explicit `http://` URL without a port use HTTP port 8080. An `https://` URL without a port uses the standard HTTPS port. Explicit schemes and ports are preserved.
3. Explain the order of `auth login`: it normalizes the address, checks public health, service identity, and protocol compatibility, and only then opens the hidden remote-latexmk API-token prompt. It verifies authenticated read access before saving the login. A failure does not replace the saved login.
4. If login fails, ask for the exact error text but never the token. Read [references/login-errors.md](references/login-errors.md) and give the narrow matching correction. Do not weaken TLS, authentication, network policy, or path boundaries.
5. After a successful login, run `npx --yes --ignore-scripts remote-latexmk@0.4.1 doctor` once to check the remaining local policy. Do not ask for more connection details when it succeeds.

For an existing user-managed token file or private CA, preview the non-secret
configuration before applying it:

```sh
npx --yes --ignore-scripts remote-latexmk@0.4.1 setup --json --server SERVER_URL --token-file TOKEN_FILE
npx --yes --ignore-scripts remote-latexmk@0.4.1 setup --yes --json --server SERVER_URL --token-file TOKEN_FILE
```

Add the same `--ca-file CA_FILE` to both commands when a private CA is needed.
Do not read, print, summarize, copy, or edit the token or CA file. Show the
resolved non-secret paths and ask for explicit confirmation before apply.
