import { spawnSync, type SpawnSyncReturns } from 'node:child_process';
import { existsSync } from 'node:fs';
import { resolvePlatform, type Platform } from './platform.js';
import { resolveBinary, type BinaryResolveResult } from './download.js';

export interface RunOptions {
	argv: string[];
	env: NodeJS.ProcessEnv;
	stderr: NodeJS.WritableStream;
	platform: Platform;
	resolveBinaryFn?: typeof resolveBinary;
	spawnFn?: typeof spawnSync;
	hasGoOnPathFn?: () => boolean;
}

export const FALLBACK_GO_MODULE =
	'github.com/binsarjr/sveltego/packages/init/cmd/sveltego-init@latest';

export async function main(rawArgv: string[]): Promise<void> {
	const code = await run({
		argv: rawArgv,
		env: process.env,
		stderr: process.stderr,
		platform: resolvePlatform(process.platform, process.arch),
	});
	process.exit(code);
}

export async function run(opts: RunOptions): Promise<number> {
	const argv = opts.argv;
	const spawn = opts.spawnFn ?? spawnSync;
	const resolveFn = opts.resolveBinaryFn ?? resolveBinary;
	const hasGo = opts.hasGoOnPathFn ?? hasGoOnPath;

	const skipBinary =
		argv.includes('--no-binary-download') ||
		opts.env.SVELTEGO_NO_BINARY_DOWNLOAD === '1';

	const passthrough = argv.filter((a) => a !== '--no-binary-download');

	// CI escape hatch: SVELTEGO_INIT_LOCAL_PATH points at a pre-built
	// sveltego-init binary (e.g. `go build -o $tmp/sveltego-init
	// ./packages/init/cmd/sveltego-init` produced by the workflow).
	// Lets the smoke test exercise the wrapper end-to-end before the
	// release proxy carries the right commit.
	if (opts.env.SVELTEGO_INIT_LOCAL_PATH) {
		const r = spawn(opts.env.SVELTEGO_INIT_LOCAL_PATH, passthrough, {
			stdio: 'inherit',
			env: opts.env,
		});
		return exitCode(r);
	}

	if (!skipBinary) {
		let resolved: BinaryResolveResult;
		try {
			resolved = await resolveFn({
				platform: opts.platform,
				cacheDir: cacheDirFor(opts.env),
				env: opts.env,
			});
		} catch (err) {
			opts.stderr.write(
				`create-sveltego: binary resolve failed: ${(err as Error).message}\n`,
			);
			resolved = { kind: 'unavailable', reason: (err as Error).message };
		}
		if (resolved.kind === 'cached' || resolved.kind === 'downloaded') {
			const r = spawn(resolved.path, passthrough, {
				stdio: 'inherit',
				env: opts.env,
			});
			return exitCode(r);
		}
		opts.stderr.write(
			`create-sveltego: ${resolved.reason}; falling back to \`go run @latest\` (release binaries pending #368)\n`,
		);
	}

	if (hasGo()) {
		const r = spawn('go', ['run', FALLBACK_GO_MODULE, ...passthrough], {
			stdio: 'inherit',
			env: opts.env,
		});
		return exitCode(r);
	}

	opts.stderr.write(
		'create-sveltego: no scaffold path available.\n' +
			'  - install Go >= 1.23 (https://go.dev/dl/) so we can fall back to `go run @latest`, OR\n' +
			'  - wait for sveltego release binaries (https://github.com/binsarjr/sveltego/issues/368).\n',
	);
	return 1;
}

function exitCode(r: SpawnSyncReturns<Buffer>): number {
	if (r.error) {
		throw r.error;
	}
	if (typeof r.status === 'number') {
		return r.status;
	}
	return r.signal ? 1 : 0;
}

export function hasGoOnPath(): boolean {
	const r = spawnSync('go', ['version'], { stdio: 'ignore' });
	return r.status === 0;
}

function cacheDirFor(env: NodeJS.ProcessEnv): string {
	if (env.SVELTEGO_CACHE_DIR) return env.SVELTEGO_CACHE_DIR;
	const xdg = env.XDG_CACHE_HOME;
	if (xdg) return joinPath(xdg, 'sveltego');
	const home = env.HOME ?? env.USERPROFILE ?? '.';
	return joinPath(home, '.cache', 'sveltego');
}

function joinPath(...parts: string[]): string {
	return parts.join('/').replace(/\\/g, '/');
}

export { resolvePlatform } from './platform.js';
export { resolveBinary } from './download.js';
export { existsSync };
