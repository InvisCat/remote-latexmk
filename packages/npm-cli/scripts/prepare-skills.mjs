import { cp, mkdir, readFile, readdir, rm, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const repositoryRoot = path.resolve(packageRoot, '../..');
const source = path.join(repositoryRoot, '.agents/skills');
const destination = path.join(packageRoot, 'bundled-skills');

await rm(destination, { recursive: true, force: true });
await mkdir(destination, { recursive: true });
for (const name of ['remote-latex', 'remote-latex-maintenance']) {
  await cp(path.join(source, name), path.join(destination, name), { recursive: true });
}

async function rewriteCommands(directory) {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const target = path.join(directory, entry.name);
    if (entry.isDirectory()) await rewriteCommands(target);
    else if (entry.isFile() && entry.name.endsWith('.md')) {
      const content = await readFile(target, 'utf8');
      const rewritten = content
        .replace('client command named `latexmk`', 'npm launcher command named `remote-latexmk`')
        .replace(/\blatexmk(?= (?:doctor|meta|files|compile|jobs|diagnostics|logs|artifacts|cache|remote|help)\b)/g, 'remote-latexmk');
      await writeFile(target, rewritten);
    }
  }
}

await rewriteCommands(destination);
