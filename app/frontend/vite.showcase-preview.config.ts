import { existsSync, readFileSync } from 'node:fs';
import type { IncomingMessage, ServerResponse } from 'node:http';
import { extname, join, normalize, resolve, sep } from 'node:path';
import { brotliCompressSync, constants } from 'node:zlib';
import { defineConfig, type Plugin } from 'vite';
// Where vite.showcase-build.config.ts writes the built app.
const showcaseSite = '.svelte-kit/showcase/site';

// The built app on 18729: `vite preview -c vite.showcase-preview.config.ts`
// serves what `vite build --watch -c vite.showcase-build.config.ts` writes,
// with the same /api proxy and allowed hosts as vite.showcase.config.ts, so
// sign-in works the same. Hashed files are kept by browsers for a year and
// sent brotli-compressed; everything else is checked again each time and
// gzipped by vite.
const target = 'http://127.0.0.1:18728';
const cookieNames = [
	{ backend: 'holyarrs_session', preview: 'holyarrs_showcase_session' },
	{ backend: 'holyarrs_oidc', preview: 'holyarrs_showcase_oidc' }
] as const;

function requestCookies(value: string | undefined): string | undefined {
	if (!value) return undefined;
	const parts = value.split(';').map((part) => part.trim()).filter(Boolean);
	const withoutBackend = parts.filter((part) => !cookieNames.some(({ backend }) => part.startsWith(`${backend}=`)));
	const remapped = withoutBackend.map((part) => {
		const pair = cookieNames.find(({ preview }) => part.startsWith(`${preview}=`));
		return pair ? `${pair.backend}=${part.slice(pair.preview.length + 1)}` : part;
	});
	return remapped.length ? remapped.join('; ') : undefined;
}

function responseCookies(values: string[] | undefined): string[] | undefined {
	return values?.map((value) => {
		const pair = cookieNames.find(({ backend }) => value.startsWith(`${backend}=`));
		return pair ? `${pair.preview}=${value.slice(pair.backend.length + 1)}` : value;
	});
}

function isolatedProxy() {
	return {
		target,
		configure(proxy: { on: (event: string, listener: (...args: any[]) => void) => void }) {
			proxy.on('proxyReq', (proxyReq: { setHeader: (name: string, value: string) => void; removeHeader: (name: string) => void }, req: IncomingMessage) => {
				const value = requestCookies(typeof req.headers.cookie === 'string' ? req.headers.cookie : undefined);
				if (value) proxyReq.setHeader('cookie', value);
				else proxyReq.removeHeader('cookie');
			});
			proxy.on('proxyRes', (proxyRes: IncomingMessage) => {
				const values = responseCookies(proxyRes.headers['set-cookie']);
				if (values) proxyRes.headers['set-cookie'] = values;
			});
		}
	};
}

const site = resolve(showcaseSite);
const types: Record<string, string> = {
	'.js': 'text/javascript; charset=utf-8',
	'.css': 'text/css; charset=utf-8',
	'.json': 'application/json; charset=utf-8',
	'.svg': 'image/svg+xml',
	'.woff2': 'font/woff2'
};
// Hashed names never change content, so each is compressed once.
const brotli = new Map<string, Buffer>();

/** builtFile is the file a request path names inside the built site, or ''. */
function builtFile(pathname: string): string {
	let p = pathname;
	try {
		p = decodeURIComponent(pathname);
	} catch {
		return '';
	}
	const file = normalize(join(site, p));
	return file.startsWith(site + sep) && existsSync(file) ? file : '';
}

// The Go server fills the sign-in seed mark in its own index.html; take the
// block from the backend's /login so a first visit paints the mosaic here too.
const seedMark = '<!--holyarrs-signin-seed-->';
async function loginPage(res: ServerResponse) {
	const block = await fetch(`${target}/login`, { signal: AbortSignal.timeout(800) })
		.then((r) => r.text())
		.then((html) => html.match(/<script type="application\/json" id="holyarrs-signin-inline">[^<]*<\/script>/)?.[0] ?? '')
		.catch(() => '');
	const page = readFileSync(join(site, 'index.html'), 'utf8').replace(seedMark, block);
	res.setHeader('Content-Type', 'text/html; charset=utf-8');
	res.end(page);
}

function builtSite(): Plugin {
	return {
		name: 'holyarrs-showcase-built-site',
		configurePreviewServer(server) {
			server.middlewares.use((req, res, next) => {
				if (req.method !== 'GET' && req.method !== 'HEAD') return next();
				const url = new URL(req.url ?? '/', 'http://x');
				if (/^\/(api|live|healthz|oauth|\.well-known)(\/|$)|^\/mcp$/.test(url.pathname)) return next();
				if (!url.pathname.startsWith('/_app/immutable/')) {
					res.setHeader('Cache-Control', 'no-cache');
					if (req.method === 'GET' && /^\/login\/?$/.test(url.pathname) && existsSync(join(site, 'index.html'))) {
						loginPage(res).catch(() => next());
						return;
					}
					return next();
				}
				const type = types[extname(url.pathname)];
				const file = builtFile(url.pathname);
				// Kept for a year only when the file is there: a request that lands while a rebuild has emptied the
				// folder must not be remembered, or that browser keeps the failed answer and the page breaks for good.
				res.setHeader('Cache-Control', file ? 'public, max-age=31536000, immutable' : 'no-store');
				if (!file || !type || type === 'font/woff2' || !/\bbr\b/.test(String(req.headers['accept-encoding'] ?? ''))) return next();
				let body = brotli.get(file);
				if (!body) {
					const raw = readFileSync(file);
					body = brotliCompressSync(raw, { params: { [constants.BROTLI_PARAM_QUALITY]: 11, [constants.BROTLI_PARAM_SIZE_HINT]: raw.length } });
					if (brotli.size > 2000) brotli.clear();
					brotli.set(file, body);
				}
				res.setHeader('Content-Type', type);
				res.setHeader('Content-Encoding', 'br');
				res.setHeader('Vary', 'Accept-Encoding');
				res.setHeader('Content-Length', body.length);
				res.end(req.method === 'HEAD' ? undefined : body);
			});
		}
	};
}

export default defineConfig({
	appType: 'spa',
	plugins: [builtSite()],
	build: { outDir: showcaseSite },
	preview: {
		host: '0.0.0.0',
		port: 18729,
		strictPort: true,
		// Extra host names come from HOLYARRS_PREVIEW_HOSTS (comma-separated).
		allowedHosts: ['localhost', ...(process.env.HOLYARRS_PREVIEW_HOSTS ?? '').split(',').filter(Boolean)],
		// AI app sign-in and the short AI address go to the server too (it serves them in the built app).
		proxy: { '/api': isolatedProxy(), '/live': isolatedProxy(), '/healthz': isolatedProxy(), '/oauth': isolatedProxy(), '/.well-known': isolatedProxy(), '^/mcp$': isolatedProxy() }
	}
});
