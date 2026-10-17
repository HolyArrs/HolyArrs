import type { IncomingMessage } from 'node:http';
import { existsSync, readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { defineConfig, mergeConfig } from 'vite';
import appConfig from './vite.config.ts';

const target = 'http://127.0.0.1:18747';
const catalogPath = resolve('../../docs/design-review/intercom/sample-catalog.json');
const cookieNames = [
	{ backend: 'holyarrs_session', preview: 'holyarrs_intercom_preview_session' },
	{ backend: 'holyarrs_oidc', preview: 'holyarrs_intercom_preview_oidc' }
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

type CatalogTitle = { name?: string; title?: string; artwork?: string };

function artworkByTitle() {
	if (!existsSync(catalogPath)) return new Map<string, string>();
	const catalog = JSON.parse(readFileSync(catalogPath, 'utf8')) as {
		movies?: CatalogTitle[];
		shows?: CatalogTitle[];
		artists?: CatalogTitle[];
	};
	const out = new Map<string, string>();
	for (const [kind, entries] of [['movie', catalog.movies], ['show', catalog.shows], ['artist', catalog.artists]] as const) {
		for (const entry of entries ?? []) {
			const name = entry.name ?? entry.title;
			if (name && entry.artwork?.startsWith('https://')) out.set(`${kind}\u0000${name.toLocaleLowerCase()}`, entry.artwork);
		}
	}
	return out;
}

function sampleArtwork() {
	const pictures = artworkByTitle();
	return {
		name: 'intercom-sample-artwork',
		configureServer(server: { middlewares: { use: (handler: (...args: any[]) => void) => void } }) {
			server.middlewares.use(async (req: IncomingMessage, res: any, next: (error?: unknown) => void) => {
				if (req.method !== 'GET' || pictures.size === 0) return next();
				const url = new URL(req.url ?? '/', target);
				const titlePath = url.pathname === '/api/v1/titles' || /^\/api\/v1\/titles\/\d+$/.test(url.pathname);
				const askPath = url.pathname === '/api/v1/asks' || /^\/api\/v1\/asks\/\d+$/.test(url.pathname);
				if (!titlePath && !askPath) return next();
				try {
					const headers = new Headers();
					for (const [name, value] of Object.entries(req.headers)) {
						if (!value || ['host', 'content-length', 'accept-encoding', 'cookie'].includes(name)) continue;
						headers.set(name, Array.isArray(value) ? value.join(', ') : value);
					}
					const cookie = requestCookies(typeof req.headers.cookie === 'string' ? req.headers.cookie : undefined);
					if (cookie) headers.set('cookie', cookie);
					const response = await fetch(`${target}${url.pathname}${url.search}`, { headers });
					const data = await response.json();
					const enrich = (title: any) => {
						const picture = pictures.get(`${title?.kind}\u0000${String(title?.name ?? '').toLocaleLowerCase()}`);
						if (picture) {
							if (askPath) title.poster = picture;
							else title.artwork = [picture];
						}
					};
					if (Array.isArray(data.titles)) data.titles.forEach(enrich);
					if (Array.isArray(data.asks)) data.asks.forEach(enrich);
					if (data.ask) enrich(data.ask);
					if (data.title) enrich(data.title);
					else if (data.kind && data.name) enrich(data);
					res.statusCode = response.status;
					res.setHeader('Content-Type', 'application/json; charset=utf-8');
					res.setHeader('Cache-Control', 'no-store');
					const cookies = responseCookies(response.headers.getSetCookie?.());
					if (cookies) res.setHeader('Set-Cookie', cookies);
					res.end(JSON.stringify(data));
				} catch (error) {
					next(error);
				}
			});
		}
	};
}

export default mergeConfig(appConfig, defineConfig({
	plugins: [sampleArtwork()],
	server: {
		host: '0.0.0.0',
		port: 18746,
		strictPort: true,
		proxy: {
			'/api': isolatedProxy(),
			'/live': isolatedProxy(),
			'/healthz': isolatedProxy()
		}
	}
}));
