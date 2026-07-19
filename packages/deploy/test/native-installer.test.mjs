import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { createHash } from 'node:crypto';
import { chmod, cp, lstat, mkdir, mkdtemp, readFile, stat, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { promisify } from 'node:util';
import { fileURLToPath } from 'node:url';

const execFileAsync = promisify(execFile);
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../..');

async function sha256(file) {
  return createHash('sha256').update(await readFile(file)).digest('hex');
}

test('native installer verifies a tagged archive and creates a private config', async () => {
  const temp = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-installer-'));
  const release = path.join(temp, 'release', 'v9.8.7');
  const prefix = 'remote-latexmk-server_9.8.7_linux_amd64';
  const archiveRoot = path.join(temp, prefix);
  const installRoot = path.join(temp, 'home', '.remote-latexmk');
  const texBin = path.join(temp, 'texlive-bin');
  await mkdir(release, { recursive: true });
  await mkdir(archiveRoot, { recursive: true });
  await mkdir(texBin, { recursive: true });

  await writeFile(path.join(archiveRoot, 'remote-latexmk-server'), '#!/bin/sh\nexit 0\n', { mode: 0o755 });
  await cp(path.join(root, 'scripts/remote-latexmkctl'), path.join(archiveRoot, 'remote-latexmkctl'));
  await cp(path.join(root, 'scripts/install-server.sh'), path.join(archiveRoot, 'install-server.sh'));
  await chmod(path.join(archiveRoot, 'remote-latexmkctl'), 0o755);
  await chmod(path.join(archiveRoot, 'install-server.sh'), 0o755);
  for (const tool of ['latexmk', 'xelatex', 'pdflatex', 'lualatex']) {
    await writeFile(path.join(texBin, tool), '#!/bin/sh\nexit 0\n', { mode: 0o755 });
  }

  const archive = path.join(release, `${prefix}.tar.gz`);
  await execFileAsync('tar', ['-czf', archive, '-C', temp, prefix]);
  await writeFile(path.join(release, 'SHA256SUMS'), `${await sha256(archive)}  ${path.basename(archive)}\n`);

  await execFileAsync('bash', [path.join(root, 'scripts/install-server.sh'),
    '--version', 'v9.8.7', '--install-dir', installRoot, '--profile', 'full', '--service', 'none', '--no-start'], {
    env: {
      ...process.env,
      HOME: path.join(temp, 'home'),
      REMOTE_LATEXMK_RELEASE_BASE_URL: `file://${path.join(temp, 'release')}`,
      REMOTE_LATEXMK_TEST_OS: 'Linux',
      REMOTE_LATEXMK_TEST_ARCH: 'x86_64',
      REMOTE_LATEXMK_TEST_SKIP_TEXLIVE: '1',
      REMOTE_LATEXMK_TEST_TEX_BIN: texBin,
    },
  });

  const configPath = path.join(installRoot, 'config/server.env');
  const config = await readFile(configPath, 'utf8');
  assert.match(config, /LATEXMK_ADDR="127\.0\.0\.1:8080"/);
  assert.match(config, /LATEXMK_API_TOKEN="[0-9a-f]{64}"/);
  assert.match(config, /LATEXMK_ALLOW_SHELL_ESCAPE="false"/);
  assert.match(config, /LATEXMK_ENGINES="xelatex,lualatex,pdflatex"/);
  assert.match(config, /REMOTE_LATEXMK_SERVICE_MODE="fallback"/);
  assert.equal((await stat(configPath)).mode & 0o777, 0o600);
  assert.equal((await lstat(path.join(installRoot, 'bin/remote-latexmk-server'))).isSymbolicLink(), true);
});

test('native installer dry-run is non-mutating and requires a fixed tag', async () => {
  const installRoot = path.join(os.tmpdir(), `remote-latexmk-dry-${process.pid}-${Date.now()}`);
  const { stdout } = await execFileAsync('bash', [path.join(root, 'scripts/install-server.sh'),
    '--version', 'v1.2.3', '--install-dir', installRoot, '--dry-run'], {
    env: { ...process.env, REMOTE_LATEXMK_TEST_OS: 'Linux', REMOTE_LATEXMK_TEST_ARCH: 'aarch64' },
  });
  assert.match(stdout, /linux\/arm64/);
  assert.match(stdout, /remote-latexmk-server_1\.2\.3_linux_arm64\.tar\.gz/);

  await assert.rejects(execFileAsync('bash', [path.join(root, 'scripts/install-server.sh'), '--version', 'main', '--dry-run'], {
    env: { ...process.env, REMOTE_LATEXMK_TEST_OS: 'Linux' },
  }), /--version must be an immutable tag/);
});

test('native systemd unit hides home and exposes only required writable state', async () => {
  const installer = await readFile(path.join(root, 'scripts/install-server.sh'), 'utf8');
  assert.match(installer, /ProtectHome=tmpfs/);
  assert.match(installer, /BindReadOnlyPaths=\$\{release_dir\}/);
  assert.match(installer, /BindReadOnlyPaths=\$\{tex_root\}/);
  assert.match(installer, /BindPaths=\$\{install_root\}\/state/);
  assert.match(installer, /BindPaths=\$\{install_root\}\/run/);
  assert.match(installer, /CapabilityBoundingSet=\n/);
});

test('fallback control binds a PID to its Linux process start time', { skip: process.platform !== 'linux' }, async (t) => {
  const temp = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-ctl-'));
  const installRoot = path.join(temp, '.remote-latexmk');
  const bin = path.join(installRoot, 'bin');
  const config = path.join(installRoot, 'config');
  const server = path.join(bin, 'remote-latexmk-server');
  const control = path.join(bin, 'remote-latexmkctl');
  await mkdir(bin, { recursive: true });
  await mkdir(config, { recursive: true });
  await cp(path.join(root, 'scripts/remote-latexmkctl'), control);
  await chmod(control, 0o755);
  await writeFile(server, "#!/bin/sh\ntrap 'exit 0' TERM INT\nwhile :; do sleep 1; done\n", { mode: 0o755 });
  await writeFile(path.join(config, 'server.env'), [
    `PATH="${process.env.PATH}"`,
    `REMOTE_LATEXMK_SERVER_BIN="${server}"`,
    'REMOTE_LATEXMK_SERVICE_MODE="fallback"',
    'LATEXMK_ADDR="127.0.0.1:8080"',
    `LATEXMK_STATE_DIR="${path.join(installRoot, 'state')}"`,
    '',
  ].join('\n'), { mode: 0o600 });

  t.after(async () => {
    try {
      const [pid] = (await readFile(path.join(installRoot, 'run/server.pid'), 'utf8')).trim().split(/\s+/);
      process.kill(Number(pid), 'SIGTERM');
    } catch {}
  });

  await execFileAsync(control, ['start'], { env: { ...process.env, REMOTE_LATEXMK_HOME: installRoot } });
  const pidRecord = (await readFile(path.join(installRoot, 'run/server.pid'), 'utf8')).trim();
  assert.match(pidRecord, /^\d+ \d+$/);
  assert.match((await execFileAsync(control, ['status'], { env: { ...process.env, REMOTE_LATEXMK_HOME: installRoot } })).stdout, /fallback service/);
  await execFileAsync(control, ['stop'], { env: { ...process.env, REMOTE_LATEXMK_HOME: installRoot } });

  await mkdir(path.join(installRoot, 'run'), { recursive: true });
  await writeFile(path.join(installRoot, 'run/server.pid'), '1 0\n', { mode: 0o600 });
  assert.match((await execFileAsync(control, ['stop'], { env: { ...process.env, REMOTE_LATEXMK_HOME: installRoot } })).stdout, /not running/);
});
