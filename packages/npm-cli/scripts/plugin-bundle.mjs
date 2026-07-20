import { cp, readFile, readdir, rm, writeFile } from 'node:fs/promises';
import path from 'node:path';

const versionPattern = /^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$/;

async function rewriteManifest(target, version) {
  const manifest = JSON.parse(await readFile(target, 'utf8'));
  manifest.version = version;
  await writeFile(target, `${JSON.stringify(manifest, null, 2)}\n`);
}

async function rewriteVersionedCommands(directory, version) {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const target = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      await rewriteVersionedCommands(target, version);
    } else if (entry.isFile() && entry.name.endsWith('.md')) {
      const content = await readFile(target, 'utf8');
      await writeFile(
        target,
        content.replace(
          /remote-latexmk@[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?/g,
          `remote-latexmk@${version}`,
        ),
      );
    }
  }
}

export async function copyBundledPlugin(source, destination, version) {
  if (!versionPattern.test(version)) throw new Error(`invalid Plugin version: ${version}`);
  await rm(destination, { recursive: true, force: true });
  await cp(source, destination, { recursive: true, errorOnExist: true });

  for (const relative of ['.codex-plugin/plugin.json', '.claude-plugin/plugin.json']) {
    await rewriteManifest(path.join(destination, relative), version);
  }

  const mcpPath = path.join(destination, '.mcp.json');
  const mcp = JSON.parse(await readFile(mcpPath, 'utf8'));
  const args = mcp?.mcpServers?.['remote-latexmk']?.args;
  if (!Array.isArray(args)) throw new Error(`${mcpPath} is missing remote-latexmk MCP args`);
  let replacements = 0;
  mcp.mcpServers['remote-latexmk'].args = args.map((value) => {
    if (typeof value === 'string' && value.startsWith('remote-latexmk@')) {
      replacements += 1;
      return `remote-latexmk@${version}`;
    }
    return value;
  });
  if (replacements !== 1) throw new Error(`${mcpPath} must contain one versioned remote-latexmk package`);
  await writeFile(mcpPath, `${JSON.stringify(mcp, null, 2)}\n`);
  await rewriteVersionedCommands(path.join(destination, 'skills'), version);
}

export { versionPattern };
