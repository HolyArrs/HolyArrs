import { createServer } from 'node:http';
import { createHash, generateKeyPairSync, randomBytes, sign } from 'node:crypto';
import { embyUser, jellyKey, jellyfinPort, mediaUser } from './ports.mjs';
import { body, send } from './http.mjs';

// The fake single sign-on provider's signing key and its open codes.
const oidcKey = generateKeyPairSync('rsa', { modulusLength: 2048 });
/** @type {Map<string, { user: string, nonce: string | null, challenge: string | null }>} */
const oidcCodes = new Map();
const esc = (/** @type {string} */ s) => s.replace(/[&<>"]/g, (ch) => `&#${ch.charCodeAt(0)};`);

/** Starts the fake Jellyfin (with the fake Emby and the single sign-on provider). */
export function startJellyfinFake() {
	/** @type {unknown[]} */
	const updates = [];
	const jelly = createServer(async (req, res) => {
		// A single sign-on provider: discovery for the Settings Test button, and a
		// sign-in page where the test types a name, then code, token and keys.
		if (req.url?.startsWith('/oidc/')) {
			const base = `http://127.0.0.1:${jellyfinPort}/oidc`;
			const url = new URL(req.url, base);
			if (req.method === 'GET' && url.pathname === '/oidc/.well-known/openid-configuration')
				return send(res, 200, { issuer: base, authorization_endpoint: `${base}/auth`, token_endpoint: `${base}/token`, jwks_uri: `${base}/keys`, id_token_signing_alg_values_supported: ['RS256'] });
			if (req.method === 'GET' && url.pathname === '/oidc/keys') return send(res, 200, { keys: [{ ...oidcKey.publicKey.export({ format: 'jwk' }), kid: 'e2e', alg: 'RS256', use: 'sig' }] });
			if (req.method === 'GET' && url.pathname === '/oidc/auth') {
				const keep = [...url.searchParams].map(([k, v]) => `<input type="hidden" name="${esc(k)}" value="${esc(v)}">`).join('');
				return send(res, 200, `<!doctype html><title>Fake provider</title><form action="${base}/auth/ok">${keep}<label>Username <input name="user"></label><button>Sign in</button></form>`, 'text/html');
			}
			if (req.method === 'GET' && url.pathname === '/oidc/auth/ok') {
				const q = url.searchParams;
				const user = q.get('user')?.trim();
				const to = q.get('redirect_uri');
				if (!user || !to || q.get('client_id') !== 'holyarrs' || q.get('code_challenge_method') !== 'S256') return send(res, 400, 'bad sign-in request', 'text/plain');
				const code = randomBytes(16).toString('hex');
				oidcCodes.set(code, { user, nonce: q.get('nonce'), challenge: q.get('code_challenge') });
				const back = new URL(to);
				back.searchParams.set('code', code);
				back.searchParams.set('state', q.get('state') ?? '');
				res.writeHead(303, { Location: back.toString() });
				return res.end();
			}
			if (req.method === 'POST' && url.pathname === '/oidc/token') {
				const f = new URLSearchParams(await body(req));
				const c = oidcCodes.get(f.get('code') ?? '');
				oidcCodes.delete(f.get('code') ?? '');
				const pkce = createHash('sha256').update(f.get('code_verifier') ?? '').digest('base64url');
				const why = !c ? 'unknown code' : pkce !== c.challenge ? 'code_verifier does not match' : !String(req.headers.authorization ?? '').startsWith('Basic ') ? 'no client credentials' : '';
				if (why || !c) return send(res, 400, { error: 'invalid_grant', error_description: why });
				const now = Math.floor(Date.now() / 1000);
				const part = (/** @type {object} */ o) => Buffer.from(JSON.stringify(o)).toString('base64url');
				const head = `${part({ alg: 'RS256', kid: 'e2e', typ: 'JWT' })}.${part({ iss: base, aud: 'holyarrs', sub: `sub-${c.user}`, name: c.user, nonce: c.nonce, iat: now, exp: now + 300 })}`;
				const jwt = `${head}.${sign('sha256', Buffer.from(head), oidcKey.privateKey).toString('base64url')}`;
				return send(res, 200, { access_token: randomBytes(16).toString('hex'), token_type: 'Bearer', expires_in: 300, id_token: jwt });
			}
			return send(res, 404, { error: 'no route' });
		}
		// Emby, as it often sits at http://host/emby. It wants the X-Emby-Authorization header.
		if (req.url?.startsWith('/emby/')) {
			if (req.method === 'POST' && req.url === '/emby/Users/AuthenticateByName') {
				if (!String(req.headers['x-emby-authorization'] ?? '').startsWith('MediaBrowser ')) return send(res, 400, 'no client header', 'text/plain');
				const b = JSON.parse((await body(req)) || '{}');
				if (b.Username === embyUser.name && b.Pw === embyUser.password)
					return send(res, 200, { User: { Id: 'em-1', Name: 'Gran', Policy: { IsDisabled: false } }, AccessToken: 'etok' });
				return send(res, 401, 'Error processing request.', 'text/plain');
			}
			if (req.method === 'POST' && req.url === '/emby/Sessions/Logout') return send(res, 204, '');
			if (req.method === 'GET' && req.url === '/emby/System/Info/Public') return send(res, 200, { ServerName: 'Fake Emby', Version: '4.8.0', ProductName: 'Emby Server', Id: 'fake-emby' });
			return send(res, 404, { error: 'no route' });
		}
		if (req.method === 'POST' && req.url === '/Users/AuthenticateByName') {
			const b = JSON.parse((await body(req)) || '{}');
			if (b.Username === mediaUser.name && b.Pw === mediaUser.password)
				return send(res, 200, { User: { Id: 'jf-1', Name: 'Mum', Policy: { IsDisabled: false } }, AccessToken: 'jtok' });
			return send(res, 401, 'Error processing request.', 'text/plain');
		}
		if (req.method === 'POST' && req.url === '/Sessions/Logout') return send(res, 204, '');
		// The address test in Settings asks the open server info, as a real Jellyfin answers it.
		if (req.method === 'GET' && req.url === '/System/Info/Public') return send(res, 200, { ServerName: 'Fake Jellyfin', Version: '10.9.0', ProductName: 'Jellyfin Server', Id: 'fake-jellyfin' });
		// Current Jellyfin uses the MediaBrowser Authorization token for server calls.
		const serverCall = ['/System/Info', '/Library/Media/Updated', '/Library/Refresh'].includes(req.url ?? '');
		if (serverCall && (!req.headers.authorization?.startsWith('MediaBrowser ') || !req.headers.authorization.includes(`Token="${jellyKey}"`))) return send(res, 401, '');
		if (req.method === 'GET' && req.url === '/System/Info') return send(res, 200, { ServerName: 'Fake Jellyfin', Version: '10.9.0' });
		if (req.method === 'POST' && req.url === '/Library/Media/Updated') {
			updates.push(JSON.parse((await body(req)) || '{}'));
			return send(res, 204, '');
		}
		if (req.method === 'POST' && req.url === '/Library/Refresh') return send(res, 204, '');
		if (req.method === 'GET' && req.url === '/updates') return send(res, 200, updates);
		send(res, 404, { error: 'no route' });
	});
	jelly.listen(jellyfinPort, '127.0.0.1');
	return jelly;
}
