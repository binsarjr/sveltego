export interface Platform {
	os: 'linux' | 'darwin' | 'windows';
	arch: 'amd64' | 'arm64';
	assetSuffix: string;
	binaryName: string;
}

export class UnsupportedPlatformError extends Error {
	constructor(platform: string, arch: string) {
		super(
			`unsupported platform ${platform}/${arch}; supported: linux-amd64, linux-arm64, darwin-amd64, darwin-arm64, windows-amd64`,
		);
		this.name = 'UnsupportedPlatformError';
	}
}

// resolvePlatform maps Node's process.platform/arch to GoReleaser asset
// suffixes (linux-amd64, darwin-arm64, windows-amd64, ...). Unsupported
// combos throw — there is no point silently downloading the wrong binary.
export function resolvePlatform(nodePlatform: string, nodeArch: string): Platform {
	let os: Platform['os'];
	switch (nodePlatform) {
		case 'linux':
			os = 'linux';
			break;
		case 'darwin':
			os = 'darwin';
			break;
		case 'win32':
			os = 'windows';
			break;
		default:
			throw new UnsupportedPlatformError(nodePlatform, nodeArch);
	}
	let arch: Platform['arch'];
	switch (nodeArch) {
		case 'x64':
			arch = 'amd64';
			break;
		case 'arm64':
			arch = 'arm64';
			break;
		default:
			throw new UnsupportedPlatformError(nodePlatform, nodeArch);
	}
	if (os === 'windows' && arch !== 'amd64') {
		throw new UnsupportedPlatformError(nodePlatform, nodeArch);
	}
	const assetSuffix = `${os}-${arch}`;
	const binaryName = os === 'windows' ? 'sveltego-init.exe' : 'sveltego-init';
	return { os, arch, assetSuffix, binaryName };
}
