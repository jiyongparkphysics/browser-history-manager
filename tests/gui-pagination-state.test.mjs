import test from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, '..');

async function importRepoModule(relativePath) {
  return import(pathToFileURL(path.join(repoRoot, relativePath)).href);
}

test('filter pagination clamps an out-of-range page to the last available page', async () => {
  const { clampPage, paginateEntries } = await importRepoModule('cmd/gui/frontend/src/paginationState.js');

  const entries = Array.from({ length: 6 }, (_, index) => ({ id: index + 1 }));
  const paged = paginateEntries(entries, 4, 2);

  assert.equal(clampPage(4, 3), 3);
  assert.deepEqual(paged.map((entry) => entry.id), [5, 6]);
});

test('filter pagination keeps the first page when the filtered set is empty', async () => {
  const { clampPage, paginateEntries } = await importRepoModule('cmd/gui/frontend/src/paginationState.js');

  assert.equal(clampPage(9, 0), 1);
  assert.deepEqual(paginateEntries([], 9, 50), []);
});
test('filter reload path no longer swallows SearchHistoryAll failures silently', async () => {
  const source = await import('node:fs/promises').then((fs) => fs.readFile(path.join(repoRoot, 'cmd/gui/frontend/src/App.jsx'), 'utf8'));

  assert.match(source, /Filter reload failed:/);
  assert.match(source, /toast\.error\(/);
});