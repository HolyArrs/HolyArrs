import { createServer } from 'node:http';
import { discordPort } from './ports.mjs';
import { body, send } from './http.mjs';

/** Starts the fake Discord (and the other alert targets it keeps). */
export function startDiscordFake() {
	/** @type {unknown[]} */
	const posts = [];
	/** @type {unknown[]} */
	const hooks = [];
	/** @type {unknown[]} */
	const telegram = [];
	/** @type {unknown[]} */
	const apps = [];
	const discord = createServer(async (req, res) => {
		// Fake ntfy (POST /<topic>), Gotify (POST /message) and Pushover
		// (POST /1/messages.json); GET /apps reads back what each was sent.
		const appPath = (req.url ?? '').split('?')[0];
		if (req.method === 'POST' && (appPath === '/message' || appPath === '/1/messages.json' || /^\/[A-Za-z0-9_-]{1,64}$/.test(appPath)) && appPath !== '/hook') {
			const app = appPath === '/message' ? 'gotify' : appPath === '/1/messages.json' ? 'pushover' : 'ntfy';
			apps.push({
				app,
				path: appPath,
				title: req.headers.title ?? '',
				click: req.headers.click ?? '',
				auth: Boolean(req.headers.authorization || req.headers['x-gotify-key']),
				body: await body(req)
			});
			return send(res, 200, app === 'pushover' ? { status: 1, request: 'e2e' } : { id: apps.length });
		}
		if (req.method === 'GET' && req.url === '/apps') return send(res, 200, apps);
		// A fake Telegram Bot API: POST /bot<token>/sendMessage; GET /telegram reads the bodies back.
		if (req.method === 'POST' && /^\/bot[^/]+\/sendMessage$/.test(req.url ?? '')) {
			telegram.push(JSON.parse((await body(req)) || '{}'));
			return send(res, 200, { ok: true, result: {} });
		}
		if (req.method === 'GET' && req.url === '/telegram') return send(res, 200, telegram);
		if (req.method === 'POST' && req.url?.startsWith('/api/webhooks/')) {
			posts.push(JSON.parse((await body(req)) || '{}'));
			return send(res, 204, '');
		}
		if (req.method === 'GET' && req.url === '/posts') return send(res, 200, posts);
		// A webhook receiver: keeps the body and the HolyArrs headers.
		if (req.method === 'POST' && req.url === '/hook') {
			hooks.push({ body: JSON.parse((await body(req)) || '{}'), event: req.headers['x-holyarrs-event'], signature: req.headers['x-holyarrs-signature'] ?? '' });
			return send(res, 204, '');
		}
		if (req.method === 'GET' && req.url === '/hooks') return send(res, 200, hooks);
		send(res, 404, { error: 'no route' });
	});
	discord.listen(discordPort, '127.0.0.1');
	return discord;
}
