import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, '..');

test('sidebar filter help stays neutral and avoids special-case examples', async () => {
  const sidebarSource = await readFile(
    path.join(repoRoot, 'cmd/gui/frontend/src/components/sidebar/Sidebar.jsx'),
    'utf8'
  );

  assert.doesNotMatch(sidebarSource, /github\.com/i);
  assert.doesNotMatch(sidebarSource, /<code>docs<\/code>/i);
  assert.doesNotMatch(sidebarSource, /ads,\s*trackers/i);
  assert.doesNotMatch(sidebarSource, /\?\?/);
  assert.match(
    sidebarSource,
    /Include<\/strong>.*title or URL matches these keywords/i
  );
  assert.match(
    sidebarSource,
    /Exclude<\/strong>.*title or URL matches these keywords/i
  );
});
