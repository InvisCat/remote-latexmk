import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { createHash } from 'node:crypto';
import { chmod, cp, lstat, mkdir, mkdtemp, readFile, readlink, stat, unlink, writeFile } from 'node:fs/promises';
import { createServer as createHttpServer } from 'node:http';
import { createServer as createNetServer } from 'node:net';
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

async function createNativeRelease(temp, version, {
  installerTransform = (value) => value,
  serverContent = '#!/bin/sh\nexit 0\n',
} = {}) {
  const release = path.join(temp, 'release', `v${version}`);
  const prefix = `remote-latexmk-server_${version}_linux_amd64`;
  const archiveRoot = path.join(temp, prefix);
  await mkdir(release, { recursive: true });
  await mkdir(archiveRoot, { recursive: true });
  await writeFile(path.join(archiveRoot, 'remote-latexmk-server'), serverContent, { mode: 0o755 });
  await cp(path.join(root, 'scripts/remote-latexmkctl'), path.join(archiveRoot, 'remote-latexmkctl'));
  const installer = installerTransform(await readFile(path.join(root, 'scripts/install-server.sh'), 'utf8'));
  await writeFile(path.join(archiveRoot, 'install-server.sh'), installer, { mode: 0o755 });
  await chmod(path.join(archiveRoot, 'remote-latexmkctl'), 0o755);
  const archive = path.join(release, `${prefix}.tar.gz`);
  await execFileAsync('tar', ['-czf', archive, '-C', path.dirname(archiveRoot), path.basename(archiveRoot)]);
  await writeFile(path.join(release, 'install-server.sh'), installer, { mode: 0o755 });
  await writeFile(path.join(release, 'SHA256SUMS'), [
    `${await sha256(archive)}  ${path.basename(archive)}`,
    `${await sha256(path.join(release, 'install-server.sh'))}  install-server.sh`,
    '',
  ].join('\n'));
  return { release, archive, archiveRoot };
}

async function findFreePort() {
  const server = createNetServer();
  await new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(0, '127.0.0.1', resolve);
  });
  const { port } = server.address();
  await new Promise((resolve, reject) => server.close((error) => error ? reject(error) : resolve()));
  return port;
}

async function startMetadataServer(port, service = 'remote-latexmk') {
  const server = createHttpServer((request, response) => {
    response.setHeader('content-type', 'application/json');
    if (request.url === '/healthz') return response.end('{"status":"ok"}');
    if (request.url === '/v1/meta') return response.end(JSON.stringify({ service }));
    response.statusCode = 404;
    response.end('{}');
  });
  await new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(port, '127.0.0.1', resolve);
  });
  return async () => new Promise((resolve, reject) => server.close((error) => error ? reject(error) : resolve()));
}

async function startRedirectServer(port) {
  const server = createHttpServer((request, response) => {
    response.statusCode = 302;
    response.setHeader('location', '/redirected');
    response.end('{"service":"remote-latexmk","status":"ok"}');
  });
  await new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(port, '127.0.0.1', resolve);
  });
  return async () => new Promise((resolve, reject) => server.close((error) => error ? reject(error) : resolve()));
}

async function createSystemctlStub(binDir) {
  const stub = path.join(binDir, 'systemctl');
  await writeFile(stub, `#!/bin/sh
printf '%s\\n' "$*" >>"$REMOTE_LATEXMK_TEST_SYSTEMCTL_LOG"
case " $* " in
  *" is-enabled "*) [ "${'${REMOTE_LATEXMK_TEST_SYSTEMD_ENABLED:-0}'}" = 1 ] ;;
  *" enable --now "*)
    if [ -n "${'${REMOTE_LATEXMK_TEST_SYSTEMCTL_COUNT_FILE:-}'}" ]; then
      count=0
      [ ! -f "$REMOTE_LATEXMK_TEST_SYSTEMCTL_COUNT_FILE" ] || count=$(cat "$REMOTE_LATEXMK_TEST_SYSTEMCTL_COUNT_FILE")
      count=$((count + 1))
      printf '%s\\n' "$count" >"$REMOTE_LATEXMK_TEST_SYSTEMCTL_COUNT_FILE"
      if [ "${'${REMOTE_LATEXMK_TEST_FAIL_ENABLE_AT:-0}'}" -gt 0 ] && [ "$count" -ge "$REMOTE_LATEXMK_TEST_FAIL_ENABLE_AT" ]; then exit 1; fi
    fi
    exit 0 ;;
  *) exit 0 ;;
esac
`, { mode: 0o755 });
  return stub;
}

async function createCurlLoggingWrapper(binDir, realCurl) {
  const wrapper = path.join(binDir, 'curl');
  await writeFile(wrapper, `#!/bin/sh
printf '%s\\n' "$*" >>"$REMOTE_LATEXMK_TEST_CURL_LOG"
exec "${realCurl}" "$@"
`, { mode: 0o755 });
  return wrapper;
}

function fakeHttpServerSource({ badIdentityPort = -1 } = {}) {
  return `#!${process.execPath}
const http = require('node:http');
const address = process.env.LATEXMK_ADDR || '127.0.0.1:8080';
let host;
let port;
const bracketed = /^\\[([^\\]]+)\\]:(\\d+)$/.exec(address);
if (bracketed) {
  host = bracketed[1];
  port = Number(bracketed[2]);
} else {
  const split = address.lastIndexOf(':');
  host = address.slice(0, split);
  port = Number(address.slice(split + 1));
}
const server = http.createServer((request, response) => {
  response.setHeader('content-type', 'application/json');
  if (request.url === '/healthz') return response.end('{"status":"ok"}');
  if (request.url === '/v1/meta') {
    return response.end(JSON.stringify({ service: port === ${badIdentityPort} ? 'not-remote-latexmk' : 'remote-latexmk' }));
  }
  response.statusCode = 404;
  response.end('{}');
});
server.listen(port, host);
for (const signal of ['SIGTERM', 'SIGINT']) {
  process.on(signal, () => server.close(() => process.exit(0)));
}
`;
}

function failAfterActivation(value) {
  const marker = '\nactivation_started=false\n\nif [[ "${server_started}"';
  assert.ok(value.includes(marker));
  return value.replace(marker, '\nfalse\n\nactivation_started=false\n\nif [[ "${server_started}"');
}

async function createFakeTexBin(temp) {
  const texBin = path.join(temp, 'texlive-bin');
  await mkdir(texBin, { recursive: true });
  for (const tool of ['latexmk', 'xelatex', 'pdflatex', 'lualatex']) {
    await writeFile(path.join(texBin, tool), '#!/bin/sh\nexit 0\n', { mode: 0o755 });
  }
  return texBin;
}

function nativeEnv(temp, texBin) {
  return {
    ...process.env,
    HOME: path.join(temp, 'home'),
    REMOTE_LATEXMK_RELEASE_BASE_URL: `file://${path.join(temp, 'release')}`,
    REMOTE_LATEXMK_TEST_OS: 'Linux',
    REMOTE_LATEXMK_TEST_ARCH: 'x86_64',
    REMOTE_LATEXMK_TEST_SKIP_TEXLIVE: '1',
    REMOTE_LATEXMK_TEST_TEX_BIN: texBin,
  };
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

  const { stdout } = await execFileAsync('bash', [path.join(root, 'scripts/install-server.sh'),
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
  const tokenPath = path.join(installRoot, 'config/token');
  const config = await readFile(configPath, 'utf8');
  const token = (await readFile(tokenPath, 'utf8')).trim();
  assert.match(config, /LATEXMK_ADDR="127\.0\.0\.1:8080"/);
  assert.match(config, /LATEXMK_API_TOKEN=""/);
  assert.ok(config.includes(`LATEXMK_API_TOKEN_FILE="${tokenPath}"`));
  assert.doesNotMatch(config, new RegExp(token));
  assert.match(token, /^[0-9a-f]{64}$/);
  assert.match(config, /LATEXMK_ALLOW_SHELL_ESCAPE="false"/);
  assert.match(config, /LATEXMK_ENGINES="xelatex,pdflatex"/);
  assert.ok(config.includes(`LATEXMK_TOOLCHAIN_PATH="${texBin}:/usr/local/bin:/usr/bin:/bin"`));
  assert.match(config, /REMOTE_LATEXMK_SERVICE_MODE="fallback"/);
  assert.equal((await stat(configPath)).mode & 0o777, 0o600);
  assert.equal((await stat(tokenPath)).mode & 0o777, 0o600);
  assert.match(stdout, /remote-latexmk v9\.8\.7 is installed/);
  assert.match(stdout, /installed but not running/);
  assert.match(stdout, /codex plugin add remote-latexmk@remote-latexmk/);
  assert.match(stdout, /claude plugin install remote-latexmk@remote-latexmk/);
  assert.match(stdout, /remote-latexmk@9\.8\.7 auth login --server "http:\/\/127\.0\.0\.1:8080"/);
  assert.match(stdout, /Server listen address: 127\.0\.0\.1:8080/);
  assert.ok(stdout.indexOf('claude plugin install') < stdout.indexOf('| REMOTE-LATEXMK API TOKEN'));
  assert.ok(stdout.indexOf('auth login --server') < stdout.indexOf('| REMOTE-LATEXMK API TOKEN'));
  assert.match(stdout.trimEnd(), new RegExp(`\\| ${token} \\|\\n\\+[-]+\\+$`));
  const shownToken = await execFileAsync(path.join(installRoot, 'bin/remote-latexmkctl'), ['token'], {
    env: { ...process.env, REMOTE_LATEXMK_HOME: installRoot },
  });
  assert.equal(shownToken.stdout.trim(), token);
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
  assert.match(stdout, /engines:\s+xelatex,pdflatex/);

  const { stdout: luaStdout } = await execFileAsync('bash', [path.join(root, 'scripts/install-server.sh'),
    '--version', 'v1.2.3', '--profile', 'full', '--engines', 'xelatex,lualatex,pdflatex', '--dry-run'], {
    env: { ...process.env, REMOTE_LATEXMK_TEST_OS: 'Linux', REMOTE_LATEXMK_TEST_ARCH: 'aarch64' },
  });
  assert.match(luaStdout, /engines:\s+xelatex,lualatex,pdflatex/);

  await assert.rejects(execFileAsync('bash', [path.join(root, 'scripts/install-server.sh'),
    '--version', 'v1.2.3', '--profile', 'slim', '--engines', 'lualatex', '--dry-run'], {
    env: { ...process.env, REMOTE_LATEXMK_TEST_OS: 'Linux' },
  }), /lualatex requires --profile full/);

  await assert.rejects(execFileAsync('bash', [path.join(root, 'scripts/install-server.sh'), '--version', 'main', '--dry-run'], {
    env: { ...process.env, REMOTE_LATEXMK_TEST_OS: 'Linux' },
  }), /--version must be an immutable tag/);
});

test('interactive installer lists interfaces, keeps safe defaults, and selects an explicit listener', async () => {
  const temp = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-wizard-'));
  await createNativeRelease(temp, '1.2.3');
  const texBin = await createFakeTexBin(temp);
  const installRoot = path.join(temp, 'home', '.remote-latexmk');
  const input = path.join(temp, 'answers');
  // full, all three engines, the VPN-labelled address, port 9090, fallback service, confirm
  await writeFile(input, '\n4\n3\n9090\n3\n\n');
  const { stdout } = await execFileAsync('bash', [path.join(root, 'scripts/install-server.sh'),
    '--version', 'v1.2.3', '--install-dir', installRoot, '--interactive', '--no-start'], {
    env: {
      ...nativeEnv(temp, texBin),
      REMOTE_LATEXMK_TEST_INPUT_FILE: input,
      REMOTE_LATEXMK_TEST_INTERFACES: 'eth0|192.168.50.20\ntailscale0|100.64.10.4\neth1|2001:db8::20\neth2|8.8.8.8',
    },
  });
  const config = await readFile(path.join(installRoot, 'config/server.env'), 'utf8');
  assert.match(stdout, /127\.0\.0\.1 — local machine or SSH tunnel \(recommended\)/);
  assert.match(stdout, /tailscale0 \(VPN interface\) — 100\.64\.10\.4/);
  assert.match(stdout, /eth1 — 2001:db8::20 \(may be public\)/);
  assert.match(stdout, /eth2 — 8\.8\.8\.8 \(may be public\)/);
  assert.ok(stdout.indexOf('Enter another host or IP') < stdout.indexOf('0.0.0.0 — all IPv4 interfaces'));
  assert.ok(stdout.indexOf('0.0.0.0 — all IPv4 interfaces') < stdout.indexOf(':: — all IPv6 interfaces'));
  assert.match(stdout, /Install plan[\s\S]*release:\s+v1\.2\.3 .*verified server archive will be downloaded/);
  assert.match(stdout, new RegExp(`install:\\s+${installRoot.replaceAll('/', '\\/')}`));
  assert.match(stdout, /profile:\s+full/);
  assert.match(stdout, /engines:\s+xelatex,pdflatex,lualatex/);
  assert.match(stdout, /listen:\s+http:\/\/100\.64\.10\.4:9090/);
  assert.match(stdout, /service:\s+none/);
  assert.match(stdout, /TeX Live:\s+install a private TeX Live/);
  assert.match(config, /LATEXMK_ADDR="100\.64\.10\.4:9090"/);
  assert.match(config, /LATEXMK_ENGINES="xelatex,pdflatex,lualatex"/);
  assert.match(config, /REMOTE_LATEXMK_SERVICE_MODE="fallback"/);
});

test('wildcard listener is last, needs confirmation, and is never printed as a client URL', async () => {
  const temp = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-wildcard-'));
  await createNativeRelease(temp, '1.2.3');
  const texBin = await createFakeTexBin(temp);
  const installRoot = path.join(temp, 'home', '.remote-latexmk');
  const input = path.join(temp, 'answers');
  // defaults, wildcard (fourth with one discovered address), explicit warning confirmation, defaults
  await writeFile(input, '\n\n4\ny\n\n3\n\n');
  const { stdout, stderr } = await execFileAsync('bash', [path.join(root, 'scripts/install-server.sh'),
    '--version', 'v1.2.3', '--install-dir', installRoot, '--interactive', '--no-start'], {
    env: {
      ...nativeEnv(temp, texBin),
      REMOTE_LATEXMK_TEST_INPUT_FILE: input,
      REMOTE_LATEXMK_TEST_INTERFACES: 'eth0|192.168.50.20',
    },
  });
  assert.match(stdout + stderr, /0\.0\.0\.0 listens on every IPv4 interface/);
  assert.match(stdout, /listen:\s+0\.0\.0\.0:8080 \(bind address only; not a client URL\)/);
  assert.doesNotMatch(stdout, /http:\/\/0\.0\.0\.0:8080/);
  assert.match(stdout, /--server "SERVER_URL"/);
  assert.match(await readFile(path.join(installRoot, 'config/server.env'), 'utf8'), /LATEXMK_ADDR="0\.0\.0\.0:8080"/);
});

test('IPv6 wildcard listener is last and needs explicit confirmation', async () => {
  const temp = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-ipv6-wildcard-'));
  await createNativeRelease(temp, '1.2.3');
  const texBin = await createFakeTexBin(temp);
  const installRoot = path.join(temp, 'home', '.remote-latexmk');
  const input = path.join(temp, 'answers');
  await writeFile(input, '\n\n5\ny\n\n3\n\n');
  const { stdout, stderr } = await execFileAsync('bash', [path.join(root, 'scripts/install-server.sh'),
    '--version', 'v1.2.3', '--install-dir', installRoot, '--interactive', '--no-start'], {
    env: {
      ...nativeEnv(temp, texBin),
      REMOTE_LATEXMK_TEST_INPUT_FILE: input,
      REMOTE_LATEXMK_TEST_INTERFACES: 'eth0|192.168.50.20',
    },
  });
  assert.match(stdout + stderr, /:: listens on every IPv6 interface/);
  assert.match(stdout, /listen:\s+\[::\]:8080 \(bind address only; not a client URL\)/);
  assert.doesNotMatch(stdout, /http:\/\/\[::\]:8080/);
  assert.match(stdout, /--server "SERVER_URL"/);
  assert.match(await readFile(path.join(installRoot, 'config/server.env'), 'utf8'), /LATEXMK_ADDR="\[::\]:8080"/);
});

test('an existing wildcard listener never becomes the wizard default', async () => {
  for (const [index, wildcard, expectedLabel] of [
    [0, '0.0.0.0:8080', /0\.0\.0\.0 .*current configuration/],
    [1, '[::]:8080', /:: .*current configuration/],
  ]) {
    const temp = await mkdtemp(path.join(os.tmpdir(), `remote-latexmk-wildcard-default-${index}-`));
    await createNativeRelease(temp, '1.2.3');
    const texBin = await createFakeTexBin(temp);
    const installRoot = path.join(temp, 'home', '.remote-latexmk');
    const env = nativeEnv(temp, texBin);
    await execFileAsync('bash', [path.join(root, 'scripts/install-server.sh'),
      '--version', 'v1.2.3', '--install-dir', installRoot, '--service', 'none',
      '--listen', wildcard, '--no-start', '--non-interactive'], { env });
    await createNativeRelease(temp, '1.2.4');
    const input = path.join(temp, 'answers');
    await writeFile(input, '\n\n\n\n\n\n');
    const { stdout } = await execFileAsync('bash', [path.join(root, 'scripts/install-server.sh'),
      '--version', 'v1.2.4', '--install-dir', installRoot, '--interactive', '--no-start'], {
      env: { ...env, REMOTE_LATEXMK_TEST_INPUT_FILE: input, REMOTE_LATEXMK_TEST_INTERFACES: '' },
    });
    assert.match(stdout, expectedLabel);
    assert.match(stdout, /Listen address[\s\S]*Choice \[1\]:/);
    assert.match(await readFile(path.join(installRoot, 'config/server.env'), 'utf8'), /LATEXMK_ADDR="127\.0\.0\.1:8080"/);
  }
});

test('native upgrade verifies and runs the target installer while preserving installation state', async () => {
  const temp = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-upgrade-'));
  await createNativeRelease(temp, '1.2.3');
  const texBin = await createFakeTexBin(temp);
  const installRoot = path.join(temp, 'home', '.remote-latexmk');
  const env = nativeEnv(temp, texBin);
  await execFileAsync('bash', [path.join(root, 'scripts/install-server.sh'),
    '--version', 'v1.2.3', '--install-dir', installRoot, '--profile', 'slim',
    '--engines', 'xelatex', '--listen', '10.20.30.40:9123', '--service', 'auto',
    '--no-start', '--non-interactive'], { env });
  const tokenBefore = await readFile(path.join(installRoot, 'config/token'), 'utf8');
  await writeFile(path.join(installRoot, 'state', 'keep-me'), 'state\n');

  await createNativeRelease(temp, '1.2.4', {
    installerTransform: (value) => `${value}\nprintf '%s\\n' target-installer >"${'${install_root}'}/state/target-migration"\n`,
  });
  const control = path.join(installRoot, 'bin/remote-latexmkctl');
  const preview = await execFileAsync(control, ['upgrade', '--version', 'v1.2.4', '--dry-run'], {
    env: { ...env, REMOTE_LATEXMK_HOME: installRoot },
  });
  assert.match(preview.stdout, /Would install remote-latexmk v1\.2\.4/);
  assert.match(await readlink(path.join(installRoot, 'current')), /releases\/1\.2\.3\/linux-amd64$/);
  const { stdout } = await execFileAsync(control, ['upgrade', '--version', 'v1.2.4'], {
    env: { ...env, REMOTE_LATEXMK_HOME: installRoot },
  });
  assert.match(stdout, /Preparing remote-latexmk update: v1\.2\.3 -> v1\.2\.4/);
  assert.match(stdout, /installed but not running/);
  assert.match(await readlink(path.join(installRoot, 'current')), /releases\/1\.2\.4\/linux-amd64$/);
  assert.equal(await readFile(path.join(installRoot, 'config/token'), 'utf8'), tokenBefore);
  assert.equal(await readFile(path.join(installRoot, 'state', 'keep-me'), 'utf8'), 'state\n');
  assert.equal(await readFile(path.join(installRoot, 'state', 'target-migration'), 'utf8'), 'target-installer\n');
  const config = await readFile(path.join(installRoot, 'config/server.env'), 'utf8');
  assert.match(config, /REMOTE_LATEXMK_PROFILE="slim"/);
  assert.match(config, /LATEXMK_ENGINES="xelatex"/);
  assert.match(config, /LATEXMK_ADDR="10\.20\.30\.40:9123"/);
  assert.match(config, /REMOTE_LATEXMK_SERVICE_MODE="stopped"/);
  assert.match(await readFile(path.join(installRoot, 'config/install.env'), 'utf8'), /REMOTE_LATEXMK_ACTIVE_VERSION="v1\.2\.4"/);
  assert.equal((await execFileAsync(control, ['version'], { env: { ...env, REMOTE_LATEXMK_HOME: installRoot } })).stdout.trim(), 'v1.2.4');
});

test('native upgrade rejects a bad target installer checksum without changing the active release', async () => {
  const temp = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-upgrade-checksum-'));
  await createNativeRelease(temp, '1.2.3');
  const texBin = await createFakeTexBin(temp);
  const installRoot = path.join(temp, 'home', '.remote-latexmk');
  const env = nativeEnv(temp, texBin);
  await execFileAsync('bash', [path.join(root, 'scripts/install-server.sh'),
    '--version', 'v1.2.3', '--install-dir', installRoot, '--service', 'auto', '--no-start', '--non-interactive'], { env });
  const configBefore = await readFile(path.join(installRoot, 'config/server.env'), 'utf8');
  const target = await createNativeRelease(temp, '1.2.4');
  await writeFile(path.join(target.release, 'install-server.sh'), '\n# modified after checksum\n', { flag: 'a' });
  await assert.rejects(execFileAsync(path.join(installRoot, 'bin/remote-latexmkctl'),
    ['upgrade', '--version', 'v1.2.4'], { env: { ...env, REMOTE_LATEXMK_HOME: installRoot } }), /checksum mismatch/);
  assert.match(await readlink(path.join(installRoot, 'current')), /releases\/1\.2\.3\/linux-amd64$/);
  assert.equal(await readFile(path.join(installRoot, 'config/server.env'), 'utf8'), configBefore);
});

test('target installer failure rolls back the active release and config', async () => {
  const temp = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-upgrade-rollback-'));
  await createNativeRelease(temp, '1.2.3');
  const texBin = await createFakeTexBin(temp);
  const installRoot = path.join(temp, 'home', '.remote-latexmk');
  const env = nativeEnv(temp, texBin);
  await execFileAsync('bash', [path.join(root, 'scripts/install-server.sh'),
    '--version', 'v1.2.3', '--install-dir', installRoot, '--listen', '10.0.0.5:8088',
    '--service', 'auto', '--no-start', '--non-interactive'], { env });
  const configBefore = await readFile(path.join(installRoot, 'config/server.env'), 'utf8');
  const stateBefore = await readFile(path.join(installRoot, 'config/install.env'), 'utf8');
  await createNativeRelease(temp, '1.2.4', {
    installerTransform: failAfterActivation,
  });
  await assert.rejects(execFileAsync(path.join(installRoot, 'bin/remote-latexmkctl'),
    ['upgrade', '--version', 'v1.2.4'], { env: { ...env, REMOTE_LATEXMK_HOME: installRoot } }));
  assert.match(await readlink(path.join(installRoot, 'current')), /releases\/1\.2\.3\/linux-amd64$/);
  assert.equal(await readFile(path.join(installRoot, 'config/server.env'), 'utf8'), configBefore);
  assert.equal(await readFile(path.join(installRoot, 'config/install.env'), 'utf8'), stateBefore);
});

test('direct reinstall migrates a legacy config and preserves supported server tuning', async () => {
  const temp = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-legacy-migration-'));
  await createNativeRelease(temp, '1.2.3');
  const texBin = await createFakeTexBin(temp);
  const installRoot = path.join(temp, 'home', '.remote-latexmk');
  const env = nativeEnv(temp, texBin);
  await execFileAsync('bash', [path.join(root, 'scripts/install-server.sh'),
    '--version', 'v1.2.3', '--install-dir', installRoot, '--service', 'none',
    '--no-start', '--non-interactive'], { env });

  const configPath = path.join(installRoot, 'config/server.env');
  const overridePath = path.join(installRoot, 'config/server.override.env');
  const migratedTuning = [
    'LATEXMK_COMPILE_TIMEOUT="9m"',
    'LATEXMK_MAX_FILES="4321"',
    "LATEXMK_CORS_ORIGINS='https://paper.example,https://lab.example'",
    "DATABASE_URL='postgres://user:p$a\\ss@db.example/papers?sslmode=require&x=1'",
    '',
  ].join('\n');
  const legacy = (await readFile(configPath, 'utf8'))
    .replace(/^LATEXMK_TOOLCHAIN_PATH=.*\n/m, '')
    .concat(migratedTuning);
  await writeFile(configPath, legacy, { mode: 0o600 });
  await unlink(overridePath);

  await createNativeRelease(temp, '1.2.4');
  const reinstall = await execFileAsync('bash', [path.join(root, 'scripts/install-server.sh'),
    '--version', 'v1.2.4', '--install-dir', installRoot, '--non-interactive'], { env });
  assert.match(reinstall.stdout, /installed but not running/);
  assert.match(await readFile(configPath, 'utf8'), new RegExp(`LATEXMK_TOOLCHAIN_PATH="${texBin.replaceAll('/', '\\/')}:/usr/local/bin:/usr/bin:/bin"`));
  assert.equal(await readFile(overridePath, 'utf8'), migratedTuning);
  assert.match(await readFile(path.join(installRoot, 'config/install.env'), 'utf8'), /REMOTE_LATEXMK_SERVICE="fallback"/);

  await createNativeRelease(temp, '1.2.5');
  await execFileAsync(path.join(installRoot, 'bin/remote-latexmkctl'), ['upgrade', '--version', 'v1.2.5'], {
    env: { ...env, REMOTE_LATEXMK_HOME: installRoot },
  });
  assert.equal(await readFile(overridePath, 'utf8'), migratedTuning);
});

test('invalid override diagnostics never echo a secret value', async () => {
  const temp = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-override-secret-'));
  await createNativeRelease(temp, '1.2.3');
  const texBin = await createFakeTexBin(temp);
  const installRoot = path.join(temp, 'home', '.remote-latexmk');
  const env = nativeEnv(temp, texBin);
  await execFileAsync('bash', [path.join(root, 'scripts/install-server.sh'),
    '--version', 'v1.2.3', '--install-dir', installRoot, '--service', 'none',
    '--no-start', '--non-interactive'], { env });
  const secret = 'DO_NOT_PRINT_THIS_SECRET_39f8d1';
  await writeFile(path.join(installRoot, 'config/server.override.env'),
    `LATEXMK_COMPILE_TIMEOUT="$(printf ${secret})"\n`, { mode: 0o600 });
  const control = path.join(installRoot, 'bin/remote-latexmkctl');

  let controlFailure;
  try {
    await execFileAsync(control, ['doctor'], { env: { ...env, REMOTE_LATEXMK_HOME: installRoot } });
  } catch (error) {
    controlFailure = error;
  }
  assert.ok(controlFailure);
  assert.match(controlFailure.stderr, /server\.override\.env at line 1/);
  assert.match(controlFailure.stderr, /LATEXMK_COMPILE_TIMEOUT/);
  assert.doesNotMatch(controlFailure.stderr, new RegExp(secret));

  await createNativeRelease(temp, '1.2.4');
  let installerFailure;
  try {
    await execFileAsync('bash', [path.join(root, 'scripts/install-server.sh'),
      '--version', 'v1.2.4', '--install-dir', installRoot, '--no-start', '--non-interactive'], { env });
  } catch (error) {
    installerFailure = error;
  }
  assert.ok(installerFailure);
  assert.match(installerFailure.stderr, /server\.override\.env at line 1/);
  assert.match(installerFailure.stderr, /LATEXMK_COMPILE_TIMEOUT/);
  assert.doesNotMatch(installerFailure.stderr, new RegExp(secret));

  await unlink(path.join(installRoot, 'config/server.override.env'));
  await writeFile(path.join(installRoot, 'config/server.env'), `LATEXMK_MAX_FILES=$'${secret}'\n`, { flag: 'a' });
  let legacyFailure;
  try {
    await execFileAsync('bash', [path.join(root, 'scripts/install-server.sh'),
      '--version', 'v1.2.4', '--install-dir', installRoot, '--no-start', '--non-interactive'], { env });
  } catch (error) {
    legacyFailure = error;
  }
  assert.ok(legacyFailure);
  assert.match(legacyFailure.stderr, /server\.env at line \d+.*LATEXMK_MAX_FILES/);
  assert.doesNotMatch(legacyFailure.stderr, new RegExp(secret));
});

test('same-version and stale target release contents are restored after activation failure', async () => {
  const temp = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-release-rollback-'));
  await createNativeRelease(temp, '1.2.3', { serverContent: '#!/bin/sh\n# original-release\nexit 0\n' });
  const texBin = await createFakeTexBin(temp);
  const installRoot = path.join(temp, 'home', '.remote-latexmk');
  const env = nativeEnv(temp, texBin);
  await execFileAsync('bash', [path.join(root, 'scripts/install-server.sh'),
    '--version', 'v1.2.3', '--install-dir', installRoot, '--service', 'none',
    '--no-start', '--non-interactive'], { env });
  const activeServer = path.join(installRoot, 'releases/1.2.3/linux-amd64/remote-latexmk-server');
  const originalServer = await readFile(activeServer, 'utf8');

  await createNativeRelease(temp, '1.2.3', {
    installerTransform: failAfterActivation,
    serverContent: '#!/bin/sh\n# replacement-release\nexit 0\n',
  });
  await assert.rejects(execFileAsync(path.join(installRoot, 'bin/remote-latexmkctl'),
    ['upgrade', '--version', 'v1.2.3'], { env: { ...env, REMOTE_LATEXMK_HOME: installRoot } }));
  assert.equal(await readFile(activeServer, 'utf8'), originalServer);

  const staleDir = path.join(installRoot, 'releases/1.2.4/linux-amd64');
  await mkdir(staleDir, { recursive: true });
  await writeFile(path.join(staleDir, 'stale-marker'), 'keep stale copy\n');
  await createNativeRelease(temp, '1.2.4', {
    installerTransform: failAfterActivation,
    serverContent: '#!/bin/sh\n# failed-new-release\nexit 0\n',
  });
  await assert.rejects(execFileAsync(path.join(installRoot, 'bin/remote-latexmkctl'),
    ['upgrade', '--version', 'v1.2.4'], { env: { ...env, REMOTE_LATEXMK_HOME: installRoot } }));
  assert.equal(await readFile(path.join(staleDir, 'stale-marker'), 'utf8'), 'keep stale copy\n');
  assert.match(await readlink(path.join(installRoot, 'current')), /releases\/1\.2\.3\/linux-amd64$/);
});

test('a running fallback service stays running across a verified upgrade', { skip: process.platform !== 'linux' }, async (t) => {
  const temp = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-running-upgrade-'));
  const port = await findFreePort();
  const serverContent = fakeHttpServerSource();
  await createNativeRelease(temp, '1.2.3', { serverContent });
  const texBin = await createFakeTexBin(temp);
  const installRoot = path.join(temp, 'home', '.remote-latexmk');
  const env = nativeEnv(temp, texBin);
  const control = path.join(installRoot, 'bin/remote-latexmkctl');
  t.after(async () => {
    try { await execFileAsync(control, ['stop'], { env: { ...env, REMOTE_LATEXMK_HOME: installRoot } }); } catch {}
  });
  await execFileAsync('bash', [path.join(root, 'scripts/install-server.sh'),
    '--version', 'v1.2.3', '--install-dir', installRoot, '--service', 'none',
    '--listen', `127.0.0.1:${port}`, '--non-interactive'], { env });
  const oldPid = (await readFile(path.join(installRoot, 'run/server.pid'), 'utf8')).split(/\s+/)[0];

  await createNativeRelease(temp, '1.2.4', { serverContent });
  const upgrade = await execFileAsync(control, ['upgrade', '--version', 'v1.2.4'], {
    env: { ...env, REMOTE_LATEXMK_HOME: installRoot },
  });
  const newPid = (await readFile(path.join(installRoot, 'run/server.pid'), 'utf8')).split(/\s+/)[0];
  assert.notEqual(newPid, oldPid);
  assert.match(upgrade.stdout, /running and passed its health and identity checks/);
  assert.match((await execFileAsync(control, ['status'], {
    env: { ...env, REMOTE_LATEXMK_HOME: installRoot },
  })).stdout, /is running/);
});

test('native systemd unit hides home and exposes only required writable state', async () => {
  const installer = await readFile(path.join(root, 'scripts/install-server.sh'), 'utf8');
  const control = await readFile(path.join(root, 'scripts/remote-latexmkctl'), 'utf8');
  assert.match(installer, /ProtectHome=tmpfs/);
  assert.match(installer, /BindReadOnlyPaths=\$\{release_dir\}/);
  assert.match(installer, /BindReadOnlyPaths=\$\{tex_root\}/);
  assert.match(installer, /BindReadOnlyPaths=\$\{install_root\}\/config/);
  assert.match(installer, /EnvironmentFile=-\$\{override_file\}/);
  assert.match(installer, /systemctl --user is-enabled --quiet remote-latexmk\.service/);
  assert.match(installer, /BindPaths=\$\{install_root\}\/state/);
  assert.match(installer, /BindPaths=\$\{install_root\}\/run/);
  assert.match(installer, /CapabilityBoundingSet=\n/);
  assert.match(installer, /\[\[ -t 1 && "\$\{TERM:-dumb\}" != dumb && -z "\$\{NO_COLOR\+x\}" \]\]/);
  assert.match(installer, /accent=\$'\\033\[1;38;2;103;232;197m'/);
  assert.match(installer, /accent=\$'\\033\[1;36m'/);
  for (const script of [installer, control]) {
    assert.match(script, /--noproxy '\*' --connect-timeout 1 --max-time 3/);
    assert.match(script, /--no-proxy --max-redirect=0 --tries=1 --timeout=3/);
  }
});

test('control command previews and atomically applies a listener change', async () => {
  const temp = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-configure-'));
  const installRoot = path.join(temp, '.remote-latexmk');
  const configDir = path.join(installRoot, 'config');
  const binDir = path.join(installRoot, 'bin');
  await mkdir(configDir, { recursive: true });
  await mkdir(binDir, { recursive: true });
  const control = path.join(binDir, 'remote-latexmkctl');
  await cp(path.join(root, 'scripts/remote-latexmkctl'), control);
  await chmod(control, 0o755);
  await writeFile(path.join(configDir, 'server.env'), [
    `PATH="${process.env.PATH}"`,
    `REMOTE_LATEXMK_SERVER_BIN="${path.join(binDir, 'remote-latexmk-server')}"`,
    'REMOTE_LATEXMK_PROFILE="full"',
    'REMOTE_LATEXMK_SERVICE_MODE="stopped"',
    'LATEXMK_ADDR="127.0.0.1:8080"',
    'LATEXMK_ENGINES="xelatex,pdflatex"',
    '',
  ].join('\n'), { mode: 0o600 });
  await writeFile(path.join(configDir, 'install.env'), 'REMOTE_LATEXMK_LISTEN="127.0.0.1:8080"\n', { mode: 0o600 });
  const env = { ...process.env, REMOTE_LATEXMK_HOME: installRoot };
  const preview = await execFileAsync(control, ['configure', '--listen', '100.64.0.4:9090'], { env });
  assert.match(preview.stdout, /Preview only/);
  assert.match(await readFile(path.join(configDir, 'server.env'), 'utf8'), /LATEXMK_ADDR="127\.0\.0\.1:8080"/);
  const apply = await execFileAsync(control, ['configure', '--listen', '100.64.0.4:9090', '--yes'], { env });
  assert.match(apply.stdout, /updated to 100\.64\.0\.4:9090/);
  assert.match(await readFile(path.join(configDir, 'server.env'), 'utf8'), /LATEXMK_ADDR="100\.64\.0\.4:9090"/);
  assert.match(await readFile(path.join(configDir, 'install.env'), 'utf8'), /REMOTE_LATEXMK_LISTEN="100\.64\.0\.4:9090"/);
});

test('systemd listener updates preserve enablement on success and rollback', async () => {
  const realCurl = (await execFileAsync('sh', ['-c', 'command -v curl'])).stdout.trim();
  for (const scenario of [
    { name: 'success-disabled', service: 'remote-latexmk', enabled: '0', succeeds: true, finalAction: /--user disable remote-latexmk\.service$/ },
    { name: 'rollback-enabled', service: 'wrong-service', enabled: '1', succeeds: false, finalAction: /--user enable remote-latexmk\.service$/ },
    { name: 'redirect-disabled', service: 'redirect', enabled: '0', succeeds: false, finalAction: /--user disable remote-latexmk\.service$/ },
  ]) {
    const temp = await mkdtemp(path.join(os.tmpdir(), `remote-latexmk-systemd-configure-${scenario.name}-`));
    const installRoot = path.join(temp, '.remote-latexmk');
    const configDir = path.join(installRoot, 'config');
    const binDir = path.join(temp, 'test-bin');
    const controlDir = path.join(installRoot, 'bin');
    const home = path.join(temp, 'home');
    const unitDir = path.join(home, '.config/systemd/user');
    await mkdir(configDir, { recursive: true });
    await mkdir(binDir, { recursive: true });
    await mkdir(controlDir, { recursive: true });
    await mkdir(unitDir, { recursive: true });
    await createSystemctlStub(binDir);
    await createCurlLoggingWrapper(binDir, realCurl);
    const control = path.join(controlDir, 'remote-latexmkctl');
    await cp(path.join(root, 'scripts/remote-latexmkctl'), control);
    await chmod(control, 0o755);
    await writeFile(path.join(unitDir, 'remote-latexmk.service'), '[Service]\nExecStart=/bin/true\n');
    const oldPort = await findFreePort();
    const newPort = await findFreePort();
    await writeFile(path.join(configDir, 'server.env'), [
      `PATH="${binDir}:${process.env.PATH}"`,
      'REMOTE_LATEXMK_SERVER_BIN="/bin/true"',
      'REMOTE_LATEXMK_PROFILE="full"',
      'REMOTE_LATEXMK_SERVICE_MODE="systemd"',
      `LATEXMK_ADDR="127.0.0.1:${oldPort}"`,
      'LATEXMK_ENGINES="xelatex,pdflatex"',
      '',
    ].join('\n'), { mode: 0o600 });
    await writeFile(path.join(configDir, 'install.env'), `REMOTE_LATEXMK_LISTEN="127.0.0.1:${oldPort}"\n`, { mode: 0o600 });
    const systemctlLog = path.join(temp, 'systemctl.log');
    const curlLog = path.join(temp, 'curl.log');
    await writeFile(systemctlLog, '');
    await writeFile(curlLog, '');
    const closeServer = scenario.service === 'redirect'
      ? await startRedirectServer(newPort)
      : await startMetadataServer(newPort, scenario.service);
    const env = {
      ...process.env,
      HOME: home,
      REMOTE_LATEXMK_HOME: installRoot,
      REMOTE_LATEXMK_TEST_SYSTEMCTL_LOG: systemctlLog,
      REMOTE_LATEXMK_TEST_SYSTEMD_ENABLED: scenario.enabled,
      REMOTE_LATEXMK_TEST_CURL_LOG: curlLog,
    };
    let failure;
    try {
      await execFileAsync(control, ['configure', '--listen', `127.0.0.1:${newPort}`, '--yes'], { env });
    } catch (error) {
      failure = error;
    } finally {
      await closeServer();
    }
    assert.equal(!failure, scenario.succeeds);
    const systemctlCalls = (await readFile(systemctlLog, 'utf8')).trim().split('\n');
    assert.ok(systemctlCalls.some((line) => line.includes('--user is-enabled --quiet remote-latexmk.service')));
    assert.match(systemctlCalls.filter((line) => /--user (?:enable|disable)(?: --now)? remote-latexmk\.service/.test(line)).at(-1), scenario.finalAction);
    const probeCall = (await readFile(curlLog, 'utf8')).split('\n').find((line) => line.includes('/healthz'));
    assert.match(probeCall, /--noproxy \*/);
    assert.doesNotMatch(probeCall, /--location/);
    const config = await readFile(path.join(configDir, 'server.env'), 'utf8');
    if (scenario.succeeds) assert.match(config, new RegExp(`LATEXMK_ADDR="127\\.0\\.0\\.1:${newPort}"`));
    else assert.match(config, new RegExp(`LATEXMK_ADDR="127\\.0\\.0\\.1:${oldPort}"`));
  }
});

test('installer identity failure disables a newly created systemd unit', async () => {
  const temp = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-systemd-install-rollback-'));
  await createNativeRelease(temp, '1.2.3');
  const texBin = await createFakeTexBin(temp);
  const realCurl = (await execFileAsync('sh', ['-c', 'command -v curl'])).stdout.trim();
  await createSystemctlStub(texBin);
  await createCurlLoggingWrapper(texBin, realCurl);
  const installRoot = path.join(temp, 'home', '.remote-latexmk');
  const port = await findFreePort();
  const closeServer = await startMetadataServer(port, 'wrong-service');
  const systemctlLog = path.join(temp, 'systemctl.log');
  const curlLog = path.join(temp, 'curl.log');
  await writeFile(systemctlLog, '');
  await writeFile(curlLog, '');
  const env = {
    ...nativeEnv(temp, texBin),
    PATH: `${texBin}:${process.env.PATH}`,
    REMOTE_LATEXMK_TEST_SYSTEMCTL_LOG: systemctlLog,
    REMOTE_LATEXMK_TEST_SYSTEMD_ENABLED: '0',
    REMOTE_LATEXMK_TEST_CURL_LOG: curlLog,
  };
  let failure;
  try {
    await execFileAsync('bash', [path.join(root, 'scripts/install-server.sh'),
      '--version', 'v1.2.3', '--install-dir', installRoot, '--service', 'systemd',
      '--listen', `127.0.0.1:${port}`, '--non-interactive'], { env });
  } catch (error) {
    failure = error;
  } finally {
    await closeServer();
  }
  assert.ok(failure);
  assert.match(`${failure.stdout}\n${failure.stderr}`, /does not identify as remote-latexmk/);
  assert.match(await readFile(systemctlLog, 'utf8'), /--user disable --now remote-latexmk\.service/);
  await assert.rejects(stat(path.join(temp, 'home/.config/systemd/user/remote-latexmk.service')));
  await assert.rejects(lstat(path.join(installRoot, 'current')));
  const probeCall = (await readFile(curlLog, 'utf8')).split('\n').find((line) => line.includes('/healthz'));
  assert.match(probeCall, /--noproxy \*/);
  assert.doesNotMatch(probeCall, /--location/);
});

test('configure reports a rollback restart failure prominently', async () => {
  const temp = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-configure-restart-failure-'));
  const installRoot = path.join(temp, '.remote-latexmk');
  const configDir = path.join(installRoot, 'config');
  const binDir = path.join(temp, 'test-bin');
  const controlDir = path.join(installRoot, 'bin');
  const home = path.join(temp, 'home');
  const unitDir = path.join(home, '.config/systemd/user');
  await mkdir(configDir, { recursive: true });
  await mkdir(binDir, { recursive: true });
  await mkdir(controlDir, { recursive: true });
  await mkdir(unitDir, { recursive: true });
  await createSystemctlStub(binDir);
  const control = path.join(controlDir, 'remote-latexmkctl');
  await cp(path.join(root, 'scripts/remote-latexmkctl'), control);
  await chmod(control, 0o755);
  await writeFile(path.join(unitDir, 'remote-latexmk.service'), '[Service]\nExecStart=/bin/true\n');
  const oldPort = await findFreePort();
  const badPort = await findFreePort();
  await writeFile(path.join(configDir, 'server.env'), [
    `PATH="${binDir}:${process.env.PATH}"`,
    'REMOTE_LATEXMK_SERVER_BIN="/bin/true"',
    'REMOTE_LATEXMK_SERVICE_MODE="systemd"',
    `LATEXMK_ADDR="127.0.0.1:${oldPort}"`,
    '',
  ].join('\n'), { mode: 0o600 });
  await writeFile(path.join(configDir, 'install.env'), `REMOTE_LATEXMK_LISTEN="127.0.0.1:${oldPort}"\n`, { mode: 0o600 });
  const systemctlLog = path.join(temp, 'systemctl.log');
  const countFile = path.join(temp, 'enable-count');
  await writeFile(systemctlLog, '');
  const closeServer = await startMetadataServer(badPort, 'wrong-service');
  let failure;
  try {
    await execFileAsync(control, ['configure', '--listen', `127.0.0.1:${badPort}`, '--yes'], {
      env: {
        ...process.env,
        HOME: home,
        REMOTE_LATEXMK_HOME: installRoot,
        REMOTE_LATEXMK_TEST_SYSTEMCTL_LOG: systemctlLog,
        REMOTE_LATEXMK_TEST_SYSTEMD_ENABLED: '0',
        REMOTE_LATEXMK_TEST_SYSTEMCTL_COUNT_FILE: countFile,
        REMOTE_LATEXMK_TEST_FAIL_ENABLE_AT: '2',
      },
    });
  } catch (error) {
    failure = error;
  } finally {
    await closeServer();
  }
  assert.ok(failure);
  assert.match(failure.stderr, /FAILED TO RESTART/);
  assert.match(failure.stderr, /rollback was incomplete/);
});

test('listener change restores config and running service after identity failure', { skip: process.platform !== 'linux' }, async (t) => {
  const temp = await mkdtemp(path.join(os.tmpdir(), 'remote-latexmk-configure-rollback-'));
  const oldPort = await findFreePort();
  const badPort = await findFreePort();
  const serverContent = fakeHttpServerSource({ badIdentityPort: badPort });
  await createNativeRelease(temp, '1.2.3', { serverContent });
  const texBin = await createFakeTexBin(temp);
  const installRoot = path.join(temp, 'home', '.remote-latexmk');
  const env = nativeEnv(temp, texBin);
  const control = path.join(installRoot, 'bin/remote-latexmkctl');
  t.after(async () => {
    try { await execFileAsync(control, ['stop'], { env: { ...env, REMOTE_LATEXMK_HOME: installRoot } }); } catch {}
  });
  await execFileAsync('bash', [path.join(root, 'scripts/install-server.sh'),
    '--version', 'v1.2.3', '--install-dir', installRoot, '--service', 'none',
    '--listen', `127.0.0.1:${oldPort}`, '--non-interactive'], { env });
  const configPath = path.join(installRoot, 'config/server.env');
  const statePath = path.join(installRoot, 'config/install.env');
  const configBefore = await readFile(configPath, 'utf8');
  const stateBefore = await readFile(statePath, 'utf8');

  let failure;
  try {
    await execFileAsync(control, ['configure', '--listen', `127.0.0.1:${badPort}`, '--yes'], {
      env: { ...env, REMOTE_LATEXMK_HOME: installRoot },
    });
  } catch (error) {
    failure = error;
  }
  assert.ok(failure);
  assert.match(`${failure.stdout}\n${failure.stderr}`, /does not identify as remote-latexmk/);
  assert.match(`${failure.stdout}\n${failure.stderr}`, /previous listen address and running state were restored/);
  assert.equal(await readFile(configPath, 'utf8'), configBefore);
  assert.equal(await readFile(statePath, 'utf8'), stateBefore);
  assert.match((await execFileAsync(control, ['status'], {
    env: { ...env, REMOTE_LATEXMK_HOME: installRoot },
  })).stdout, /is running/);
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
