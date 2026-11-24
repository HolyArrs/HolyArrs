import { emptyForm, formBody, formChanged, formFrom, type DownloaderView } from './downloaders.ts';
import { describe, expect, it } from 'vitest';

// Usenet Downloaders remove a job and its files once it is imported, on by
// default as in the older apps; the switch is sent only for Usenet kinds.
const view = (kind: DownloaderView['kind'], clear?: boolean): DownloaderView => ({
	id: 4, name: kind, kind, protocol: kind === 'sabnzbd' || kind === 'nzbget' ? 'usenet' : 'torrent', url: 'http://client:8080',
	username: '', secret: '••••••••', category: 'movies', priority: 1, path_map: [],
	allow_local_network: true, writable: true, writable_locked: false, clear_finished: clear,
	health: { state: 'working', label: 'Working', text: '' }, active_downloads: 0
});

describe('remove after import', () => {
	const cases: [string, DownloaderView['kind'], boolean | undefined, unknown][] = [
		['SABnzbd on', 'sabnzbd', true, true],
		['SABnzbd off', 'sabnzbd', false, false],
		['NZBGet on', 'nzbget', true, true],
		['NZBGet off', 'nzbget', false, false],
		['older server without the field counts as on', 'sabnzbd', undefined, true],
		['qBittorrent never sends it', 'qbittorrent', true, undefined],
		['Transmission never sends it', 'transmission', false, undefined]
	];
	for (const [name, kind, clear, want] of cases) {
		it(name, () => {
			expect(formBody(formFrom(view(kind, clear))).clear_finished).toBe(want);
		});
	}

	it('a new Usenet Downloader starts with it on', () => {
		expect(formBody(emptyForm('sabnzbd')).clear_finished).toBe(true);
		expect(formBody(emptyForm('nzbget')).clear_finished).toBe(true);
	});

	it('turning it off is a change to save', () => {
		const d = view('sabnzbd', true);
		const f = formFrom(d);
		f.clear_finished = false;
		expect(formChanged(f, d)).toBe(true);
	});
});
