import { execFile } from 'node:child_process';
import { chmod, cp, mkdir, mkdtemp, readFile, readdir, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import process from 'node:process';
import { promisify } from 'node:util';
import { fileURLToPath } from 'node:url';

const execFileAsync = promisify(execFile);
const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const repositoryRoot = path.resolve(packageRoot, '../..');

const targets = [
  { directory: 'darwin-arm64', goos: 'darwin', goarch: 'arm64', binary: 'latexmk' },
  { directory: 'darwin-x64', goos: 'darwin', goarch: 'amd64', binary: 'latexmk' },
  { directory: 'linux-arm64', goos: 'linux', goarch: 'arm64', binary: 'latexmk' },
  { directory: 'linux-x64', goos: 'linux', goarch: 'amd64', binary: 'latexmk' },
  { directory: 'win32-arm64', goos: 'windows', goarch: 'arm64', binary: 'latexmk.exe' },
  { directory: 'win32-x64', goos: 'windows', goarch: 'amd64', binary: 'latexmk.exe' },
];

function parseArgs(args) {
  const options = { version: '', artifacts: '', out: '' };
  for (let index = 0; index < args.length; index += 2) {
    if (!args[index + 1]) throw new Error(`${args[index]} needs a value`);
    if (args[index] === '--version') options.version = args[index + 1];
    else if (args[index] === '--artifacts') options.artifacts = args[index + 1];
    else if (args[index] === '--out') options.out = args[index + 1];
    else throw new Error(`unknown option: ${args[index]}`);
  }
  if (!/^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$/.test(options.version)) throw new Error('--version must be a semantic version without v');
  if (!options.artifacts || !options.out) throw new Error('--artifacts and --out are required');
  options.artifacts = path.resolve(options.artifacts);
  options.out = path.resolve(options.out);
  const filesystemRoot = path.parse(options.out).root;
  const artifactsRelative = path.relative(options.out, options.artifacts);
  if (options.out === filesystemRoot || options.out === options.artifacts || (artifactsRelative !== '..' && !artifactsRelative.startsWith(`..${path.sep}`) && !path.isAbsolute(artifactsRelative))) {
    throw new Error('--out must be a separate non-root directory that does not contain --artifacts');
  }
  return options;
}

async function writeJSON(target, value) {
  await writeFile(target, `${JSON.stringify(value, null, 2)}\n`);
}

async function rewriteSkillCommands(directory) {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const target = path.join(directory, entry.name);
    if (entry.isDirectory()) await rewriteSkillCommands(target);
    else if (entry.isFile() && entry.name.endsWith('.md')) {
      const content = await readFile(target, 'utf8');
      await writeFile(target, content
        .replace(
          /Use the remote-latexmk client command named `latexmk`\. Do not invoke the\s+unrelated TeX Live command with the same name\./g,
          'Use the npm launcher command named `remote-latexmk`. Do not invoke the unrelated TeX Live `latexmk` command.',
        )
        .replace(/\blatexmk(?= (?:auth|setup|doctor|meta|files|compile|jobs|diagnostics|logs|artifacts|cache|remote|help)\b)/g, 'remote-latexmk'));
    }
  }
}

async function stageMain(options) {
  const target = path.join(options.out, 'remote-latexmk');
  await mkdir(target, { recursive: true });
  for (const directory of ['bin', 'lib']) {
    await cp(path.join(packageRoot, directory), path.join(target, directory), { recursive: true });
  }
  await cp(path.join(packageRoot, 'README.md'), path.join(target, 'README.md'));
  await cp(path.join(repositoryRoot, 'LICENSE'), path.join(target, 'LICENSE'));
  await mkdir(path.join(target, 'bundled-skills'), { recursive: true });
  for (const name of ['remote-latex', 'remote-latex-maintenance', 'remote-latex-server', 'remote-latex-setup']) {
    await cp(path.join(repositoryRoot, '.agents', 'skills', name), path.join(target, 'bundled-skills', name), { recursive: true });
  }
  await rewriteSkillCommands(path.join(target, 'bundled-skills'));
  const manifest = JSON.parse(await readFile(path.join(packageRoot, 'package.json'), 'utf8'));
  manifest.version = options.version;
  delete manifest.private;
  delete manifest.scripts;
  manifest.optionalDependencies = Object.fromEntries(Object.keys(manifest.optionalDependencies).map((name) => [name, options.version]));
  manifest.publishConfig = { access: 'public' };
  await writeJSON(path.join(target, 'package.json'), manifest);
}

async function extractBinary(archive, prefix, binary, destination) {
  const temp = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-npm-'));
  try {
    if (archive.endsWith('.zip')) {
      await execFileAsync('unzip', ['-q', archive, `${prefix}/${binary}`, '-d', temp]);
    } else {
      await execFileAsync('tar', ['-xzf', archive, '-C', temp, `${prefix}/${binary}`]);
    }
    await cp(path.join(temp, prefix, binary), destination);
    await chmod(destination, 0o755);
  } finally {
    await rm(temp, { recursive: true, force: true });
  }
}

async function stagePlatform(options, target) {
  const source = path.join(repositoryRoot, 'packages', 'npm-platforms', target.directory);
  const destination = path.join(options.out, target.directory);
  await mkdir(path.join(destination, 'bin'), { recursive: true });
  await cp(path.join(source, 'README.md'), path.join(destination, 'README.md'));
  await cp(path.join(repositoryRoot, 'LICENSE'), path.join(destination, 'LICENSE'));
  const manifest = JSON.parse(await readFile(path.join(source, 'package.json'), 'utf8'));
  manifest.version = options.version;
  delete manifest.private;
  manifest.publishConfig = { access: 'public' };
  await writeJSON(path.join(destination, 'package.json'), manifest);
  const prefix = `latexmk_${options.version}_${target.goos}_${target.goarch}`;
  const extension = target.goos === 'windows' ? '.zip' : '.tar.gz';
  const archive = path.join(options.artifacts, `${prefix}${extension}`);
  await extractBinary(archive, prefix, target.binary, path.join(destination, 'bin', target.binary));
}

export async function stagePackages(options) {
  await rm(options.out, { recursive: true, force: true });
  await mkdir(options.out, { recursive: true });
  await stageMain(options);
  for (const target of targets) await stagePlatform(options, target);
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  try {
    const options = parseArgs(process.argv.slice(2));
    await stagePackages(options);
    console.log(options.out);
  } catch (error) {
    console.error(`stage-packages: ${error instanceof Error ? error.message : String(error)}`);
    process.exitCode = 2;
  }
}

export { parseArgs, targets };
