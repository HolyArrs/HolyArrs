// Disposable backend harness for the Intercom redesign preview.
// It deliberately reuses an already-built review binary; it is not build certification.
import { spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, mkdtempSync, readFileSync } from 'node:fs';
import { createServer } from 'node:net';
import { dirname, join, resolve } from 'node:path';
import { tmpdir } from 'node:os';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const repo = resolve(here, '../../..');
const backendPort = 18747;
const ports = {
	HOLYARRS_E2E_MB_PORT: 28760,
	HOLYARRS_E2E_TVMAZE_PORT: 28761,
	HOLYARRS_E2E_PLEX_PORT: 28762,
	HOLYARRS_E2E_JELLYFIN_PORT: 28763,
	HOLYARRS_E2E_TMDB_PORT: 28764,
	HOLYARRS_E2E_TRAKT_PORT: 28765,
	HOLYARRS_E2E_DISCORD_PORT: 28766,
	HOLYARRS_E2E_INDEXER_PORT: 28767,
	HOLYARRS_E2E_QBIT_PORT: 28768,
	HOLYARRS_E2E_SAB_PORT: 28769,
	HOLYARRS_E2E_SUBS_PORT: 28770,
	HOLYARRS_E2E_OLD_RADARR_PORT: 28771,
	HOLYARRS_E2E_OLD_SEERR_PORT: 28772
};
const reservedPorts = [28773, 28774];
const expectedBinHash = 'D1FA075167517E84FD834EC112C211EE84F27AA8BE8191E07DB9AE62E316C914';
const defaultArtifacts = resolve(repo, '../app-redesign-review/captures/gumloop-verification-v7');
const bin = resolve(process.env.HOLYARRS_PREVIEW_BIN || join(defaultArtifacts, process.platform === 'win32' ? 'holyarrs.exe' : 'holyarrs'));
const subsBin = resolve(process.env.HOLYARRS_PREVIEW_SUBS_BIN || join(dirname(bin), process.platform === 'win32' ? 'holyarrs-fakesubs.exe' : 'holyarrs-fakesubs'));
const requestedRuntime = process.env.HOLYARRS_PREVIEW_RUNTIME?.trim();
if (requestedRuntime && existsSync(resolve(requestedRuntime))) throw new Error('HOLYARRS_PREVIEW_RUNTIME must name a new path; refusing to replace an existing runtime.');
const runtime = requestedRuntime ? resolve(requestedRuntime) : mkdtempSync(join(tmpdir(), 'holyarrs-intercom-preview-18747-'));
const config = join(runtime, 'config');
const movies = join(runtime, 'movies');
const downloads = join(runtime, 'downloads');

for (const [name, port] of Object.entries(ports)) process.env[name] = String(port);

function assertFile(path, label) {
	if (!existsSync(path)) throw new Error(`${label} was not found at ${path}`);
}

function portIsFree(port) {
	return new Promise((resolveFree, reject) => {
		const probe = createServer();
		probe.unref();
		probe.once('error', (error) => {
			if (error && error.code === 'EADDRINUSE') resolveFree(false);
			else reject(error);
		});
		probe.listen(port, '127.0.0.1', () => probe.close(() => resolveFree(true)));
	});
}

async function assertPortsFree() {
	const requested = [backendPort, ...Object.values(ports), ...reservedPorts];
	const checks = await Promise.all(requested.map(async (port) => ({ port, free: await portIsFree(port) })));
	const occupied = checks.filter((check) => !check.free).map((check) => check.port);
	if (occupied.length) throw new Error(`Preview ports already in use: ${occupied.join(', ')}`);
}

async function waitUntilReady(child) {
	const deadline = Date.now() + 15_000;
	while (Date.now() < deadline) {
		if (child.exitCode !== null) throw new Error(`HolyArrs exited before readiness with code ${child.exitCode}`);
		try {
			const response = await fetch(`http://127.0.0.1:${backendPort}/healthz`);
			if (response.ok) return;
		} catch {
			// Startup is still in progress.
		}
		await new Promise((resolveWait) => setTimeout(resolveWait, 150));
	}
	throw new Error('HolyArrs did not become ready within 15 seconds.');
}

assertFile(bin, 'Preview HolyArrs binary');
assertFile(subsBin, 'Preview subtitle fake binary');
const actualBinHash = createHash('sha256').update(readFileSync(bin)).digest('hex').toUpperCase();
if (actualBinHash !== expectedBinHash) throw new Error(`Preview HolyArrs binary hash mismatch: expected ${expectedBinHash}, received ${actualBinHash}`);
await assertPortsFree();

// The runtime is always new. No installed, shared, or earlier preview config is read or replaced.
mkdirSync(config, { recursive: true });
mkdirSync(movies, { recursive: true });
mkdirSync(downloads, { recursive: true });

const fakes = await import('../../../web/app/e2e/fakes.mjs');
const { addCatalogToFakes } = await import('./sample-preview.mjs');
addCatalogToFakes(fakes);
const { startFakes, mbPort, plexPort, tmdbPort, traktPort, tvmazePort } = fakes;
const stopFakes = startFakes();
const subs = spawn(subsBin, ['-addr', `127.0.0.1:${ports.HOLYARRS_E2E_SUBS_PORT}`], { cwd: runtime, stdio: 'inherit' });
const plex = `http://127.0.0.1:${plexPort}`;
const backend = spawn(bin, ['-config', config, '-addr', `127.0.0.1:${backendPort}`, '-movies', movies, '-save-path', downloads], {
	cwd: runtime,
	stdio: 'inherit',
	env: {
		...process.env,
		HOLYARRS_PLEX_URL: plex,
		HOLYARRS_PLEX_APP_URL: `${plex}/app`,
		HOLYARRS_TMDB_URL: `http://127.0.0.1:${tmdbPort}`,
		HOLYARRS_RELEASE_FEED: `http://127.0.0.1:${tmdbPort}/releases/latest`,
		HOLYARRS_TRAKT_URL: `http://127.0.0.1:${traktPort}`,
		HOLYARRS_TVMAZE_URL: `http://127.0.0.1:${tvmazePort}`,
		HOLYARRS_MUSICBRAINZ_URL: `http://127.0.0.1:${mbPort}`
	}
});

let stopping = false;
function stop(code = 0) {
	if (stopping) return;
	stopping = true;
	backend.kill();
	subs.kill();
	stopFakes();
	process.exitCode = code;
}

process.on('SIGINT', () => stop(0));
process.on('SIGTERM', () => stop(0));
backend.on('exit', (code) => stop(code ?? 1));
subs.on('exit', (code) => {
	if (!stopping) {
		process.stderr.write(`Subtitle fake exited with code ${code ?? 1}.\n`);
		stop(code ?? 1);
	}
});

try {
	await waitUntilReady(backend);
	process.stdout.write(`Disposable HolyArrs backend ready on http://127.0.0.1:${backendPort}\n`);
	process.stdout.write(`Runtime: ${runtime}\n`);
} catch (error) {
	process.stderr.write(`${error instanceof Error ? error.message : String(error)}\n`);
	stop(1);
}
