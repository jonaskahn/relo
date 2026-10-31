import assert from 'node:assert/strict';
import { test } from 'node:test';

import { createDesktopShortcut, removeDesktopShortcut } from './shortcut.mjs';

test('other platforms get no desktop shortcut', () => {
	assert.equal(createDesktopShortcut('/bin/relo', { nodePlatform: 'linux' }), false);
	assert.equal(removeDesktopShortcut({ nodePlatform: 'darwin' }), false);
});
