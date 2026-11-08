// A throwaway HolyArrs with a fake qBittorrent that answers sync/maindata,
// for the push-updates test (live-push.spec.ts). The app is the built
// showcase site (.svelte-kit/showcase/site, written by the showcase watch
// build), served here with /api and /live sent to this backend; web/dist
// is never touched.
import { spawn, spawnSync } from 'node:child_process';
import { createReadStream, existsSync, mkdirSync, rmSync, statSync } from 'node:fs';
import { createServer, request } from 'node:http';
import { dirname, extname, join, normalize, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
import { metadataEnv, startFakes } from '../fakes/index.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const repo = resolve(here, '../../..');
// The specs reach this folder through helpers/data-folder.ts; keep both in step.
const tmp = join(here, '..', '.tmp-live');
const config = join(tmp, 'config');
const bin = join(tmp, process.platform === 'win32' ? 'holyarrs.exe' : 'holyarrs');
export const backendPort = Number(process.env.HOLYARRS_LIVE_BACKEND_PORT ?? 18732);
export const qbitPort = Number(process.env.HOLYARRS_LIVE_QBIT_PORT ?? 18733);
export const webPort = Number(process.env.HOLYARRS_LIVE_WEB_PORT ?? 18734);
const site = resolve(here, '../../frontend/.svelte-kit/showcase/site');

if (process.argv[1] === fileURLToPath(import.meta.url)) main();

function main() {
	if (!existsSync(join(site, 'index.html'))) {
		console.error(`no built app at ${site}: start the showcase watch build first`);
		process.exit(1);
	}
	rmSync(tmp, { recursive: true, force: true });
	mkdirSync(config, { recursive: true });
	const build = spawnSync('go', ['build', '-o', bin, './app/backend/cmd/holyarrs'], { cwd: repo, stdio: 'inherit', env: { ...process.env, CGO_ENABLED: '0' } });
	if (build.status !== 0) process.exit(build.status ?? 1);

	// Fake qBittorrent: one torrent HolyArrs did not send; /set moves it.
	/** @type {Record<string, string | number>} */
	const t = { hash: 'c'.repeat(40), name: 'Live.Push.Test.ISO', category: 'linux', size: 4_000_000_000, progress: 0.2, dlspeed: 2_097_152, upspeed: 0,
		eta: 900, state: 'downloading', save_path: '/downloads', added_on: 1_700_000_000, ratio: 0, num_seeds: 3, num_leechs: 1 };
	let rid = 0;
	// /set?count=N adds N more torrents (the long-list test); the first 200 of them move on every read.
	/** @type {Record<string, string | number>[]} */
	let bulk = [];
	const all = () => [t, ...bulk];
	const qbit = createServer((req, res) => {
		const url = new URL(req.url ?? '/', 'http://x');
		const json = (/** @type {unknown} */ v) => {
			res.writeHead(200, { 'Content-Type': 'application/json' });
			res.end(JSON.stringify(v));
		};
		const text = (/** @type {string} */ v) => {
			res.writeHead(200, { 'Content-Type': 'text/plain' });
			res.end(v);
		};
		switch (url.pathname) {
			case '/set':
				for (const k of ['progress', 'dlspeed', 'eta']) if (url.searchParams.get(k)) t[k] = Number(url.searchParams.get(k));
				if (url.searchParams.has('count')) {
					const n = Number(url.searchParams.get('count'));
					bulk = Array.from({ length: n }, (_, i) => ({ ...t, hash: (i + 1).toString(16).padStart(40, '0'), name: `Bulk.Row.${String(i).padStart(4, '0')}`, progress: (i % 97) / 100,
						dlspeed: i < 200 ? 100_000 + i : 0, state: i < 200 ? 'downloading' : 'uploading', added_on: 1_700_000_000 + i }));
				}
				return json({ ok: true });
			case '/api/v2/auth/login':
				return text('Ok.');
			case '/api/v2/app/version':
				return text('v5.2.4');
			case '/api/v2/app/webapiVersion':
				return text('2.11.4');
			case '/api/v2/sync/maindata': {
				const from = Number(url.searchParams.get('rid') ?? 0);
				rid++;
				for (const b of bulk.slice(0, 200)) b.progress = Math.min(0.99, Number(b.progress) + 0.001);
				return json({ rid, full_update: from === 0, torrents: Object.fromEntries(all().map((x) => [x.hash, { ...x }])), server_state: { dl_info_speed: t.dlspeed, up_info_speed: 0 } });
			}
			case '/api/v2/torrents/info':
				return json(all());
			case '/api/v2/torrents/files':
			case '/api/v2/torrents/trackers':
				return json([]);
		}
		res.writeHead(200, { 'Content-Type': 'text/plain' });
		res.end('');
	});
	qbit.listen(qbitPort, '127.0.0.1');

	// The usual e2e fakes stand in for the metadata server, so this throwaway install never reaches the real one.
	const stopFakes = startFakes();
	const child = spawn(bin, ['-config', config, '-addr', `127.0.0.1:${backendPort}`], { cwd: tmp, stdio: 'inherit', env: { ...process.env, ...metadataEnv } });

	// The built app, with /api, /live and /healthz sent to the backend.
	/** @type {Record<string, string>} */
	const types = { '.js': 'text/javascript', '.css': 'text/css', '.json': 'application/json', '.svg': 'image/svg+xml', '.woff2': 'font/woff2', '.png': 'image/png', '.html': 'text/html' };
	// Open /live streams through this front, for the shared-stream test (one per browser).
	let liveOpen = 0;
	let liveTotal = 0;
	const web = createServer((req, res) => {
		const url = new URL(req.url ?? '/', 'http://x');
		if (url.pathname === '/__test/live') {
			res.writeHead(200, { 'Content-Type': 'application/json' });
			res.end(JSON.stringify({ open: liveOpen, total: liveTotal }));
			return;
		}
		if (url.pathname === '/live') {
			liveOpen++;
			liveTotal++;
			res.on('close', () => liveOpen--);
		}
		if (/^\/(api|live|healthz)(\/|$)/.test(url.pathname)) {
			const up = request({ host: '127.0.0.1', port: backendPort, path: req.url, method: req.method, headers: req.headers }, (r) => {
				res.writeHead(r.statusCode ?? 502, r.headers);
				r.pipe(res);
			});
			up.on('error', () => {
				res.writeHead(502);
				res.end();
			});
			req.pipe(up);
			return;
		}
		let file = normalize(join(site, decodeURIComponent(url.pathname)));
		if (!file.startsWith(site + sep) || !existsSync(file) || statSync(file).isDirectory()) file = join(site, 'index.html');
		res.writeHead(200, { 'Content-Type': types[extname(file)] ?? 'application/octet-stream', 'Cache-Control': 'no-cache' });
		createReadStream(file).pipe(res);
	});
	web.listen(webPort, '127.0.0.1');

	const stop = () => {
		child.kill();
		stopFakes();
		qbit.close();
		web.close();
	};
	process.on('SIGINT', stop);
	process.on('SIGTERM', stop);
	process.on('exit', stop);
	child.on('exit', (code) => process.exit(code ?? 0));
}
