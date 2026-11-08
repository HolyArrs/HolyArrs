// Starts a throwaway HolyArrs for the browser tests: a fresh config folder
// in e2e/.tmp, the program built from this repo, listening on 127.0.0.1.
import { spawn, spawnSync } from 'node:child_process';
import { mkdirSync, rmSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { deezerURL, mbPort, metadataEnv, plexPort, startFakes, subsPort, tmdbPort, traktPort, tvmazePort } from '../fakes/index.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const repo = resolve(here, '../../..');
// The specs reach this folder through helpers/data-folder.ts; keep both in step.
const tmp = join(here, "..", ".tmp-phone");
const config = join(tmp, 'config');
const movies = join(tmp, 'movies');
const showsDir = join(tmp, 'shows');
const musicDir = join(tmp, 'music');
const bin = join(tmp, process.platform === 'win32' ? 'holyarrs.exe' : 'holyarrs');
const port = process.env.HOLYARRS_E2E_PORT ?? '18797';

rmSync(tmp, { recursive: true, force: true });
mkdirSync(config, { recursive: true });
mkdirSync(movies, { recursive: true });
mkdirSync(showsDir, { recursive: true });
mkdirSync(musicDir, { recursive: true });

const build = spawnSync('go', ['build', '-o', bin, './app/backend/cmd/holyarrs'], { cwd: repo, stdio: 'inherit', env: { ...process.env, CGO_ENABLED: '0' } });
if (build.status !== 0) process.exit(build.status ?? 1);
const subs = { kill() {} };

// Without the fakes: their fixed ports may be busy with the main suite, and layout needs no metadata.
const stopFakes = () => {};
const plex = `http://127.0.0.1:${plexPort}`;
const child = spawn(bin, ['-config', config, '-addr', `127.0.0.1:${port}`], {
	cwd: tmp,
	stdio: 'inherit',
	env: { ...process.env, HOLYARRS_PLEX_URL: plex, HOLYARRS_PLEX_APP_URL: `${plex}/app`, HOLYARRS_PLEX_DISCOVER_URL: `${plex}/discover`, ...metadataEnv, HOLYARRS_RELEASE_FEED: `http://127.0.0.1:${tmdbPort}/releases/latest`, HOLYARRS_TRAKT_URL: `http://127.0.0.1:${traktPort}`, HOLYARRS_TVMAZE_URL: `http://127.0.0.1:${tvmazePort}`, HOLYARRS_MUSICBRAINZ_URL: `http://127.0.0.1:${mbPort}`, HOLYARRS_DEEZER_URL: deezerURL }
});
const stop = () => {
	child.kill();
	subs.kill();
	stopFakes();
};
process.on('SIGINT', stop);
process.on('SIGTERM', stop);
process.on('exit', stop);
child.on('exit', (code) => process.exit(code ?? 0));
