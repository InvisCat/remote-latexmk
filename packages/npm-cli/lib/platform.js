import { createRequire } from 'node:module';
import path from 'node:path';
import process from 'node:process';

const require = createRequire(import.meta.url);

const packages = new Map([
  ['darwin/arm64', '@inviscat/remote-latexmk-darwin-arm64'],
  ['darwin/x64', '@inviscat/remote-latexmk-darwin-x64'],
  ['linux/arm64', '@inviscat/remote-latexmk-linux-arm64'],
  ['linux/x64', '@inviscat/remote-latexmk-linux-x64'],
  ['win32/arm64', '@inviscat/remote-latexmk-win32-arm64'],
  ['win32/x64', '@inviscat/remote-latexmk-win32-x64'],
]);

export function platformPackageName(platform = process.platform, arch = process.arch) {
  return packages.get(`${platform}/${arch}`) ?? null;
}

export function resolveNativeBinary(platform = process.platform, arch = process.arch) {
  const packageName = platformPackageName(platform, arch);
  if (!packageName) {
    throw new Error(`no native client is published for ${platform}/${arch}; use the Docker client or a native release archive`);
  }
  let packageJSON;
  try {
    packageJSON = require.resolve(`${packageName}/package.json`);
  } catch (error) {
    throw new Error(`optional package ${packageName} is missing. Reinstall remote-latexmk without --no-optional`, { cause: error });
  }
  const binary = path.join(path.dirname(packageJSON), 'bin', platform === 'win32' ? 'latexmk.exe' : 'latexmk');
  return binary;
}
