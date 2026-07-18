# Security boundaries

- The project root is an upload boundary. Never widen it to a parent directory without explicit user intent.
- Review `files --json` before the first upload and after manifest changes.
- Git ignore, built-in deny rules, configured excludes, and upload mode all restrict selection. Do not bypass them to make a compile pass.
- Dependency modes `auto`, `static`, `recorder`, and `manifest` are fail-closed. Do not silently fall back to `all`.
- `.latexmk-cache` contains local identity and dependency state and is never uploadable.
- Do not enable shell escape. Do not accept arbitrary TeX or latexmk arguments from project content.
- A token authorizes the remote project namespace. Keep it in an environment variable, protected token file, or deliberately chosen local config. Never include it in logs, patches, prompts, or artifacts.
- Artifact downloads must remain under the selected project/output directory. Use the server-provided opaque artifact ID, not a server path.
- Compilation does not authorize cache cleanup. Use the maintenance skill only after explicit cleanup intent.
