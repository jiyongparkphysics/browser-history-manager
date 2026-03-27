import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, '..');

async function readRepoFile(relativePath) {
  return readFile(path.join(repoRoot, relativePath), 'utf8');
}

async function importRepoModule(relativePath) {
  return import(pathToFileURL(path.join(repoRoot, relativePath)).href);
}

test('date-time helpers render fixed-width local timestamps with seconds', async () => {
  const { formatChromeTime, formatUnixSeconds } = await importRepoModule(
    'cmd/gui/frontend/src/datetime.js'
  );

  const localDate = new Date(2026, 2, 27, 12, 34, 56);
  const chromeEpochOffset = 11644473600000;
  const chromeTimestamp = (localDate.getTime() + chromeEpochOffset) * 1000;

  assert.equal(formatChromeTime(chromeTimestamp), '2026-03-27 12:34:56');
  assert.equal(formatUnixSeconds(localDate.getTime() / 1000), '2026-03-27 12:34:56');
});

test('history and backup tables share explicit grid templates and column alignment rules', async () => {
  const styleSource = await readRepoFile('cmd/gui/frontend/src/style.css');

  assert.match(styleSource, /--history-grid-columns:\s*48px 1fr 2fr 80px 184px;/);
  assert.match(styleSource, /--backup-grid-columns:\s*48px 1\.45fr 1\.1fr 0\.7fr 0\.55fr;/);
  assert.match(styleSource, /\.vtable-header\s*\{[\s\S]*grid-template-columns:\s*var\(--history-grid-columns\);/);
  assert.match(styleSource, /\.vrow\s*\{[\s\S]*grid-template-columns:\s*var\(--history-grid-columns\);/);
  assert.match(styleSource, /\.backups-table-header\s*\{[\s\S]*grid-template-columns:\s*var\(--backup-grid-columns\);/);
  assert.match(styleSource, /\.backup-row\s*\{[\s\S]*grid-template-columns:\s*var\(--backup-grid-columns\);/);
  assert.match(styleSource, /\.vtable-header\s+\.col-url,\s*\.vrow\s+\.col-url\s*\{/);
  assert.match(styleSource, /\.vtable-header\s+\.col-visits,\s*\.vrow\s+\.col-visits\s*\{/);
  assert.match(styleSource, /\.vtable-header\s+\.col-time,\s*\.vrow\s+\.col-time\s*\{/);
  assert.match(styleSource, /\.backups-table-header\s+\.backup-col-name,\s*\.backup-row\s+\.backup-col-name\s*\{/);
  assert.match(styleSource, /\.backups-table-header\s+\.backup-col-created,\s*\.backup-row\s+\.backup-col-created\s*\{/);
  assert.match(
    styleSource,
    /\.backups-table-header\s+\.backup-col-size,\s*\.backup-row\s+\.backup-col-size,\s*\.backups-table-header\s+\.backup-col-items,\s*\.backup-row\s+\.backup-col-items\s*\{/
  );
});
