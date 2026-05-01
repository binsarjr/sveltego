import { test } from 'node:test';
import assert from 'node:assert/strict';
import { run, FALLBACK_GO_MODULE } from '../dist/index.js';

const PLATFORM = { os: 'linux', arch: 'amd64', assetSuffix: 'linux-amd64', binaryName: 'sveltego-init' };

function fakeStderr() {
	const buf = [];
	return {
		write: (s) => buf.push(String(s)),
		text: () => buf.join(''),
	};
}

function fakeSpawn(record) {
	return (cmd, args, _opts) => {
		record.calls.push({ cmd, args });
		return record.next ?? { status: 0, signal: null, error: null };
	};
}

test('falls back to go run @latest when binary unavailable and go on PATH', async () => {
	const stderr = fakeStderr();
	const record = { calls: [] };
	const code = await run({
		argv: ['/tmp/hello'],
		env: {},
		stderr,
		platform: PLATFORM,
		resolveBinaryFn: async () => ({ kind: 'unavailable', reason: 'no release yet' }),
		spawnFn: fakeSpawn(record),
		hasGoOnPathFn: () => true,
	});
	assert.equal(code, 0);
	assert.equal(record.calls.length, 1);
	assert.equal(record.calls[0].cmd, 'go');
	assert.deepEqual(record.calls[0].args, ['run', FALLBACK_GO_MODULE, '/tmp/hello']);
	assert.match(stderr.text(), /falling back to `go run @latest`/);
});

test('uses cached binary when present', async () => {
	const stderr = fakeStderr();
	const record = { calls: [] };
	const code = await run({
		argv: ['--non-interactive', '/tmp/hello'],
		env: {},
		stderr,
		platform: PLATFORM,
		resolveBinaryFn: async () => ({ kind: 'cached', path: '/cache/sveltego-init' }),
		spawnFn: fakeSpawn(record),
		hasGoOnPathFn: () => false,
	});
	assert.equal(code, 0);
	assert.equal(record.calls[0].cmd, '/cache/sveltego-init');
	assert.deepEqual(record.calls[0].args, ['--non-interactive', '/tmp/hello']);
	assert.equal(stderr.text(), '');
});

test('passes every flag through unchanged to the binary', async () => {
	const record = { calls: [] };
	await run({
		argv: ['--ai', '--tailwind=v4', '--service-worker', '--module', 'example.com/x', '/tmp/hi'],
		env: {},
		stderr: fakeStderr(),
		platform: PLATFORM,
		resolveBinaryFn: async () => ({ kind: 'cached', path: '/cache/init' }),
		spawnFn: fakeSpawn(record),
		hasGoOnPathFn: () => false,
	});
	assert.deepEqual(record.calls[0].args, [
		'--ai',
		'--tailwind=v4',
		'--service-worker',
		'--module',
		'example.com/x',
		'/tmp/hi',
	]);
});

test('strips --no-binary-download from passthrough args', async () => {
	const record = { calls: [] };
	await run({
		argv: ['--no-binary-download', '/tmp/hi'],
		env: {},
		stderr: fakeStderr(),
		platform: PLATFORM,
		resolveBinaryFn: async () => {
			throw new Error('resolveBinary should not run when --no-binary-download is set');
		},
		spawnFn: fakeSpawn(record),
		hasGoOnPathFn: () => true,
	});
	assert.equal(record.calls[0].cmd, 'go');
	assert.deepEqual(record.calls[0].args, ['run', FALLBACK_GO_MODULE, '/tmp/hi']);
});

test('SVELTEGO_NO_BINARY_DOWNLOAD=1 also skips binary path', async () => {
	const record = { calls: [] };
	await run({
		argv: ['/tmp/hi'],
		env: { SVELTEGO_NO_BINARY_DOWNLOAD: '1' },
		stderr: fakeStderr(),
		platform: PLATFORM,
		resolveBinaryFn: async () => {
			throw new Error('should not call resolveBinary when env opts out');
		},
		spawnFn: fakeSpawn(record),
		hasGoOnPathFn: () => true,
	});
	assert.equal(record.calls[0].cmd, 'go');
});

test('fails with friendly error when neither path is available', async () => {
	const stderr = fakeStderr();
	const record = { calls: [] };
	const code = await run({
		argv: ['/tmp/hi'],
		env: {},
		stderr,
		platform: PLATFORM,
		resolveBinaryFn: async () => ({ kind: 'unavailable', reason: 'no release' }),
		spawnFn: fakeSpawn(record),
		hasGoOnPathFn: () => false,
	});
	assert.equal(code, 1);
	assert.equal(record.calls.length, 0);
	assert.match(stderr.text(), /install Go >= 1\.23/);
	assert.match(stderr.text(), /github\.com\/binsarjr\/sveltego\/issues\/368/);
});

test('SVELTEGO_INIT_LOCAL_PATH wins over both binary and go-run paths', async () => {
	const record = { calls: [] };
	await run({
		argv: ['/tmp/hi'],
		env: { SVELTEGO_INIT_LOCAL_PATH: '/tmp/local-bin' },
		stderr: fakeStderr(),
		platform: PLATFORM,
		resolveBinaryFn: async () => {
			throw new Error('should not call resolveBinary when local path is set');
		},
		spawnFn: fakeSpawn(record),
		hasGoOnPathFn: () => {
			throw new Error('should not probe go when local path is set');
		},
	});
	assert.equal(record.calls.length, 1);
	assert.equal(record.calls[0].cmd, '/tmp/local-bin');
	assert.deepEqual(record.calls[0].args, ['/tmp/hi']);
});

test('propagates non-zero exit code from underlying binary', async () => {
	const code = await run({
		argv: ['/tmp/hi'],
		env: {},
		stderr: fakeStderr(),
		platform: PLATFORM,
		resolveBinaryFn: async () => ({ kind: 'cached', path: '/cache/init' }),
		spawnFn: () => ({ status: 2, signal: null, error: null }),
		hasGoOnPathFn: () => false,
	});
	assert.equal(code, 2);
});
