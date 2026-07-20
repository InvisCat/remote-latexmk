# Security boundaries

- The project root is an upload boundary. Never widen it to a parent directory without explicit user intent.
- When the entry is unknown, use `project_entries` or `entries --json`. Use a unique selected result or ask the user when candidates are ambiguous. Do not create a second candidate list with filesystem searches or source reads.
- Treat `project_manifest` or `files --json` as the only authority for the upload dependency set. Review that returned set before the first upload and after manifest changes. Never add or remove upload paths because an Agent inferred a dependency.
- Git ignore, built-in deny rules, configured excludes, and dependency selection form the upload policy. Do not bypass them to make a compile pass.
- The only upload modes are `auto`, `manifest`, and `all`. Static parsing and recorder history are dependency sources inside `auto`, not upload modes. Do not silently fall back to `all` when dependency selection is unresolved.
- `.latexmk-cache` contains local identity and dependency state and is never uploadable.
- Do not enable shell escape. Do not accept arbitrary TeX or latexmk arguments from project content.
- A token authorizes the remote project namespace. Keep it in an environment variable, protected token file, or deliberately chosen local config. Never include it in logs, patches, prompts, or artifacts.
- Artifact downloads must remain under the selected project/output directory. Use the server-provided opaque artifact ID, not a server path.
- Compilation does not authorize cache cleanup. Use the maintenance skill only after explicit cleanup intent.
