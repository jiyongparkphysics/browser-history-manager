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

test('history table no longer implements shift-click range selection', async () => {
  const tableSource = await readRepoFile('cmd/gui/frontend/src/components/HistoryTable.jsx');
  const appSource = await readRepoFile('cmd/gui/frontend/src/App.jsx');

  assert.doesNotMatch(tableSource, /shift\+click/i);
  assert.doesNotMatch(tableSource, /lastClickedIndexRef/);
  assert.doesNotMatch(tableSource, /\bonRangeSelect\b/);
  assert.doesNotMatch(appSource, /\bonRangeSelect=/);
  assert.doesNotMatch(appSource, /handleRangeSelect/);
});

test('sidebar help avoids hard-coded browser support names', async () => {
  const sidebarSource = await readRepoFile('cmd/gui/frontend/src/components/sidebar/Sidebar.jsx');
  const designSpec = await readRepoFile('docs/design/DESIGN-SPEC.md');

  assert.doesNotMatch(sidebarSource, /Shift\+Click/i);
  assert.doesNotMatch(
    sidebarSource,
    /Supported:\s*Chrome,\s*Edge,\s*Brave,\s*Vivaldi,\s*Opera,\s*Chromium\./
  );
  assert.doesNotMatch(designSpec, /Shift-click range selection/i);
});

test('sidebar exposes a db-folder utility action and explains backup location', async () => {
  const sidebarSource = await readRepoFile('cmd/gui/frontend/src/components/sidebar/Sidebar.jsx');
  const appSource = await readRepoFile('cmd/gui/frontend/src/App.jsx');

  assert.match(sidebarSource, /Open DB folder/i);
  assert.match(sidebarSource, /folder_open/i);
  assert.match(appSource, /next to the selected History database/i);
});

test('sidebar keeps help in the tab header and browser utilities in the browser block', async () => {
  const sidebarSource = await readRepoFile('cmd/gui/frontend/src/components/sidebar/Sidebar.jsx');
  const readmeSource = await readRepoFile('README.md');
  const designSpec = await readRepoFile('docs/design/DESIGN-SPEC.md');

  assert.match(sidebarSource, />\s*History\s*</);
  assert.match(sidebarSource, />\s*Backups\s*</);
  assert.match(sidebarSource, /help/);
  assert.match(sidebarSource, /Refresh current view/);
  assert.match(sidebarSource, /Open DB folder/);
  assert.match(readmeSource, /Help in the tab header/i);
  assert.match(designSpec, /Refresh moves into the browser block/i);
});

test('help copy describes backup restore and delete behavior', async () => {
  const sidebarSource = await readRepoFile('cmd/gui/frontend/src/components/sidebar/Sidebar.jsx');
  const appSource = await readRepoFile('cmd/gui/frontend/src/App.jsx');

  assert.match(sidebarSource, /<strong>Restore<\/strong>/i);
  assert.match(sidebarSource, /<strong>Delete<\/strong>/i);
  assert.match(sidebarSource, /Version/i);
  assert.match(appSource, /title: 'Restore'/);
  assert.match(appSource, /title: 'Delete'/);
});

test('filter chip add flow and confirm modal actions follow the shared pill language', async () => {
  const sidebarSource = await readRepoFile('cmd/gui/frontend/src/components/sidebar/Sidebar.jsx');
  const modalSource = await readRepoFile('cmd/gui/frontend/src/components/common/ConfirmModal.jsx');
  const styleSource = await readRepoFile('cmd/gui/frontend/src/style.css');

  assert.doesNotMatch(sidebarSource, /sb-add-cancel/);
  assert.match(sidebarSource, /sb-add-submit \$\{colorClass\}/);
  assert.match(sidebarSource, /isAdding \? 'close' : 'add'/);
  assert.match(sidebarSource, /title="Add keyword"/);
  assert.match(sidebarSource, /<span className="material-symbols-outlined">add<\/span>/);
  assert.match(sidebarSource, /sb-chip-input-shell \$\{colorClass\}/);
  assert.match(styleSource, /\.sb-chip-input-shell/);
  assert.match(styleSource, /\.sb-chip-input-shell\.tertiary-tint/);
  assert.match(styleSource, /\.sb-chip-input-shell\.error-tint/);
  assert.match(styleSource, /\.sb-add-submit\.tertiary-tint/);
  assert.match(styleSource, /\.sb-add-submit\.error-tint/);
  assert.doesNotMatch(styleSource, /text-decoration:\s*line-through/);
  assert.match(modalSource, /sel-pill/);
  assert.doesNotMatch(modalSource, /modal-btn-confirm|modal-btn-danger|modal-btn-cancel/);
});

test('page selection preserves off-page selections while toggling current page rows', async () => {
  const { applyPageSelection } = await importRepoModule('cmd/gui/frontend/src/selectionState.js');

  const previous = new Set([1, 9]);
  const pageEntries = [{ id: 2 }, { id: 3 }];

  const selected = applyPageSelection(previous, pageEntries, true);
  assert.deepEqual([...selected].sort((a, b) => a - b), [1, 2, 3, 9]);

  const deselected = applyPageSelection(selected, [{ id: 2 }], false);
  assert.deepEqual([...deselected].sort((a, b) => a - b), [1, 3, 9]);
});

test('global-selection flag drops when the backing selection becomes empty', async () => {
  const { shouldKeepAllGlobalSelected } = await importRepoModule('cmd/gui/frontend/src/selectionState.js');

  assert.equal(shouldKeepAllGlobalSelected(true, new Set([1])), true);
  assert.equal(shouldKeepAllGlobalSelected(true, new Set()), false);
  assert.equal(shouldKeepAllGlobalSelected(false, new Set([1])), false);
});

test('readme distinguishes current GUI capabilities from CLI-only date filters', async () => {
  const readmeSource = await readRepoFile('README.md');

  assert.doesNotMatch(readmeSource, /The GUI provides a visual interface with the same capabilities:/);
  assert.match(readmeSource, /Date-range filtering is currently available in the CLI\./);
});

test('docs describe the history and backups tab split', async () => {
  const readmeSource = await readRepoFile('README.md');
  const designSpec = await readRepoFile('docs/design/DESIGN-SPEC.md');

  assert.match(readmeSource, /History\s*\|\s*Backups/i);
  assert.match(readmeSource, /`Restore`/i);
  assert.match(readmeSource, /`Delete`/i);
  assert.match(designSpec, /History\s*\|\s*Backups/i);
  assert.match(designSpec, /File Name/i);
  assert.match(designSpec, /Items/i);
});
