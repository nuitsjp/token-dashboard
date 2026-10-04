// Builds the GitHub Pages site into site/dist with the latest release's installer link.
// site/templates holds the page with {{NAME}} placeholders; open site/dist, not the template.
import { execFileSync } from 'node:child_process';
import { cpSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const site = dirname(fileURLToPath(import.meta.url));
const root = resolve(site, '..');
const dist = resolve(site, 'dist');

const release = JSON.parse(execFileSync('gh', ['release', 'view', '--json', 'tagName,assets'], { cwd: root, encoding: 'utf8' }));
const installer = release.assets.find(asset => asset.name.endsWith('-setup.exe'));
if (!installer) throw new Error(`${release.tagName} にインストーラーがありません。`);
const values = {
  VERSION: release.tagName.slice(1),
  INSTALLER_URL: installer.url,
  INSTALLER_NAME: installer.name,
  INSTALLER_SIZE: `${(installer.size / 1024 / 1024).toFixed(1)} MB`,
};

rmSync(dist, { recursive: true, force: true });
mkdirSync(dist);
const page = readFileSync(resolve(site, 'templates/index.html'), 'utf8');
writeFileSync(resolve(dist, 'index.html'), page.replace(/\{\{(\w+)\}\}/g, (_, key) => values[key]));
cpSync(resolve(site, 'assets'), resolve(dist, 'assets'), { recursive: true });
cpSync(resolve(root, 'docs/images'), resolve(dist, 'images'), { recursive: true });
console.log(`site/dist を作成しました（${release.tagName}: ${installer.name}）。`);
