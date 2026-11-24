// Starts the real HolyArrs router with a stub "player" plugin (one page, one
// streamable file) for plugins/plugin-page-stream.spec.ts. Plugins cannot run
// on every system, so the stub lives in a Go test program built from this repo
// (TestServePluginPageForBrowser); the app it serves is the built web app.
import { spawn, spawnSync } from 'node:child_process';
import { mkdirSync, rmSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const repo = resolve(here, '../../..');
const tmp = join(here, '..', '.tmp-plugin-page');
const exe = join(tmp, process.platform === 'win32' ? 'plugin-page.test.exe' : 'plugin-page.test');
const port = process.env.HOLYARRS_E2E_PORT ?? '18796';

rmSync(tmp, { recursive: true, force: true });
mkdirSync(join(tmp, 'movies'), { recursive: true });

const build = spawnSync('go', ['test', '-c', '-o', exe, './app/backend/cmd/holyarrs'], { cwd: repo, stdio: 'inherit', env: { ...process.env, CGO_ENABLED: '0' } });
if (build.status !== 0) process.exit(build.status ?? 1);
const child = spawn(exe, ['-test.run', '^TestServePluginPageForBrowser$', '-test.timeout', '0'], {
	cwd: tmp,
	stdio: 'inherit',
	env: { ...process.env, HOLYARRS_PLUGIN_PAGE_ADDR: `127.0.0.1:${port}` }
});
const stop = () => child.kill();
process.on('SIGINT', stop);
process.on('SIGTERM', stop);
process.on('exit', stop);
child.on('exit', (code) => process.exit(code ?? 0));
