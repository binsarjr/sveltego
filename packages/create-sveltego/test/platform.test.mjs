import { test } from 'node:test';
import assert from 'node:assert/strict';
import { resolvePlatform, UnsupportedPlatformError } from '../dist/platform.js';

test('maps darwin/arm64 to darwin-arm64', () => {
	const p = resolvePlatform('darwin', 'arm64');
	assert.equal(p.os, 'darwin');
	assert.equal(p.arch, 'arm64');
	assert.equal(p.assetSuffix, 'darwin-arm64');
	assert.equal(p.binaryName, 'sveltego-init');
});

test('maps linux/x64 to linux-amd64', () => {
	const p = resolvePlatform('linux', 'x64');
	assert.equal(p.assetSuffix, 'linux-amd64');
});

test('maps win32/x64 to windows-amd64 with .exe suffix', () => {
	const p = resolvePlatform('win32', 'x64');
	assert.equal(p.assetSuffix, 'windows-amd64');
	assert.equal(p.binaryName, 'sveltego-init.exe');
});

test('rejects windows on arm64 (no GoReleaser asset planned)', () => {
	assert.throws(() => resolvePlatform('win32', 'arm64'), UnsupportedPlatformError);
});

test('rejects unknown platforms', () => {
	assert.throws(() => resolvePlatform('freebsd', 'x64'), UnsupportedPlatformError);
});

test('rejects unknown arch', () => {
	assert.throws(() => resolvePlatform('linux', 'ia32'), UnsupportedPlatformError);
});
