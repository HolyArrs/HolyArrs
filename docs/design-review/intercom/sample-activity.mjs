// Populates the disposable Intercom preview with real request and download
// records through HolyArrs' public API. No database is opened or altered.
import { existsSync, readdirSync, renameSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { readSampleCatalog } from './sample-preview.mjs';

const backend = 'http://127.0.0.1:18747';
const qbit = 'http://127.0.0.1:28768';
const wanted = {
	movies: ['The Odyssey', 'Toy Story 5', 'Spider-Man: Brand New Day', 'Insidious: Out of the Further', 'Resident Evil', 'The End of Oak Street', 'Coyote vs. Acme', 'Avatar Aang: The Last Airbender'],
	shows: ['Person of Interest', 'True Detective', 'Supernatural', 'American Horror Story', 'Fargo', 'Chicago P.D.'],
	artists: ['Lil Uzi Vert', 'Bailey Zimmerman', 'Plies', 'twenty one pilots', 'Midland', 'Billie Eilish']
};

async function call(path, init = {}, expected = [200]) {
	const response = await fetch(`${backend}${path}`, init);
	const text = await response.text();
	let data = {};
	try { data = text ? JSON.parse(text) : {}; } catch { data = { text }; }
	if (!expected.includes(response.status)) throw new Error(`${init.method ?? 'GET'} ${path}: ${response.status} ${data.error ?? text}`.trim());
	return data;
}

function assertDisposableFolder(path) {
	const absolute = resolve(path);
	const temp = resolve(tmpdir());
	const parent = dirname(absolute);
	const rootName = basename(parent).toLowerCase();
	const allowedRoot = rootName === 'holyarrs-intercom-preview-18747' || rootName === 'holyarrs-intercom-preview-media-18747';
	if (dirname(parent).toLowerCase() !== temp.toLowerCase() || !allowedRoot || !['movies', 'shows', 'music'].includes(basename(absolute).toLowerCase())) {
		throw new Error(`Refusing non-preview library folder: ${absolute}`);
	}
	if (readdirSync(absolute).length !== 0) throw new Error(`Preview fixture folder must be empty before request staging: ${absolute}`);
	return absolute;
}

const sleep = (ms) => new Promise((resolveWait) => setTimeout(resolveWait, ms));

async function waitForDownloads(count, timeoutMs = 90_000) {
	const deadline = Date.now() + timeoutMs;
	while (Date.now() < deadline) {
		const rows = (await call('/api/v1/downloads/view?limit=200')).downloads ?? [];
		if (rows.length >= count) return rows;
		await sleep(500);
	}
	throw new Error(`Timed out waiting for ${count} downloads.`);
}

export async function seedSampleActivity() {
	const me = await call('/api/v1/auth/me');
	if (me.role !== 'owner') throw new Error('Disposable preview owner access is unavailable.');
	const headers = { 'Content-Type': 'application/json' };
	if (me.csrf) headers['X-CSRF-Token'] = me.csrf;
	const post = (path, body = {}, expected = [200]) => call(path, { method: 'POST', headers, body: JSON.stringify(body) }, expected);
	const put = (path, body = {}, expected = [200]) => call(path, { method: 'PUT', headers, body: JSON.stringify(body) }, expected);
	const sources = (await call('/api/v1/sources')).sources ?? [];
	const downloaders = (await call('/api/v1/downloaders')).downloaders ?? [];
	if (!sources.some((x) => x.name === 'Lab fake indexer' && x.url === 'http://127.0.0.1:28767')) {
		throw new Error('The disposable Lab fake indexer is not configured on port 28767.');
	}
	if (!downloaders.some((x) => x.name === 'Lab qBittorrent' && x.url === qbit)) {
		throw new Error('The disposable Lab qBittorrent is not configured on port 28768.');
	}

	const catalog = readSampleCatalog();
	const selected = [
		...wanted.movies.map((name) => ({ kind: 'movie', item: catalog.movies.find((x) => x.title === name) })),
		...wanted.shows.map((name) => ({ kind: 'show', item: catalog.shows.find((x) => x.name === name) })),
		...wanted.artists.map((name) => ({ kind: 'artist', item: catalog.artists.find((x) => x.name === name) }))
	];
	if (selected.some((x) => !x.item)) throw new Error('The sample activity selection no longer matches sample-catalog.json.');
	const folders = (await call('/api/v1/library-folders')).library_folders ?? [];
	const fixtureFolders = folders.map((folder) => ({ ...folder, path: assertDisposableFolder(folder.path) }));
	for (const kind of ['movie', 'show', 'music']) {
		if (!fixtureFolders.some((folder) => folder.kind === kind)) throw new Error(`The disposable preview has no ${kind} library folder.`);
	}
	for (const folder of fixtureFolders) {
		if (existsSync(`${folder.path}.activity-hold`)) throw new Error(`A previous request fixture hold still exists: ${folder.path}.activity-hold`);
	}
	const askSettings = await call('/api/v1/settings/asks');
	if (askSettings.search_on_approve !== false) await put('/api/v1/settings/asks', { ...askSettings, search_on_approve: false });

	let asks = (await call('/api/v1/asks?limit=200')).asks ?? [];
	const existingNames = new Set(asks.map((x) => x.name));
	const newSelections = selected.filter((x) => !existingNames.has(x.kind === 'movie' ? x.item.title : x.item.name));

	if (newSelections.length) {
		const titles = (await call('/api/v1/titles?limit=500')).titles ?? [];
		for (const selection of newSelections) {
			const name = selection.kind === 'movie' ? selection.item.title : selection.item.name;
			const title = titles.find((x) => x.kind === selection.kind && x.name === name);
			if (title) await post(`/api/v1/titles/${title.id}/delete`, { delete_files: false, never_add: false, apply: true });
		}

		const hidden = [];
		try {
			for (const folder of fixtureFolders) {
				const path = folder.path;
				const held = `${path}.activity-hold`;
				renameSync(path, held);
				hidden.push([held, path]);
			}
			for (const selection of newSelections) {
				const item = selection.item;
				const body = selection.kind === 'movie' ? { id: `tmdb:${item.id}` } :
					selection.kind === 'show' ? { kind: 'show', id: `tmdb:${item.tmdb}`, seasons: [] } :
					{ kind: 'artist', id: `mbid:${item.id}`, albums: [] };
				await post('/api/v1/asks', body, [200, 201]);
			}
		} finally {
			for (const [held, path] of hidden.reverse()) renameSync(held, path);
		}
	}

	asks = (await call('/api/v1/asks?limit=200')).asks ?? [];
	const selectedNames = new Set(selected.map((x) => x.kind === 'movie' ? x.item.title : x.item.name));
	const orderedNames = selected.map((x) => x.kind === 'movie' ? x.item.title : x.item.name);
	const pendingNames = new Set(orderedNames.slice(0, 6));
	const turnedDownNames = new Set(orderedNames.slice(6, 12));
	const approvedNames = new Set(orderedNames.slice(12));
	const open = asks.filter((x) => selectedNames.has(x.name) && x.state === 'asked');
	for (const ask of open.filter((x) => approvedNames.has(x.name))) await post(`/api/v1/asks/${ask.id}/approve`, { note: 'Approved for the preview household.' });
	for (const ask of open.filter((x) => turnedDownNames.has(x.name))) await post(`/api/v1/asks/${ask.id}/turn-down`, { note: 'Not planned for the household library right now.' });
	if (open.some((x) => !pendingNames.has(x.name) && !turnedDownNames.has(x.name) && !approvedNames.has(x.name))) {
		throw new Error('An unrecognized preview request remained open.');
	}

	const allTitles = (await call('/api/v1/titles?limit=500')).titles ?? [];
	const requestedNames = new Set(selectedNames);
	const downloadTitles = allTitles.filter((x) => x.kind === 'movie' && !requestedNames.has(x.name)).slice(0, 10);
	if (downloadTitles.length < 10) throw new Error(`Need 10 untouched movie titles for download samples; found ${downloadTitles.length}.`);
	let rows = (await call('/api/v1/downloads/view?limit=200')).downloads ?? [];
	if (rows.length < 10) {
		const existingDownloadTitles = new Set(rows.map((x) => x.title));
		for (const title of downloadTitles) {
			if (!existingDownloadTitles.has(title.name)) await post(`/api/v1/titles/${title.id}/search`, { choose: true }, [202]);
		}
	}
	rows = await waitForDownloads(10);
	const desiredPhases = ['downloading', 'failed', 'paused', 'waiting'];
	if (!desiredPhases.every((phase) => rows.some((row) => row.phase === phase))) {
		const active = rows.filter((x) => x.active && x.kind === 'movie').slice(0, 10);
		if (active.length < 10) throw new Error(`Expected 10 active preview downloads before state assignment; found ${active.length}.`);
		const rawDownloads = (await call('/api/v1/downloads?limit=200')).downloads ?? [];
		const externalByID = new Map(rawDownloads.map((x) => [x.ID, x.ExternalID]));
		const states = [
			['downloading', 0.12, 740], ['downloading', 0.31, 520], ['downloading', 0.63, 260],
			['pausedDL', 0.44, 0], ['pausedDL', 0.72, 0], ['queuedDL', 0.05, 1200], ['queuedDL', 0.18, 900],
			['error', 0.27, 0], ['error', 0.56, 0], ['downloading', 0.49, 390]
		];
		const hashes = active.map((row) => externalByID.get(row.id));
		if (hashes.some((hash) => !hash)) throw new Error('A preview download is missing its fake downloader hash.');
		for (let i = 0; i < hashes.length; i++) {
			const external = hashes[i];
			const [state, progress, eta] = states[i];
			const response = await fetch(`${qbit}/set?hash=${encodeURIComponent(external)}&state=${state}&progress=${progress}&eta=${eta}`);
			if (!response.ok) throw new Error(`Fake downloader refused state update for ${external}.`);
		}
		await sleep(2500);
	}

	asks = (await call('/api/v1/asks?limit=200')).asks ?? [];
	rows = (await call('/api/v1/downloads/view?limit=200')).downloads ?? [];
	const countBy = (items, key) => Object.fromEntries([...new Set(items.map((x) => x[key]))].sort().map((value) => [value, items.filter((x) => x[key] === value).length]));
	return { requests: { total: asks.length, states: countBy(asks, 'state') }, downloads: { total: rows.length, phases: countBy(rows, 'phase') } };
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
	process.stdout.write(`${JSON.stringify(await seedSampleActivity())}\n`);
}
