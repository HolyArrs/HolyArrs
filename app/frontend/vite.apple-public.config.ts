// Production preview for the Cloudflare hostname: serves the built app (no dev server, no source files).
import { defineConfig, mergeConfig } from 'vite';
import preview from './vite.apple-preview.config.ts';

const target = 'http://127.0.0.1:18728';
export default mergeConfig(preview, defineConfig({
	preview: {
		host: '0.0.0.0',
		port: 18727,
		strictPort: true,
		allowedHosts: ['holyarrs-app.luiverse.com'],
		proxy: {
			'/api': { target, changeOrigin: false },
			'/live': { target, changeOrigin: false, ws: true },
			'/healthz': { target, changeOrigin: false }
		}
	}
}));
