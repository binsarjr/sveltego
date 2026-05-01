import { test } from 'node:test';
import assert from 'node:assert/strict';
import { resolveBinary } from '../dist/download.js';

const PLATFORM = { os: 'linux', arch: 'amd64', assetSuffix: 'linux-amd64', binaryName: 'sveltego-init' };

test('returns unavailable until release binaries land (#368)', async () => {
	const r = await resolveBinary({ platform: PLATFORM, cacheDir: '/non-existent', env: {} });
	assert.equal(r.kind, 'unavailable');
	assert.match(r.reason, /#368/);
});
