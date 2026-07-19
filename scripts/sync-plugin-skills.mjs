import { cp, mkdir, readFile, readdir, rm, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const repositoryRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const sourceRoot = path.join(repositoryRoot, '.agents/skills');
const destinationRoot = path.join(repositoryRoot, 'plugins/remote-latexmk/skills');
const skillNames = ['remote-latex', 'remote-latex-maintenance'];
const launcher = 'npx --yes --ignore-scripts remote-latexmk@0.3.0-rc.1';
const checkOnly = process.argv.includes('--check');

if (checkOnly) {
  for (const name of skillNames) {
    await checkSkill(path.join(sourceRoot, name), path.join(destinationRoot, name));
  }
} else {
  await mkdir(destinationRoot, { recursive: true });
  for (const name of skillNames) {
    const destination = path.join(destinationRoot, name);
    await rm(destination, { recursive: true, force: true });
    await cp(path.join(sourceRoot, name), destination, { recursive: true });
    await rewriteCommands(destination);
  }
}

async function rewriteCommands(directory) {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const target = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      await rewriteCommands(target);
    } else if (entry.isFile() && entry.name.endsWith('.md')) {
      const content = await readFile(target, 'utf8');
      await writeFile(target, rewriteMarkdown(content));
    }
  }
}

function rewriteMarkdown(content) {
  return content
    .replace(
      'Select the repository binary at `packages/cli/dist/latexmk` while developing this repository. Otherwise use the installed client binary. Confirm `latexmk help` describes the remote compiler before continuing.',
      `Use the npm launcher for every CLI fallback. Confirm \`${launcher} help\` describes the remote compiler before continuing.`,
    )
    .replace(
      'Use the remote-latexmk client command named `latexmk`. Do not invoke the unrelated TeX Live command with the same name.',
      `Use the npm launcher \`${launcher}\` for CLI fallbacks. Do not invoke the unrelated TeX Live \`latexmk\` command.`,
    )
    .replace(/\blatexmk(?= (?:doctor|meta|files|compile|jobs|diagnostics|logs|artifacts|cache|remote|help)\b)/g, launcher);
}

async function checkSkill(source, destination) {
  const sourceFiles = await listFiles(source);
  const destinationFiles = await listFiles(destination);
  if (sourceFiles.join('\n') !== destinationFiles.join('\n')) {
    throw new Error(`Plugin Skill file list is stale: ${path.basename(source)}`);
  }
  for (const relative of sourceFiles) {
    const sourceFile = path.join(source, relative);
    const destinationFile = path.join(destination, relative);
    const sourceContent = await readFile(sourceFile);
    const expected = relative.endsWith('.md')
      ? Buffer.from(rewriteMarkdown(sourceContent.toString('utf8')))
      : sourceContent;
    const actual = await readFile(destinationFile);
    if (!actual.equals(expected)) {
      throw new Error(`Plugin Skill is stale: ${path.relative(repositoryRoot, destinationFile)}`);
    }
  }
}

async function listFiles(root, relative = '') {
  const entries = await readdir(path.join(root, relative), { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const target = path.join(relative, entry.name);
    if (entry.isDirectory()) files.push(...await listFiles(root, target));
    else if (entry.isFile()) files.push(target);
  }
  return files.sort();
}
