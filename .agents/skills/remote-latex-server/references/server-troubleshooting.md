# Server troubleshooting

Start with read-only checks and stop after identifying the failing layer.

## Native service

```sh
~/.remote-latexmk/bin/remote-latexmkctl status
~/.remote-latexmk/bin/remote-latexmkctl doctor
~/.remote-latexmk/bin/remote-latexmkctl logs
```

If the user service stops after logout or boot, report the systemd user
lingering requirement to the server administrator. Do not change system-wide
service policy without authorization.

Inspect the saved settings before changing them:

```sh
~/.remote-latexmk/bin/remote-latexmkctl version
~/.remote-latexmk/bin/remote-latexmkctl doctor
```

To change only the native listener, preview first and show the exact old and
new values:

```sh
~/.remote-latexmk/bin/remote-latexmkctl configure --listen HOST:PORT
```

After explicit approval, repeat it with `--yes`. The command restarts only a
running service and restores the previous listener if restart fails. Do not
edit `server.env` as the normal recovery path.

The installer owns `config/server.env`. Persistent database, CORS, timeout,
resource-limit, and retention tuning belongs in
`config/server.override.env`, which accepts only the installer's closed key
list. Never echo an override value while troubleshooting because it may contain
a database credential.

## Docker service

```sh
docker compose -f compose.yaml ps
docker compose -f compose.yaml logs --tail 200 server gateway
```

Confirm that both `server` and `gateway` are running. The gateway owns the
published endpoint; do not expose the compiler container as a shortcut.

## Failure map

| Symptom | Check | Safe correction |
| --- | --- | --- |
| Connection refused or timeout | Service status, configured listen address, port, LAN/VPN route, firewall, or SSH tunnel | Correct the failing layer. Keep loopback for tunnels or bind one explicit private address for direct access. |
| Health works locally but not remotely | Whether the service listens only on `127.0.0.1` and whether the client uses a reachable address | Choose VPN, SSH tunnel, or an explicit private listen address. Do not default to `0.0.0.0`. |
| TLS or `x509` failure | Proxy hostname, certificate chain, and client CA configuration | Fix the certificate or distribute the intended CA. Do not disable verification. |
| Wrong service identity | Reverse-proxy upstream and requested scheme, host, port, and path | Point the client at the remote-latexmk gateway or server endpoint. |
| Protocol mismatch | Client and server release versions | Upgrade or select matching compatible release artifacts. |
| Engine unavailable | Installed TeX Live profile and enabled engine policy | Keep profile and engine policy distinct. Install the needed packages or enable only the intended engine. |
| Compile process exits or is killed | Bounded server logs, memory, disk, timeout, and concurrency limits | Correct the resource limit; do not remove security boundaries or enable shell escape. |

Client 401/403, saved token, private CA-file paths, and `auth login` belong to
`remote-latex-setup`. Do not ask the user to show the token while doing
server-side troubleshooting.
