import test from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, '..');

async function importRepoModule(relativePath) {
  return import(pathToFileURL(path.join(repoRoot, relativePath)).href);
}

test('backup selection enables delete for any non-empty selection', async () => {
  const { canDeleteBackups } = await importRepoModule('cmd/gui/frontend/src/backupState.js');

  assert.equal(canDeleteBackups(new Set()), false);
  assert.equal(canDeleteBackups(new Set(['a'])), true);
  assert.equal(canDeleteBackups(new Set(['a', 'b'])), true);
});

test('backup selection enables restore only for a single selected row', async () => {
  const { canRestoreBackup } = await importRepoModule('cmd/gui/frontend/src/backupState.js');

  assert.equal(canRestoreBackup(new Set()), false);
  assert.equal(canRestoreBackup(new Set(['a'])), true);
  assert.equal(canRestoreBackup(new Set(['a', 'b'])), false);
});

test('backup selection toggles rows and can be cleared', async () => {
  const { toggleBackupSelection, clearBackupSelection } = await importRepoModule('cmd/gui/frontend/src/backupState.js');

  const first = toggleBackupSelection(new Set(), 'backup-a');
  assert.deepEqual([...first], ['backup-a']);

  const second = toggleBackupSelection(first, 'backup-b');
  assert.deepEqual([...second].sort(), ['backup-a', 'backup-b']);

  const third = toggleBackupSelection(second, 'backup-a');
  assert.deepEqual([...third], ['backup-b']);

  assert.deepEqual([...clearBackupSelection()], []);
});
test('backup file-name filtering is case-insensitive and empty-query safe', async () => {
  const { filterBackupsByFileName } = await importRepoModule('cmd/gui/frontend/src/backupListState.js');

  const backups = [
    { fileName: 'History_backup_20260327_010000' },
    { fileName: 'History_backup_ProjectA' },
    { fileName: 'ManualSnapshot' },
  ];

  assert.equal(filterBackupsByFileName(backups, '').length, 3);
  assert.deepEqual(
    filterBackupsByFileName(backups, 'project').map((backup) => backup.fileName),
    ['History_backup_ProjectA']
  );
  assert.deepEqual(
    filterBackupsByFileName(backups, 'history_BACKUP').map((backup) => backup.fileName),
    ['History_backup_20260327_010000', 'History_backup_ProjectA']
  );
});
