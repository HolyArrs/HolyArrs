// Isolated real-server harness for acceptance-library.spec.ts. It owns only
// e2e/.tmp-acceptance-library and ports supplied by the matching Playwright
// config. Every unused fake binds an ephemeral port.
import { spawn, spawnSync } from 'node:child_process';
import { mkdirSync, rmSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { deezerURL, mbPort, metadataEnv, startFakes, subsPort, tmdbPort, traktPort, tvmazePort } from '../fakes/index.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const repo = resolve(here, '../../..');
// The specs reach this folder through helpers/data-folder.ts; keep both in step.
const tmp = join(here, '..', '.tmp-acceptance-library');
const config = join(tmp, 'config');
const bin = join(tmp, process.platform === 'win32' ? 'holyarrs.exe' : 'holyarrs');
const subsBin = join(tmp, process.platform === 'win32' ? 'holyarrs-fakesubs.exe' : 'holyarrs-fakesubs');
const port = process.env.HOLYARRS_E2E_PORT ?? '18739';

rmSync(tmp, { recursive: true, force: true });
mkdirSync(config, { recursive: true });

const goEnv = { ...process.env, CGO_ENABLED: '0' };
const version = spawnSync('go', ['version'], { cwd: repo, stdio: 'inherit', env: goEnv });
if (version.status !== 0) process.exit(version.status ?? 1);
const build = spawnSync('go', ['build', '-o', bin, './app/backend/cmd/holyarrs'], { cwd: repo, stdio: 'inherit', env: goEnv });
if (build.status !== 0) process.exit(build.status ?? 1);
const subsBuild = spawnSync('go', ['build', '-o', subsBin, './app/backend/tools/fakesubs'], { cwd: repo, stdio: 'inherit', env: goEnv });
if (subsBuild.status !== 0) process.exit(subsBuild.status ?? 1);

const stopFakes = startFakes();
const subs = spawn(subsBin, ['-addr', `127.0.0.1:${subsPort}`], { cwd: tmp, stdio: 'inherit' });
const child = spawn(bin, ['-config', config, '-addr', `127.0.0.1:${port}`], {
	cwd: tmp,
	stdio: 'inherit',
	env: {
		...process.env,
		...metadataEnv,
		HOLYARRS_RELEASE_FEED: `http://127.0.0.1:${tmdbPort}/releases/latest`,
		HOLYARRS_TRAKT_URL: `http://127.0.0.1:${traktPort}`,
		HOLYARRS_TVMAZE_URL: `http://127.0.0.1:${tvmazePort}`,
		HOLYARRS_MUSICBRAINZ_URL: `http://127.0.0.1:${mbPort}`,
		HOLYARRS_DEEZER_URL: deezerURL
	}
});

let stopped = false;
const stop = () => {
	if (stopped) return;
	stopped = true;
	child.kill();
	subs.kill();
	stopFakes();
};
process.on('SIGINT', stop);
process.on('SIGTERM', stop);
process.on('exit', stop);
child.on('exit', (code) => process.exit(code ?? 0));
