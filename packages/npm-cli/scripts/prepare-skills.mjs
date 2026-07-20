import { cp, mkdir, readFile, readdir, rm, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { copyBundledPlugin } from './plugin-bundle.mjs';

const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const repositoryRoot = path.resolve(packageRoot, '../..');
const source = path.join(repositoryRoot, '.agents/skills');
const destination = path.join(packageRoot, 'bundled-skills');
const packageJSON = JSON.parse(await readFile(path.join(packageRoot, 'package.json'), 'utf8'));

await rm(destination, { recursive: true, force: true });
await mkdir(destination, { recursive: true });
for (const name of ['remote-latex', 'remote-latex-maintenance', 'remote-latex-server', 'remote-latex-setup']) {
  await cp(path.join(source, name), path.join(destination, name), { recursive: true });
}

async function rewriteCommands(directory) {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const target = path.join(directory, entry.name);
    if (entry.isDirectory()) await rewriteCommands(target);
    else if (entry.isFile() && entry.name.endsWith('.md')) {
      const content = await readFile(target, 'utf8');
      const rewritten = content
        .replace(
          'Select the repository binary at `packages/cli/dist/latexmk` while developing this repository. Otherwise use the installed client binary. Do not run an extra `help` probe during a normal compile workflow.',
          'Use the npm launcher command named `remote-latexmk` for CLI fallbacks. Do not run an extra `help` probe during a normal compile workflow.',
        )
        .replace(
          /Use the remote-latexmk client command named `latexmk`\. Do not invoke the\s+unrelated TeX Live command with the same name\./g,
          'Use the npm launcher command named `remote-latexmk`. Do not invoke the unrelated TeX Live `latexmk` command.',
        )
        .replace(/\blatexmk(?= (?:auth|setup|doctor|meta|entries|files|compile|jobs|diagnostics|logs|artifacts|cache|remote|help)\b)/g, 'remote-latexmk');
      await writeFile(target, rewritten);
    }
  }
}

await rewriteCommands(destination);
await copyBundledPlugin(
  path.join(repositoryRoot, 'plugins', 'remote-latexmk'),
  path.join(packageRoot, 'bundled-plugin'),
  packageJSON.version,
);
