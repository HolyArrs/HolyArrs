import { defineConfig, mergeConfig } from 'vite';
import appConfig from './vite.config.ts';
// Review frontend uses only this chat's disposable backend, never the installed app.
export default mergeConfig(appConfig, defineConfig({
 server: {
  host: '0.0.0.0', port: 18745, strictPort: true,
  proxy: {
   '/api': { target: 'http://127.0.0.1:18744' },
   '/live': { target: 'http://127.0.0.1:18744' },
   '/healthz': { target: 'http://127.0.0.1:18744' }
  }
 }
}));

