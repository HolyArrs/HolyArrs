import type { IncomingMessage } from 'node:http';
import { defineConfig, mergeConfig } from 'vite';
import appConfig from './vite.config.ts';

// Showcase preview: real titles from the showcase backend on 18728; real artwork comes from the API.
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

// In dev, vite serves app.html itself; the Go server fills the sign-in seed
// mark only in its own index.html. Take that block from the backend's /login
// and put it into vite's page, so a first visit paints the mosaic here too.
const seedMark = '<!--holyarrs-signin-seed-->';
function signinSeed() {
	return {
		name: 'holyarrs-signin-seed',
		configureServer(server: { middlewares: { use: (fn: (req: IncomingMessage, res: any, next: () => void) => void) => void } }) {
			server.middlewares.use((req, res, next) => {
				if (req.method !== 'GET' || !/^\/login\/?(\?|$)/.test(req.url ?? '')) return next();
				fetch(`${target}/login`, { signal: AbortSignal.timeout(800) })
					.then((r) => r.text())
					.then((html) => html.match(/<script type="application\/json" id="holyarrs-signin-inline">[^<]*<\/script>/)?.[0] ?? '')
					.catch(() => '')
					.then((block) => {
						if (block) {
							const end = res.end.bind(res);
							res.end = (chunk: unknown, ...rest: unknown[]) => {
								if (typeof chunk === 'string' && chunk.includes(seedMark)) chunk = chunk.replace(seedMark, block);
								else if (Buffer.isBuffer(chunk) && chunk.includes(seedMark)) chunk = Buffer.from(chunk.toString().replace(seedMark, block));
								if (chunk && res.getHeader?.('content-length')) res.removeHeader('content-length');
								return end(chunk, ...rest);
							};
						}
						next();
					});
			});
		}
	};
}

export default mergeConfig(appConfig, defineConfig({
	plugins: [signinSeed()],
	server: {
		host: '0.0.0.0',
		port: 18729,
		strictPort: true,
		allowedHosts: ['holyarrs-app.luiverse.com'],
		// Type checks and tests rewrite generated files; don't reload the page for them.
		watch: { ignored: ['**/.svelte-kit/generated/**', '**/.svelte-kit/types/**', '**/test-results/**', '**/playwright-report/**', '../dist/**', '**/coverage/**'] },
		fs: { allow: ['.', '../../../mobbin-product-review/web/app/node_modules/@fontsource-variable'], deny: ['.env', '.env.*', '*.{crt,pem,key}', '**/.git/**'] },
		proxy: {
			'/api': isolatedProxy(),
			'/live': isolatedProxy(),
			'/healthz': isolatedProxy()
		}
	}
}));
