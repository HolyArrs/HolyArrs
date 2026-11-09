// Copies the pinned hls.js (package.json, exact version) into
// static/vendor, so the build ships it as its own file beside the app.
// The app's single bundle never includes it: TrailerPlayer loads it only
// when pass-through first plays in a browser without native HLS.
import { copyFileSync, mkdirSync, readFileSync, readdirSync, rmSync } from 'node:fs';

const root = new URL('../', import.meta.url);
const { version } = JSON.parse(readFileSync(new URL('node_modules/hls.js/package.json', root), 'utf8'));
const dir = new URL('static/vendor/', root);
mkdirSync(dir, { recursive: true });
for (const old of readdirSync(dir)) if (old.startsWith('hls-')) rmSync(new URL(old, dir));
copyFileSync(new URL('node_modules/hls.js/dist/hls.min.mjs', root), new URL(`hls-${version}.min.js`, dir));
console.log(`vendor: hls.js ${version}`);
