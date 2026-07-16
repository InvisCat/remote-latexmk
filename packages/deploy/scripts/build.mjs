import { chmod, copyFile, mkdir } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, '..');
const dist = path.join(root, 'dist');
await mkdir(dist, { recursive: true });
await copyFile(path.join(root, 'src', 'index.ts'), path.join(dist, 'index.js'));
await chmod(path.join(dist, 'index.js'), 0o755);
console.log('built @latexmk/deploy');
