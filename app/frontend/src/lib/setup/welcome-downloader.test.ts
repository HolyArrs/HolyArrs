import { describe, expect, it, vi, beforeEach } from 'vitest';

const posts: { path: string; body: any }[] = [];
vi.mock('#lib/core/api.ts', async (orig) => ({
	...(await orig<any>()),
	api: {
		get: vi.fn(async () => ({ downloaders: [] })),
		post: vi.fn(async (path: string, body?: any) => {
			posts.push({ path, body });
			if (path === '/downloaders') return { id: 1, name: 'qBittorrent', kind: 'qbittorrent', url: body.url, username: body.username };
			return { ok: true, text: 'v5.2' };
		}),
		put: vi.fn(),
		del: vi.fn(),
		onSignedOut: vi.fn()
	}
}));

import { createWelcomeDownloader } from './welcome-downloader.svelte.ts';

const submit = { preventDefault() {} } as SubmitEvent;

describe('welcome qBittorrent sign-in way', () => {
	beforeEach(() => (posts.length = 0));

	it('asks for the key first and never sends a user name left from the other way', async () => {
		const ctx: any = { error: '', busy: false };
		const s = createWelcomeDownloader(ctx);
		expect(s.qbitWay).toBe('key');
		expect(s.dlUser).toBe(false);
		s.pickWay('login');
		expect(s.dlUser).toBe(true);
		s.dl.username = 'admin';
		s.dl.secret = 'pw';
		s.pickWay('key');
		expect(s.dl.username).toBe('');
		expect(s.dl.secret).toBe('');
		s.dl.username = 'leftover';
		s.dl.url = '10.0.0.2:8080';
		s.dl.secret = 'k';
		await s.saveDownloader(submit);
		expect(posts[0].body.username).toBe('');
		expect(s.qbitWay).toBe('key');
		expect(s.dl.secret).toBe('');
	});

	it('editing a Downloader saved with a user name opens on user name and password; Cancel goes back to key', () => {
		const s = createWelcomeDownloader({ error: '', busy: false });
		s.editDl({ id: 2, name: 'q', kind: 'qbittorrent', url: 'http://q:8080', username: 'admin', path_map: [], tags: [] } as any);
		expect(s.qbitWay).toBe('login');
		s.resetForm();
		expect(s.qbitWay).toBe('key');
		expect(s.dlEditing).toBeNull();
	});
});
