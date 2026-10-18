import type { IncomingMessage } from 'node:http';
import { existsSync, readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { defineConfig, mergeConfig } from 'vite';
import appConfig from './vite.config.ts';

const target = 'http://127.0.0.1:18728';
const catalogPath = resolve('../../docs/design-review/intercom/sample-catalog.json');
const activityPath = resolve('../../docs/design-review/intercom/sample-real-activity.json');
const cookieNames = [
	{ backend: 'holyarrs_session', preview: 'holyarrs_apple_preview_session' },
	{ backend: 'holyarrs_oidc', preview: 'holyarrs_apple_preview_oidc' }
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
type ActivityTitle = CatalogTitle & { id?: number; albums?: { name?: string; artwork?: string }[] };

function artworkByTitle() {
	const out = new Map<string, string>();
	for (const path of [catalogPath, activityPath]) {
		if (!existsSync(path)) continue;
		const catalog = JSON.parse(readFileSync(path, 'utf8')) as {
			movies?: CatalogTitle[];
			shows?: CatalogTitle[];
			artists?: CatalogTitle[];
		};
		for (const [kind, entries] of [['movie', catalog.movies], ['show', catalog.shows], ['artist', catalog.artists]] as const) {
			for (const entry of entries ?? []) {
				const name = entry.name ?? entry.title;
				if (name && entry.artwork?.startsWith('https://')) out.set(`${kind}\u0000${name.toLocaleLowerCase()}`, entry.artwork);
			}
		}
	}
	return out;
}

function activityArtwork() {
	const albums = new Map<string, string>();
	const byID = new Map<number, string>();
	if (!existsSync(activityPath)) return { albums, byID };
	const activity = JSON.parse(readFileSync(activityPath, 'utf8')) as { shows?: ActivityTitle[]; artists?: ActivityTitle[] };
	for (const entry of [...(activity.shows ?? []), ...(activity.artists ?? [])]) {
		if (entry.id && entry.artwork?.startsWith('https://')) byID.set(entry.id, entry.artwork);
		for (const album of entry.albums ?? []) {
			if (entry.name && album.name && album.artwork?.startsWith('https://')) {
				albums.set(`${entry.name.toLocaleLowerCase()}\u0000${album.name.toLocaleLowerCase()}`, album.artwork);
			}
		}
	}
	return { albums, byID };
}

function sampleArtwork() {
	return {
		name: 'intercom-sample-artwork',
		configureServer(server: { middlewares: { use: (handler: (...args: any[]) => void) => void } }) {
			server.middlewares.use(async (req: IncomingMessage, res: any, next: (error?: unknown) => void) => {
				if (req.method !== 'GET') return next();
				const pictures = artworkByTitle();
				if (pictures.size === 0) return next();
				const url = new URL(req.url ?? '/', target);
				const titlePath = url.pathname === '/api/v1/titles' || /^\/api\/v1\/titles\/\d+$/.test(url.pathname);
				const askPath = url.pathname === '/api/v1/asks' || /^\/api\/v1\/asks\/\d+$/.test(url.pathname);
				const downloadPath = url.pathname === '/api/v1/downloads/view';
				const decisionPath = url.pathname === '/api/v1/needs-decision';
				const eventPath = url.pathname === '/api/v1/events';
				if (!titlePath && !askPath && !downloadPath && !decisionPath && !eventPath) return next();
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
					const activity = activityArtwork();
					const enrich = (title: any) => {
						const picture = pictures.get(`${title?.kind}\u0000${String(title?.name ?? '').toLocaleLowerCase()}`);
						if (picture) {
							if (askPath) title.poster = picture;
							else title.artwork = [picture];
						}
					};
					const enrichActivity = (item: any) => {
						const album = activity.albums.get(`${String(item?.title ?? '').toLocaleLowerCase()}\u0000${String(item?.album ?? '').toLocaleLowerCase()}`);
						const picture = album ?? pictures.get(`${item?.kind}\u0000${String(item?.title ?? '').toLocaleLowerCase()}`);
						if (picture) item.artwork = picture;
					};
					if (Array.isArray(data.titles)) data.titles.forEach(enrich);
					if (Array.isArray(data.asks)) data.asks.forEach(enrich);
					if (Array.isArray(data.downloads)) data.downloads.forEach(enrichActivity);
					if (Array.isArray(data.items)) data.items.forEach(enrichActivity);
					if (Array.isArray(data.events)) {
						for (const event of data.events) {
							const picture = activity.byID.get(Number(event?.TitleID));
							if (picture) event.artwork = picture;
						}
					}
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
	plugins: [],
	server: {
		host: '0.0.0.0',
		port: 18749,
		strictPort: true,
		allowedHosts: ['holyarrs-app.luiverse.com'],
		fs: { allow: ['.', '../../../mobbin-product-review/web/app/node_modules/@fontsource-variable'], deny: ['.env', '.env.*', '*.{crt,pem,key}', '**/.git/**'] },
		proxy: {
			'/api': isolatedProxy(),
			'/live': isolatedProxy(),
			'/healthz': isolatedProxy()
		}
	}
}));
