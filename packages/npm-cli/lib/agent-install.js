import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { cp, lstat, mkdir, readFile, readdir, realpath, rename, stat, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';
import { applyEdits, modify, parse } from 'jsonc-parser';

const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const packageJSON = JSON.parse(await readFile(path.join(packageRoot, 'package.json'), 'utf8'));
const supportedAgents = new Set(['codex', 'claude-code', 'opencode']);

function usage() {
  return `Usage: rlatexmk agent install [options]

Install bundled Agent Skills and a local MCP configuration.

Options:
  --agent codex|claude-code|opencode  Repeat to select agents (default: detect)
  --project-root PATH                 Paper root bound to MCP (default: cwd)
  --server URL                        Remote server URL
  --token-file PATH                   Protected bearer-token file
  --ca-file PATH                      Optional private CA file
  --name NAME                         MCP entry name (default: remote-latexmk)
  --dry-run                           Print the complete plan without changes
  --force                             Back up changed Skills and replace MCP entries
  -h, --help                          Show this help

Raw tokens are not accepted. Use a protected token file.`;
}

function takeValue(args, index, option) {
  if (!args[index + 1] || args[index + 1].startsWith('--')) {
    throw new Error(`${option} needs a value`);
  }
  return args[index + 1];
}

export function parseAgentInstallArgs(args, env = process.env) {
  const options = {
    agents: [],
    projectRoot: process.cwd(),
    server: env.LATEXMK_SERVER ?? '',
    tokenFile: env.LATEXMK_TOKEN_FILE ?? '',
    caFile: env.LATEXMK_CA_FILE ?? '',
    name: 'remote-latexmk',
    dryRun: false,
    force: false,
    help: false,
  };
  for (let index = 0; index < args.length; index += 1) {
    const arg = args[index];
    switch (arg) {
      case '--agent': options.agents.push(takeValue(args, index, arg)); index += 1; break;
      case '--project-root': options.projectRoot = takeValue(args, index, arg); index += 1; break;
      case '--server': options.server = takeValue(args, index, arg); index += 1; break;
      case '--token-file': options.tokenFile = takeValue(args, index, arg); index += 1; break;
      case '--ca-file': options.caFile = takeValue(args, index, arg); index += 1; break;
      case '--name': options.name = takeValue(args, index, arg); index += 1; break;
      case '--dry-run': options.dryRun = true; break;
      case '--force': options.force = true; break;
      case '-h':
      case '--help': options.help = true; break;
      case '--token': throw new Error('raw tokens are not accepted; use --token-file');
      default: throw new Error(`unknown option: ${arg}`);
    }
  }
  for (const agent of options.agents) {
    if (!supportedAgents.has(agent)) throw new Error(`unsupported agent: ${agent}`);
  }
  if (!/^[A-Za-z0-9._-]+$/.test(options.name)) throw new Error('--name contains unsupported characters');
  return options;
}

function commandAvailable(command) {
  const result = spawnSync(command, ['--version'], { stdio: 'ignore', windowsHide: true });
  return !result.error;
}

function npmMCPCommand(projectRoot) {
  return [
    'npm', 'exec', '--yes', '--ignore-scripts',
    `--package=remote-latexmk@${packageJSON.version}`,
    '--', 'rlatexmk', 'mcp', 'serve', '--stdio', '--project-root', projectRoot,
  ];
}

function environment(options) {
  const env = {
    LATEXMK_SERVER: options.server,
    LATEXMK_TOKEN_FILE: options.tokenFile,
  };
  if (options.caFile) env.LATEXMK_CA_FILE = options.caFile;
  return env;
}

function displayCommand(command, args) {
  const quote = (value) => /^[A-Za-z0-9_./:@=+-]+$/.test(value) ? value : JSON.stringify(value);
  return [command, ...args].map(quote).join(' ');
}

function runCommand(command, args, dryRun, allowFailure = false) {
  console.log(`${dryRun ? 'would run' : 'running'}: ${displayCommand(command, args)}`);
  if (dryRun) return;
  const result = spawnSync(command, args, { stdio: 'inherit', windowsHide: true });
  if (result.error) throw new Error(`cannot run ${command}: ${result.error.message}`);
  if (result.status !== 0 && !allowFailure) throw new Error(`${command} exited with status ${result.status}`);
}

async function skillSourceRoot() {
  const bundled = path.join(packageRoot, 'bundled-skills');
  try {
    await stat(path.join(bundled, 'remote-latex', 'SKILL.md'));
    return bundled;
  } catch {
    const repository = path.resolve(packageRoot, '../..', '.agents/skills');
    await stat(path.join(repository, 'remote-latex', 'SKILL.md'));
    return repository;
  }
}

function skillDestination(agent, home) {
  if (agent === 'codex') return path.join(home, '.agents', 'skills');
  if (agent === 'claude-code') return path.join(home, '.claude', 'skills');
  return path.join(xdgConfigRoot(home), 'opencode', 'skills');
}

function xdgConfigRoot(home) {
  const configured = process.env.XDG_CONFIG_HOME;
  return configured && path.isAbsolute(configured) ? configured : path.join(home, '.config');
}

async function directoryDigest(root) {
  const hash = createHash('sha256');
  async function visit(directory, prefix = '') {
    const entries = await readdir(directory, { withFileTypes: true });
    entries.sort((a, b) => a.name.localeCompare(b.name));
    for (const entry of entries) {
      const relative = path.join(prefix, entry.name);
      const absolute = path.join(directory, entry.name);
      if (entry.isSymbolicLink()) throw new Error(`refusing symlink in bundled Skill: ${relative}`);
      if (entry.isDirectory()) {
        await visit(absolute, relative);
      } else if (entry.isFile()) {
        hash.update(relative);
        hash.update('\0');
        hash.update(await readFile(absolute));
        hash.update('\0');
      }
    }
  }
  await visit(root);
  return hash.digest('hex');
}

async function installSkill(source, destination, options) {
  let exists = true;
  try {
    await lstat(destination);
  } catch (error) {
    if (error.code !== 'ENOENT') throw error;
    exists = false;
  }
  if (exists && await directoryDigest(source) === await directoryDigest(destination)) {
    console.log(`unchanged Skill: ${destination}`);
    return;
  }
  if (exists && !options.force) {
    throw new Error(`Skill already exists with different content: ${destination}; inspect it or pass --force`);
  }
  if (options.dryRun) {
    console.log(`would ${exists ? 'back up and replace' : 'install'} Skill: ${destination}`);
    return;
  }
  await mkdir(path.dirname(destination), { recursive: true, mode: 0o700 });
  if (exists) {
    const backup = `${destination}.backup-${new Date().toISOString().replace(/[:.]/g, '-')}`;
    await rename(destination, backup);
    console.log(`backed up Skill: ${backup}`);
  }
  await cp(source, destination, { recursive: true, errorOnExist: true });
  console.log(`installed Skill: ${destination}`);
}

async function installSkills(agents, options) {
  const sourceRoot = await skillSourceRoot();
  for (const agent of agents) {
    const destinationRoot = skillDestination(agent, os.homedir());
    for (const name of ['remote-latex', 'remote-latex-maintenance', 'remote-latex-server', 'remote-latex-setup']) {
      await installSkill(path.join(sourceRoot, name), path.join(destinationRoot, name), options);
    }
  }
}

async function configureOpenCode(options, command, env) {
  const configRoot = xdgConfigRoot(os.homedir());
  const configPath = path.join(configRoot, 'opencode', 'opencode.json');
  let text = '{\n}\n';
  let exists = true;
  try {
    text = await readFile(configPath, 'utf8');
  } catch (error) {
    if (error.code !== 'ENOENT') throw error;
    exists = false;
  }
  const errors = [];
  const config = parse(text, errors, { allowTrailingComma: true, disallowComments: false });
  if (errors.length > 0) throw new Error(`cannot safely parse ${configPath}; no changes were made`);
  const entry = { type: 'local', command, enabled: true, environment: env };
  const current = config?.mcp?.[options.name];
  if (current && JSON.stringify(current) === JSON.stringify(entry)) {
    console.log(`unchanged MCP entry: ${configPath}#mcp.${options.name}`);
    return;
  }
  if (current && !options.force) {
    throw new Error(`OpenCode MCP entry ${options.name} already exists with different content; inspect it or pass --force`);
  }
  const edits = modify(text, ['mcp', options.name], entry, {
    formattingOptions: { insertSpaces: true, tabSize: 2, eol: '\n' },
  });
  const next = applyEdits(text, edits);
  console.log(`${options.dryRun ? 'would update' : 'updating'}: ${configPath}`);
  if (options.dryRun) return;
  await mkdir(path.dirname(configPath), { recursive: true, mode: 0o700 });
  if (exists) {
    const backup = `${configPath}.backup-${new Date().toISOString().replace(/[:.]/g, '-')}`;
    await writeFile(backup, text, { mode: 0o600 });
    console.log(`backed up OpenCode config: ${backup}`);
  }
  await writeFile(`${configPath}.new`, next, { mode: 0o600 });
  await rename(`${configPath}.new`, configPath);
}

async function configureMCP(agent, options) {
  const command = npmMCPCommand(options.projectRoot);
  const env = environment(options);
  if (agent === 'opencode') {
    await configureOpenCode(options, command, env);
    return;
  }
  if (options.force) {
    if (agent === 'codex') runCommand('codex', ['mcp', 'remove', options.name], options.dryRun, true);
    else runCommand('claude', ['mcp', 'remove', '--scope', 'user', options.name], options.dryRun, true);
  }
  const envArgs = Object.entries(env).flatMap(([key, value]) => ['--env', `${key}=${value}`]);
  if (agent === 'codex') {
    runCommand('codex', ['mcp', 'add', options.name, ...envArgs, '--', ...command], options.dryRun);
  } else {
    runCommand('claude', ['mcp', 'add', '--scope', 'user', ...envArgs, options.name, '--', ...command], options.dryRun);
  }
}

export async function installAgents(args) {
  const options = parseAgentInstallArgs(args);
  if (options.help) {
    console.log(usage());
    return;
  }
  options.projectRoot = await realpath(path.resolve(options.projectRoot));
  if (!options.server) throw new Error('--server or LATEXMK_SERVER is required');
  const server = new URL(options.server);
  if (server.protocol !== 'https:' && server.protocol !== 'http:') throw new Error('--server must use http or https');
  if (server.username || server.password) throw new Error('--server must not contain credentials');
  if (!options.tokenFile) throw new Error('--token-file or LATEXMK_TOKEN_FILE is required');
  options.tokenFile = await realpath(path.resolve(options.tokenFile));
  const tokenInfo = await stat(options.tokenFile);
  if (!tokenInfo.isFile()) throw new Error('--token-file must name a regular file');
  if (process.platform !== 'win32' && (tokenInfo.mode & 0o077) !== 0) {
    throw new Error('--token-file is readable by group or other users; use chmod 600');
  }
  const tokenRelative = path.relative(options.projectRoot, options.tokenFile);
  if (tokenRelative === '' || (!tokenRelative.startsWith(`..${path.sep}`) && tokenRelative !== '..' && !path.isAbsolute(tokenRelative))) {
    throw new Error('--token-file must be outside the paper project root');
  }
  if (options.caFile) {
    options.caFile = await realpath(path.resolve(options.caFile));
    if (!(await stat(options.caFile)).isFile()) throw new Error('--ca-file must name a regular file');
  }
  const agents = options.agents.length > 0
    ? [...new Set(options.agents)]
    : ['codex', 'claude-code', 'opencode'].filter((agent) => commandAvailable(agent === 'claude-code' ? 'claude' : agent));
  if (agents.length === 0) throw new Error('no supported Agent CLI was detected; pass --agent explicitly');

  console.log(`project root: ${options.projectRoot}`);
  console.log(`server:       ${options.server}`);
  console.log(`token file:   ${options.tokenFile}`);
  console.log(`agents:       ${agents.join(', ')}`);
  for (const agent of agents) await configureMCP(agent, options);
  await installSkills(agents, options);
  console.log(options.dryRun ? 'Dry run complete. No files were changed.' : 'Agent setup complete. Restart the Agent if it is already running.');
}
