#!/usr/bin/env node

import { spawn } from 'node:child_process';
import process from 'node:process';
import { installAgents } from '../lib/agent-install.js';
import { installCodexPlugin } from '../lib/plugin-install.js';
import { resolveNativeBinary } from '../lib/platform.js';

async function runNative(args) {
  const binary = resolveNativeBinary();
  const child = spawn(binary, args, { stdio: 'inherit', windowsHide: true });
  const forward = (signal) => {
    if (!child.killed) child.kill(signal);
  };
  process.once('SIGINT', () => forward('SIGINT'));
  process.once('SIGTERM', () => forward('SIGTERM'));
  return await new Promise((resolve, reject) => {
    child.once('error', reject);
    child.once('exit', (code, signal) => resolve(signal ? 1 : (code ?? 1)));
  });
}

async function main(args) {
  if (args[0] === 'agent' && args[1] === 'install') {
    await installAgents(args.slice(2));
    return 0;
  }
  if (args[0] === 'plugin' && args[1] === 'install') {
    if (args[2] !== 'codex') throw new Error('Usage: remote-latexmk plugin install codex [options]');
    await installCodexPlugin(args.slice(3));
    return 0;
  }
  return runNative(args);
}

try {
  process.exitCode = await main(process.argv.slice(2));
} catch (error) {
  console.error(`remote-latexmk: ${error instanceof Error ? error.message : String(error)}`);
  process.exitCode = 2;
}
