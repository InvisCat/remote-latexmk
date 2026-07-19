import assert from 'node:assert/strict';
import { chmod, mkdir, mkdtemp, readFile, readdir, realpath, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { parse } from 'jsonc-parser';
import { installAgents, parseAgentInstallArgs } from '../lib/agent-install.js';

test('Agent installer accepts token files but rejects raw tokens', () => {
  const options = parseAgentInstallArgs([
    '--agent', 'codex', '--agent', 'opencode',
    '--project-root', '/paper', '--server', 'https://latex.example.edu',
    '--token-file', '/secrets/latexmk-token', '--dry-run',
  ], {});
  assert.deepEqual(options.agents, ['codex', 'opencode']);
  assert.equal(options.tokenFile, '/secrets/latexmk-token');
  assert.equal(options.dryRun, true);
  assert.throws(() => parseAgentInstallArgs(['--token', 'secret'], {}), /raw tokens are not accepted/);
});

test('Agent installer rejects unknown agents and ambiguous names', () => {
  assert.throws(() => parseAgentInstallArgs(['--agent', 'gemini'], {}), /unsupported agent/);
  assert.throws(() => parseAgentInstallArgs(['--name', 'bad name'], {}), /unsupported characters/);
});

test('OpenCode setup preserves JSONC comments and installs both Skills', async () => {
  const temp = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-agent-'));
  const project = path.join(temp, 'paper');
  const token = path.join(temp, 'token');
  const configRoot = path.join(temp, 'config');
  const configPath = path.join(configRoot, 'opencode', 'opencode.json');
  await mkdir(project, { recursive: true });
  await mkdir(path.dirname(configPath), { recursive: true });
  await writeFile(token, 'test-token-with-more-than-24-characters\n');
  await chmod(token, 0o600);
  await writeFile(configPath, '{\n  // keep this comment\n  "theme": "system",\n}\n');

  const previous = process.env.XDG_CONFIG_HOME;
  process.env.XDG_CONFIG_HOME = configRoot;
  try {
    await installAgents([
      '--agent', 'opencode', '--project-root', project,
      '--server', 'http://127.0.0.1:8080', '--token-file', token,
    ]);
  } finally {
    if (previous === undefined) delete process.env.XDG_CONFIG_HOME;
    else process.env.XDG_CONFIG_HOME = previous;
  }

  const text = await readFile(configPath, 'utf8');
  assert.match(text, /keep this comment/);
  const config = parse(text);
  assert.equal(config.mcp['remote-latexmk'].type, 'local');
  assert.equal(config.mcp['remote-latexmk'].command[0], 'npm');
  assert.equal(config.mcp['remote-latexmk'].environment.LATEXMK_TOKEN_FILE, await realpath(token));
  for (const skill of ['remote-latex', 'remote-latex-maintenance']) {
    assert.match(await readFile(path.join(configRoot, 'opencode', 'skills', skill, 'SKILL.md'), 'utf8'), /^---/);
  }
  assert.equal((await readdir(path.dirname(configPath))).some((name) => name.startsWith('opencode.json.backup-')), true);

  const projectToken = path.join(project, 'token');
  await writeFile(projectToken, 'test-token-with-more-than-24-characters\n');
  await chmod(projectToken, 0o600);
  await assert.rejects(installAgents([
    '--agent', 'opencode', '--project-root', project,
    '--server', 'http://127.0.0.1:8080', '--token-file', projectToken, '--dry-run',
  ]), /must be outside the paper project root/);
});
