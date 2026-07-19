# Login errors

Use only the branch that matches the reported error. Keep the API token in the
user's trusted terminal.

| Error | Meaning | Correction |
| --- | --- | --- |
| `server health check failed`, timeout, connection refused, or DNS failure | The client cannot reach the configured HTTP service. | Confirm the URL and port. Check the server service, its listen address, and the LAN, VPN, firewall, or SSH tunnel. Do not expose a new interface without the user's decision. |
| TLS or `x509` error | The client cannot validate the server certificate. | Confirm the HTTPS hostname and configure the intended CA with the preview/apply `setup --ca-file` flow. Never suggest `--insecure-skip-verify` as a fix. |
| `server identifies as ... not remote-latexmk` | The URL points to another service or the wrong reverse-proxy route. | Correct the scheme, host, port, or proxy path. Do not save credentials for that endpoint. |
| `server protocol ... does not match client protocol ...` | Client and server releases are incompatible. | Use client and server artifacts from the same compatible release. Do not retry compilation until versions match. |
| `remote-latexmk API token verification failed` with HTTP 401 or 403 | The server rejected the token. | Obtain the current remote-latexmk token through a trusted channel and rerun the hidden `auth login` prompt. Never request the token in chat. |
| Token is empty, multiline, or too large | The pasted value is not one valid token. | Copy only the single token value and retry in the hidden prompt. Do not normalize or store it through Agent tools. |

After correcting the cause, rerun `auth login`. It performs the connection and
authentication check before replacing saved credentials. Run `doctor`
afterward for the broader client and project-policy check.
