import { existsSync } from 'node:fs';
import { join } from 'node:path';
import type { Platform } from './platform.js';

export interface ResolveBinaryOpts {
	platform: Platform;
	cacheDir: string;
	env: NodeJS.ProcessEnv;
	// Test seam: provide a custom fetch impl for unit tests. Defaults to
	// the global fetch shipped in Node 20+.
	fetchFn?: typeof fetch;
	// Test seam: pretend the version probe returned this string. Used by
	// tests so they never hit the network.
	versionOverride?: string;
}

export type BinaryResolveResult =
	| { kind: 'cached'; path: string }
	| { kind: 'downloaded'; path: string }
	| { kind: 'unavailable'; reason: string };

// resolveBinary tries the cache first, then a remote download. Until
// release binaries land (#368) the remote path always returns
// `unavailable` so the caller can fall back to `go run @latest`.
export async function resolveBinary(
	opts: ResolveBinaryOpts,
): Promise<BinaryResolveResult> {
	const version = opts.versionOverride ?? opts.env.SVELTEGO_VERSION ?? 'latest';
	const cachePath = join(
		opts.cacheDir,
		`sveltego-init-${version}-${opts.platform.assetSuffix}${
			opts.platform.os === 'windows' ? '.exe' : ''
		}`,
	);
	if (existsSync(cachePath)) {
		return { kind: 'cached', path: cachePath };
	}
	// Release binaries are not published yet (#368). Returning a typed
	// "unavailable" lets the wrapper print a single-line note and fall
	// back to `go run @latest` instead of hanging on a 404.
	return {
		kind: 'unavailable',
		reason: 'release binaries not yet published (#368)',
	};
}
