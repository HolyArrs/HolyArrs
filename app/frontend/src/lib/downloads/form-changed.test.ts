import { formChanged, formFrom, testBody, type DownloaderView } from './downloaders.ts';
import { describe, expect, it } from 'vitest';

// The Test button inside the Change form tests what is typed there, without
// saving it.
const saved: DownloaderView = {
	id: 3, name: 'qBittorrent', kind: 'qbittorrent', protocol: 'torrent', url: 'http://qbittorrent:18080',
	username: '', secret: '••••••••', category: 'holyarrs', priority: 1, path_map: [],
	allow_local_network: false, writable: true, writable_locked: false,
	health: { state: 'resting', label: 'Resting', text: '' }, active_downloads: 0
};

describe('formChanged', () => {
	const cases: [string, (f: ReturnType<typeof formFrom>) => void, boolean][] = [
		['untouched form', () => {}, false],
		['local network switch turned on', (f) => (f.allow_local_network = true), true],
		['address changed', (f) => (f.url = 'http://172.26.0.10:18080'), true],
		['new key typed', (f) => (f.secret = 'abc'), true],
		['only spaces around the name', (f) => (f.name = ' qBittorrent '), false],
		['writable switch turned off', (f) => (f.writable = false), true]
	];
	for (const [name, edit, want] of cases) {
		it(name, () => {
			const f = formFrom(saved);
			edit(f);
			expect(formChanged(f, saved)).toBe(want);
		});
	}
});

const typed = ['new', 'pass'].join('-');

describe('testBody', () => {
	const cases: [string, (f: ReturnType<typeof formFrom>) => void, Record<string, unknown> | undefined][] = [
		['untouched form tests the saved Downloader', () => {}, undefined],
		['new password typed is sent', (f) => (f.secret = typed), { secret: typed }],
		['password left masked is sent masked', (f) => (f.allow_local_network = true), { secret: '••••••••', allow_local_network: true }],
		['new address is sent', (f) => (f.url = 'http://qbittorrent:8080'), { url: 'http://qbittorrent:8080' }]
	];
	for (const [name, edit, want] of cases) {
		it(name, () => {
			const f = formFrom(saved);
			edit(f);
			const got = testBody(f, saved);
			if (want === undefined) expect(got).toBeUndefined();
			else expect(got).toMatchObject(want);
		});
	}
});
