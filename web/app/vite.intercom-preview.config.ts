import type { IncomingMessage } from 'node:http';
import { defineConfig, mergeConfig } from 'vite';
import appConfig from './vite.config.ts';

const target = 'http://127.0.0.1:18747';
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

export default mergeConfig(appConfig, defineConfig({
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
