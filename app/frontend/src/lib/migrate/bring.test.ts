import { describe, expect, it, vi, beforeEach } from 'vitest';

const posts: { path: string; body: any }[] = [];
vi.mock('#lib/core/api.ts', async (orig) => ({
	...(await orig<any>()),
	api: {
		post: vi.fn(async (path: string, body: any) => {
			posts.push({ path, body });
			return { downloaders: [{ id: 3, app: 'radarr', name: 'SABnzbd', kind: 'sabnzbd', needs: 'key' }, { id: 0, app: 'radarr', name: 'x', kind: 'qbittorrent' }] };
		}),
		onSignedOut: vi.fn()
	}
}));

import { bringDownloaders, readSnapshots } from './bring.ts';

const snap = (id: number, kind: string) => ({ id, kind }) as any;

describe('bring Downloaders after a read', () => {
	beforeEach(() => (posts.length = 0));

	it('brings them for Sonarr, Radarr and Lidarr', async () => {
		const got = await bringDownloaders({ snapshot: snap(1, 'sonarr') }, 'sonarr');
		expect(posts[0].body.snapshots).toEqual([1]);
		expect(got.map((b) => b.id)).toEqual([3]);
	});

	it('brings the ones of the Sonarr and Radarr Overseerr keeps', async () => {
		const r = { snapshot: snap(5, 'overseerr'), linked: [{ snapshot: snap(6, 'radarr') }] };
		expect(readSnapshots(r).map((s) => s.id)).toEqual([5, 6]);
		await bringDownloaders(r, 'overseerr');
		expect(posts[0].body.snapshots).toEqual([6]);
	});

	it('asks nothing for an app that keeps no Downloaders', async () => {
		expect(await bringDownloaders({ snapshot: snap(7, 'prowlarr') }, 'prowlarr')).toEqual([]);
		expect(posts).toEqual([]);
	});
});
