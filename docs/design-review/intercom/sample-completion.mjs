// Creates genuine completed imports and decision-needed examples in the disposable preview.
// It writes only tiny synthetic files inside the preview's private temp runtime.
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { basename, join, relative, resolve, sep } from 'node:path';
import { tmpdir } from 'node:os';
import { pathToFileURL } from 'node:url';

const backend = 'http://127.0.0.1:18747';
const qbit = 'http://127.0.0.1:28768';
const runtimeName = 'holyarrs-intercom-preview-18747';
const completedTitles = ['Godzilla vs. Kong', 'Oppenheimer'];
const decisionTitles = ['After We Collided', 'Below Zero'];
const sleep = (ms) => new Promise((resolveWait) => setTimeout(resolveWait, ms));

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

function exactRuntimeChild(path, child) {
	const absolute = resolve(path);
	const expected = resolve(tmpdir(), runtimeName, child);
	if (absolute.toLowerCase() !== expected.toLowerCase()) {
		throw new Error(`Refusing non-preview ${child} path: ${absolute}`);
	}
	return absolute;
}

function safeFolderName(name) {
	if (!name || name === '.' || name === '..' || basename(name) !== name || name.includes('/') || name.includes('\\')) {
		throw new Error(`Unsafe fake download folder name: ${JSON.stringify(name)}`);
	}
	return name;
}

function previewMovieFolder(path) {
	const movies = resolve(tmpdir(), runtimeName, 'movies');
	const folder = resolve(path);
	if (!folder.startsWith(`${movies}${sep}`)) {
		throw new Error(`Refusing non-preview movie folder: ${folder}`);
	}
	return folder;
}

function fixtureFile(savePath, folder, title) {
	const downloads = exactRuntimeChild(savePath, 'downloads');
	const safeFolder = safeFolderName(folder);
	const directory = resolve(downloads, safeFolder);
	if (!directory.startsWith(`${downloads}${sep}`)) throw new Error(`Fixture escaped preview downloads: ${directory}`);
	mkdirSync(directory, { recursive: true });
	const path = join(directory, `${safeFolder}.mkv`);
	const content = `HolyArrs disposable preview fixture for ${title}.\n`;
	try {
		writeFileSync(path, content, { flag: 'wx' });
	} catch (error) {
		if (error?.code !== 'EEXIST' || readFileSync(path, 'utf8') !== content) throw error;
	}
	return { relativeFile: `${safeFolder}/${safeFolder}.mkv` };
}

async function setFinished(hash, savePath, relativeFile) {
	const params = new URLSearchParams({ hash, progress: '1', state: 'stalledUP', save_path: savePath, files: relativeFile });
	const response = await fetch(`${qbit}/set?${params}`);
	if (!response.ok) throw new Error(`Fake downloader refused completion for ${hash}.`);
}

async function titleByName(name) {
	const titles = (await get('/api/v1/titles?limit=500')).titles ?? [];
	const title = titles.find((item) => item.kind === 'movie' && item.name === name);
	if (!title) throw new Error(`Disposable sample movie is missing: ${name}.`);
	previewMovieFolder(title.folder_path);
	return title;
}

async function activeDownloadFor(title, csrf, timeoutMs = 45_000) {
	let rows = (await get('/api/v1/downloads/view?limit=500')).downloads ?? [];
	let view = rows.find((row) => row.title === title.name && row.active && row.phase === 'downloading');
	if (!view) {
		await request('POST', `/api/v1/titles/${title.id}/search`, { slot_id: title.slots[0]?.id }, csrf);
		const deadline = Date.now() + timeoutMs;
		while (Date.now() < deadline) {
			rows = (await get('/api/v1/downloads/view?limit=500')).downloads ?? [];
			view = rows.find((row) => row.title === title.name && row.active && row.phase === 'downloading');
			if (view) break;
			await sleep(500);
		}
	}
	if (!view) throw new Error(`No active fake download appeared for ${title.name}.`);
	const raw = (await get('/api/v1/downloads?limit=500')).downloads ?? [];
	const record = raw.find((row) => row.ID === view.id);
	if (!record?.ExternalID || !record?.SavePath) throw new Error(`Download ${view.id} has no fake downloader record.`);
	return { view, record };
}

function placedImportFor(imports, downloadID) {
	return imports.find((item) => item.DownloadID === downloadID && item.State === 'done' &&
		item.Result?.some((result) => result.outcome === 'placed'));
}

async function waitForPlaced(title, downloadID, timeoutMs = 45_000) {
	const deadline = Date.now() + timeoutMs;
	while (Date.now() < deadline) {
		const detail = await get(`/api/v1/titles/${title.id}`);
		const imports = (await get('/api/v1/imports?limit=200')).imports ?? [];
		const placed = placedImportFor(imports, downloadID);
		const inLibrary = detail.slots?.some((slot) => slot.state === 'in_library' || slot.state === 'better_wanted');
		if (placed && inLibrary) {
			const step = placed.Plan?.find((candidate, index) => placed.Result?.[index]?.outcome === 'placed');
			if (!step?.target_path || !existsSync(step.target_path)) {
				throw new Error(`Import ${placed.ID} did not leave its placed file on disk.`);
			}
			const folder = previewMovieFolder(title.folder_path);
			const target = resolve(step.target_path);
			if (target !== folder && !target.startsWith(`${folder}${sep}`)) {
				throw new Error(`Import ${placed.ID} placed outside ${title.name}'s library folder.`);
			}
			return { import_id: placed.ID, download_id: downloadID, file: relative(folder, target) };
		}
		await sleep(500);
	}
	throw new Error(`Timed out waiting for a placed import for ${title.name}.`);
}

async function verifyDecisionExamples() {
	const decisions = await get('/api/v1/needs-decision');
	const items = decisions.items ?? decisions.needs_decision ?? decisions.downloads ?? [];
	for (const title of decisionTitles) {
		if (!items.some((item) => item.title === title || item.title_name === title || item.name === title)) {
			throw new Error(`Needs-a-decision example is missing: ${title}.`);
		}
	}
	return decisions.count ?? items.length;
}

export async function seedSampleCompletion() {
	const me = await get('/api/v1/auth/me');
	if (me.role !== 'owner') throw new Error('Disposable preview owner access is unavailable.');
	const csrf = me.csrf ?? '';
	const originalSettings = await get('/api/v1/settings/search');
	const relaxedSettings = { ...originalSettings, refuse_tiny_releases: false, import_size_floors: false };
	const completed = [];

	try {
		await request('PUT', '/api/v1/settings/search', relaxedSettings, csrf);
		for (const name of completedTitles) {
			const title = await titleByName(name);
			const currentImports = (await get('/api/v1/imports?limit=200')).imports ?? [];
			const folder = previewMovieFolder(title.folder_path);
			const existing = currentImports.find((item) => item.State === 'done' && item.Result?.some((result) => result.outcome === 'placed') &&
				item.Plan?.some((step) => step.target_path && (resolve(step.target_path) === folder || resolve(step.target_path).startsWith(`${folder}${sep}`))));
			if (existing) {
				const step = existing.Plan.find((candidate, index) => existing.Result?.[index]?.outcome === 'placed');
				if (!existsSync(step.target_path)) throw new Error(`Recorded import ${existing.ID} is missing from disk.`);
				completed.push({ title: name, import_id: existing.ID, download_id: existing.DownloadID, file: relative(folder, step.target_path) });
				continue;
			}
			const { view, record } = await activeDownloadFor(title, csrf);
			const fixture = fixtureFile(record.SavePath, view.name, title.name);
			await setFinished(record.ExternalID, record.SavePath, fixture.relativeFile);
			completed.push({ title: name, ...await waitForPlaced(title, view.id) });
		}
	} finally {
		await request('PUT', '/api/v1/settings/search', originalSettings, csrf);
	}

	const restoredSettings = await get('/api/v1/settings/search');
	if (JSON.stringify(restoredSettings) !== JSON.stringify(originalSettings)) {
		throw new Error('Preview search settings were not restored exactly.');
	}
	const needsDecision = await verifyDecisionExamples();
	const rows = (await get('/api/v1/downloads/view?limit=500')).downloads ?? [];
	const phases = Object.fromEntries([...new Set(rows.map((row) => row.phase))].sort().map((phase) => [phase, rows.filter((row) => row.phase === phase).length]));
	return { downloads: { total: rows.length, phases }, completed, needs_decision: needsDecision, settings_restored: true };
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
	process.stdout.write(`${JSON.stringify(await seedSampleCompletion())}\n`);
}
