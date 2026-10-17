// Seeds real show and music download lifecycles in the disposable Intercom preview.
// Every record is created through HolyArrs APIs and the preview's fake providers.
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { basename, join, relative, resolve, sep } from 'node:path';
import { tmpdir } from 'node:os';
import { pathToFileURL } from 'node:url';

const backend = 'http://127.0.0.1:18747';
const qbit = 'http://127.0.0.1:28768';
const downloadRoot = resolve(tmpdir(), 'holyarrs-intercom-preview-18747', 'downloads');
const libraryRoot = resolve(tmpdir(), 'holyarrs-intercom-preview-media-18747');
const sleep = (ms) => new Promise((done) => setTimeout(done, ms));

const targets = {
	show: [
		{ title: 'Harbour Lights', episode: 'First Light', state: 'completed' },
		{ title: 'Harbour Lights', episode: 'Low Tide', state: 'decision' },
		{ title: 'Harbour Lights', episode: 'Return of the Fog', state: 'paused' },
		{ title: 'Lantern Street', episode: 'The First Lantern', state: 'downloading' },
		{ title: 'Lantern Street', episode: 'Wick', state: 'downloading' },
	],
	artist: [
		{ title: 'Tin Lanterns', album: 'Harbour Songs', state: 'completed' },
		{ title: 'Tin Lanterns', album: 'Night Ferry', state: 'decision' },
		{ title: 'Moss Choir', album: 'Slow Green', state: 'paused' },
		{ title: 'twenty one pilots', album: 'Clancy', state: 'downloading' },
	],
};

async function request(method, path, body, csrf = '') {
	const headers = {};
	if (body !== undefined) headers['Content-Type'] = 'application/json';
	if (csrf) headers['X-CSRF-Token'] = csrf;
	const response = await fetch(`${backend}${path}`, {
		method,
		headers,
		body: body === undefined ? undefined : JSON.stringify(body),
	});
	const text = await response.text();
	let data = {};
	try { data = text ? JSON.parse(text) : {}; } catch { data = { text }; }
	if (!response.ok) throw new Error(`${method} ${path}: ${response.status} ${data.error ?? text}`.trim());
	return data;
}

const get = (path) => request('GET', path);

function exactDownloadRoot(path) {
	const actual = resolve(path);
	if (actual.toLowerCase() !== downloadRoot.toLowerCase()) {
		throw new Error(`Refusing non-preview download path: ${actual}`);
	}
	return actual;
}

function previewLibraryFolder(path, kind) {
	const kindRoot = resolve(libraryRoot, kind === 'show' ? 'shows' : 'music');
	const folder = resolve(path);
	if (!folder.toLowerCase().startsWith(`${kindRoot.toLowerCase()}${sep}`)) {
		throw new Error(`Refusing non-preview ${kind} folder: ${folder}`);
	}
	return folder;
}

function safeName(name, label = 'fixture name') {
	if (!name || name === '.' || name === '..' || basename(name) !== name || name.includes('/') || name.includes('\\')) {
		throw new Error(`Unsafe ${label}: ${JSON.stringify(name)}`);
	}
	return name;
}

function writeFixture(path, content) {
	try {
		writeFileSync(path, content, { flag: 'wx' });
	} catch (error) {
		if (error?.code !== 'EEXIST' || readFileSync(path, 'utf8') !== content) throw error;
	}
}

async function setDownloader(record, state, progress, files = '') {
	exactDownloadRoot(record.SavePath);
	const params = new URLSearchParams({
		hash: record.ExternalID,
		state,
		progress: String(progress),
		save_path: record.SavePath,
	});
	if (files) params.set('files', files);
	const response = await fetch(`${qbit}/set?${params}`);
	if (!response.ok) throw new Error(`Fake downloader refused ${state} for download ${record.ID}.`);
}

async function preflight() {
	const me = await get('/api/v1/auth/me');
	if (me.role !== 'owner') throw new Error('Disposable preview owner access is unavailable.');
	const sources = (await get('/api/v1/sources')).sources ?? [];
	if (sources.length !== 1) throw new Error(`Expected one disposable Source, found ${sources.length}.`);
	const kinds = sources[0].kinds ?? [];
	const required = { tv: 5000, music: 3000 };
	for (const [kind, category] of Object.entries(required)) {
		if (!kinds.includes(kind)) {
			throw new Error(`Disposable Source ${sources[0].id} does not serve ${kind}; current kinds: ${kinds.join(', ') || '(none)'}.`);
		}
		if (!(sources[0].categories?.[kind] ?? []).includes(category)) {
			throw new Error(`Disposable Source ${sources[0].id} lacks ${kind} category ${category}.`);
		}
	}
	return me.csrf ?? '';
}

async function titleByName(kind, name) {
	const titles = (await get('/api/v1/titles?limit=500')).titles ?? [];
	const title = titles.find((item) => item.kind === kind && item.name === name);
	if (!title) throw new Error(`Disposable ${kind} title is missing: ${name}.`);
	previewLibraryFolder(title.folder_path, kind);
	return title;
}

async function showTarget(spec) {
	const title = await titleByName('show', spec.title);
	const episodes = (await get(`/api/v1/titles/${title.id}/episodes`)).episodes ?? [];
	const episode = episodes.find((item) => item.name === spec.episode);
	const slot = episode?.slots?.find((item) => ['wanted', 'searching', 'chosen', 'downloading', 'stalled', 'needs_a_decision', 'in_library', 'better_wanted'].includes(item.state));
	if (!episode || !slot) throw new Error(`Wanted episode is missing: ${spec.title} / ${spec.episode}.`);
	return { ...spec, kind: 'show', title, item: episode, slots: [slot.id] };
}

async function artistTarget(spec) {
	const title = await titleByName('artist', spec.title);
	const albums = (await get(`/api/v1/titles/${title.id}/albums`)).albums ?? [];
	const album = albums.find((item) => item.name === spec.album);
	if (!album) throw new Error(`Disposable album is missing: ${spec.title} / ${spec.album}.`);
	const tracks = (await get(`/api/v1/albums/${album.id}/tracks`)).tracks ?? [];
	const slots = tracks.flatMap((track) => track.slots ?? []).map((slot) => slot.id);
	if (!tracks.length || slots.length !== tracks.length) throw new Error(`Album has no complete track slots: ${spec.title} / ${spec.album}.`);
	return { ...spec, kind: 'artist', title, item: album, tracks, slots };
}

function rowMatches(row, target) {
	if (row.kind !== target.kind || row.title_id !== target.title.id) return false;
	const rowSlots = new Set((row.episodes ?? []).map((item) => item.slot_id));
	// A forced preview pick may intentionally cover one representative track
	// of a larger album. That is sufficient for an active Download example;
	// completed and decision examples still require every target Slot.
	if (target.state === 'downloading') return target.slots.some((id) => rowSlots.has(id));
	return target.slots.every((id) => rowSlots.has(id));
}

async function currentRow(target) {
	const rows = (await get('/api/v1/downloads/view?limit=500')).downloads ?? [];
	return rows.find((row) => rowMatches(row, target));
}

async function waitForRow(target, phases, timeoutMs = 60_000) {
	const deadline = Date.now() + timeoutMs;
	while (Date.now() < deadline) {
		const row = await currentRow(target);
		if (row && phases.includes(row.phase)) return row;
		await sleep(500);
	}
	throw new Error(`Timed out waiting for ${target.title.name} / ${target.item.name} in ${phases.join(' or ')}.`);
}

async function ensureDownload(target, csrf) {
	let row = await currentRow(target);
	if (!row) {
		const body = target.kind === 'show' ? { item_id: target.item.id } : { album_id: target.item.id };
		await request('POST', `/api/v1/titles/${target.title.id}/search`, body, csrf);
		row = await waitForRow(target, ['waiting', 'downloading', 'stalled', 'paused'], 60_000);
	}
	const raw = (await get('/api/v1/downloads?limit=500')).downloads ?? [];
	const record = raw.find((item) => item.ID === row.id);
	if (!record?.ExternalID || !record?.SavePath) throw new Error(`Download ${row.id} has no fake downloader record.`);
	exactDownloadRoot(record.SavePath);
	return { row, record };
}

function fixtureFolder(record, releaseName) {
	const root = exactDownloadRoot(record.SavePath);
	const folderName = safeName(releaseName, 'release folder');
	const folder = resolve(root, folderName);
	if (!folder.startsWith(`${root}${sep}`)) throw new Error(`Fixture escaped preview downloads: ${folder}`);
	mkdirSync(folder, { recursive: true });
	return { root, folder, folderName };
}

function showFiles(record, target, releaseName, present) {
	const fixture = fixtureFolder(record, releaseName);
	const filename = safeName(`${fixture.folderName}.mkv`, 'episode file');
	const rel = `${fixture.folderName}/${filename}`;
	if (present) writeFixture(join(fixture.folder, filename), `HolyArrs disposable show fixture for ${target.title.name} ${target.item.name}.\n`);
	return [rel];
}

function musicFiles(record, target, releaseName, present) {
	const fixture = fixtureFolder(record, releaseName);
	return target.tracks.map((track) => {
		const filename = safeName(`${String(track.number).padStart(2, '0')} - ${track.name}.flac`, 'track file');
		const rel = `${fixture.folderName}/${filename}`;
		if (present) writeFixture(join(fixture.folder, filename), `HolyArrs disposable music fixture for ${target.title.name} / ${target.item.name} / ${track.name}.\n`);
		return rel;
	});
}

async function waitForPlaced(target, downloadID, timeoutMs = 60_000) {
	const deadline = Date.now() + timeoutMs;
	const titleFolder = previewLibraryFolder(target.title.folder_path, target.kind);
	const importKind = target.kind === 'show' ? 'episodes' : 'tracks';
	while (Date.now() < deadline) {
		const imports = (await get('/api/v1/imports?limit=500')).imports ?? [];
		const placed = imports.find((item) => item.DownloadID === downloadID && item.Kind === importKind && item.State === 'done' &&
			item.Result?.length && item.Result.every((result) => result.outcome === 'placed'));
		if (placed && placed.Plan?.length === target.slots.length) {
			const planSlots = new Set(placed.Plan.flatMap((step) => step.target_slot_ids ?? []));
			const paths = placed.Plan.map((step) => resolve(step.target_path));
			if (target.slots.every((id) => planSlots.has(id)) && paths.every((path) => existsSync(path) && path.startsWith(`${titleFolder}${sep}`))) {
				return { import_id: placed.ID, download_id: downloadID, files: paths.map((path) => relative(titleFolder, path)) };
			}
		}
		await sleep(500);
	}
	throw new Error(`Timed out waiting for genuine placed ${target.kind} import: ${target.title.name} / ${target.item.name}.`);
}

async function waitForDecision(target, downloadID, timeoutMs = 60_000) {
	const deadline = Date.now() + timeoutMs;
	while (Date.now() < deadline) {
		const row = await currentRow(target);
		const items = (await get('/api/v1/needs-decision')).items ?? [];
		const found = items.filter((item) => item.download_id === downloadID && item.kind === target.kind);
		if (row?.phase === 'needs_a_decision' && found.length === target.slots.length && target.slots.every((id) => found.some((item) => item.slot_id === id))) {
			return { download_id: downloadID, decisions: found.length };
		}
		await sleep(500);
	}
	throw new Error(`Timed out waiting for real ${target.kind} decision cards: ${target.title.name} / ${target.item.name}.`);
}

async function seedSampleShowMusicActivity() {
	const csrf = await preflight();
	const originalSettings = await get('/api/v1/settings/search');
	const relaxedSettings = { ...originalSettings, refuse_tiny_releases: false, import_size_floors: false };
	const resolved = [];
	for (const spec of targets.show) resolved.push(await showTarget(spec));
	for (const spec of targets.artist) resolved.push(await artistTarget(spec));

	const seeded = [];
	try {
		await request('PUT', '/api/v1/settings/search', relaxedSettings, csrf);
		for (const target of resolved) {
			const existing = await currentRow(target);
			if (target.state === 'completed' && existing?.phase === 'done') {
				seeded.push({ kind: target.kind, title: target.title.name, item: target.item.name, state: 'completed', ...await waitForPlaced(target, existing.id) });
				continue;
			}
			if (target.state === 'decision' && existing?.phase === 'needs_a_decision') {
				seeded.push({ kind: target.kind, title: target.title.name, item: target.item.name, state: 'decision', ...await waitForDecision(target, existing.id) });
				continue;
			}
			const { row, record } = await ensureDownload(target, csrf);
			if (target.state === 'completed') {
				const files = target.kind === 'show' ? showFiles(record, target, row.name, true) : musicFiles(record, target, row.name, true);
				await setDownloader(record, 'stalledUP', 1, files.join('|'));
				seeded.push({ kind: target.kind, title: target.title.name, item: target.item.name, state: 'completed', ...await waitForPlaced(target, row.id) });
			} else if (target.state === 'decision') {
				const files = target.kind === 'show' ? showFiles(record, target, row.name, false) : musicFiles(record, target, row.name, false);
				await setDownloader(record, 'stalledUP', 1, files.join('|'));
				seeded.push({ kind: target.kind, title: target.title.name, item: target.item.name, state: 'decision', ...await waitForDecision(target, row.id) });
			} else if (target.state === 'paused') {
				await setDownloader(record, 'pausedDL', 0.55);
				const paused = await waitForRow(target, ['paused']);
				seeded.push({ kind: target.kind, title: target.title.name, item: target.item.name, state: paused.phase, download_id: paused.id });
			} else {
				await setDownloader(record, 'downloading', 0.4);
				const active = await waitForRow(target, ['downloading']);
				seeded.push({ kind: target.kind, title: target.title.name, item: target.item.name, state: active.phase, download_id: active.id });
			}
		}
	} finally {
		await request('PUT', '/api/v1/settings/search', originalSettings, csrf);
	}

	const restored = await get('/api/v1/settings/search');
	if (JSON.stringify(restored) !== JSON.stringify(originalSettings)) throw new Error('Search settings were not restored exactly.');
	const rows = (await get('/api/v1/downloads/view?limit=500')).downloads ?? [];
	const phases = {};
	for (const kind of ['show', 'artist']) {
		phases[kind] = rows.filter((row) => row.kind === kind).reduce((counts, row) => {
			counts[row.phase] = (counts[row.phase] ?? 0) + 1;
			return counts;
		}, {});
		for (const phase of ['downloading', 'paused', 'done', 'needs_a_decision']) {
			if (!phases[kind][phase]) throw new Error(`${kind} sample lacks ${phase}.`);
		}
	}
	return { seeded, phases, settings_restored: true };
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
	seedSampleShowMusicActivity().then((result) => console.log(JSON.stringify(result, null, 2))).catch((error) => {
		console.error(error.stack || error.message);
		process.exitCode = 1;
	});
}

export { seedSampleShowMusicActivity };
