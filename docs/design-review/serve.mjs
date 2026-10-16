import http from 'node:http';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

const root = new URL('./', import.meta.url);
const allowed = new Map([
	['/', ['index.html', 'text/html; charset=utf-8']],
	['/concepts/one.html', ['concepts/one.html', 'text/html; charset=utf-8']],
	['/concepts/two.html', ['concepts/two.html', 'text/html; charset=utf-8']],
	['/concepts/three.html', ['concepts/three.html', 'text/html; charset=utf-8']],
	['/assets/artwork.png', ['assets/artwork.png', 'image/png']],
	['/assets/mark.svg', ['assets/mark.svg', 'image/svg+xml']],
	['/assets/geist.woff2', ['assets/geist.woff2', 'font/woff2']],
	['/assets/sora.woff2', ['assets/sora.woff2', 'font/woff2']]
]);
const server = http.createServer(async (req, res) => {
	const path = new URL(req.url, 'http://preview.invalid').pathname;
	const asset = allowed.get(path);
	if (req.method !== 'GET' || !asset) {
		res.writeHead(404).end();
		return;
	}
	try {
		const bytes = await readFile(fileURLToPath(new URL(asset[0], root)));
		res.writeHead(200, { 'Content-Type': asset[1], 'Cache-Control': 'no-store' });
		res.end(bytes);
	} catch {
		res.writeHead(500).end('Review asset unavailable');
	}
});
server.listen(18744, '0.0.0.0', () => process.stdout.write('Direction preview listening on 18744\n'));
