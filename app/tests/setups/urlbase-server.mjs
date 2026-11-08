// A second throwaway HolyArrs for the URL base test: it runs with
// -url-base /holyarrs behind a small reverse proxy that passes /holyarrs/*
// on unchanged (as nginx-proxy-manager, Traefik or Caddy would) and
// answers 404 for anything else. Its own folder (e2e/.tmp-base).
import { spawn, spawnSync } from 'node:child_process';
import { mkdirSync, rmSync } from 'node:fs';
import { createServer, request } from 'node:http';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const repo = resolve(here, '../../..');
// The specs reach this folder through helpers/data-folder.ts; keep both in step.
const tmp = join(here, '..', '.tmp-base');
const config = join(tmp, 'config');
const bin = join(tmp, process.platform === 'win32' ? 'holyarrs.exe' : 'holyarrs');
export const appPort = Number(process.env.HOLYARRS_E2E_BASE_APP_PORT ?? 18734);
export const proxyPort = Number(process.env.HOLYARRS_E2E_BASE_PROXY_PORT ?? 18735);

rmSync(tmp, { recursive: true, force: true });
mkdirSync(config, { recursive: true });
const build = spawnSync('go', ['build', '-o', bin, './app/backend/cmd/holyarrs'], { cwd: repo, stdio: 'inherit', env: { ...process.env, CGO_ENABLED: '0' } });
if (build.status !== 0) process.exit(build.status ?? 1);

const child = spawn(bin, ['-config', config, '-addr', `127.0.0.1:${appPort}`, '-url-base', '/holyarrs'], {
	cwd: tmp,
	stdio: 'inherit',
	// No metadata server here: never the live one from tests.
	env: { ...process.env, HOLYARRS_RELEASE_FEED: 'http://127.0.0.1:1/none', HOLYARRS_DEEZER_URL: 'off', HOLYARRS_METADATA_URL: 'http://127.0.0.1:1', HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL: 'true' }
});

const proxy = createServer((req, res) => {
	if (!(req.url === '/holyarrs' || req.url?.startsWith('/holyarrs/'))) {
		res.writeHead(404, { 'Content-Type': 'text/plain' });
		res.end('not under /holyarrs\n');
		return;
	}
	const up = request({ host: '127.0.0.1', port: appPort, method: req.method, path: req.url, headers: req.headers }, (r) => {
		res.writeHead(r.statusCode ?? 502, r.headers);
		r.pipe(res);
	});
	up.on('error', () => {
		res.writeHead(502);
		res.end();
	});
	req.pipe(up);
});
proxy.listen(proxyPort, '127.0.0.1');

const stop = () => {
	child.kill();
	proxy.close();
};
process.on('SIGINT', stop);
process.on('SIGTERM', stop);
process.on('exit', stop);
child.on('exit', (code) => process.exit(code ?? 0));
