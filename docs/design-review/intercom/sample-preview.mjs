// Adds the generated catalog to the existing disposable metadata fakes and
// persists it through HolyArrs' public API. It never opens the database.
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
export const catalogPath = resolve(here, 'sample-catalog.json');

export function readSampleCatalog() {
	return JSON.parse(readFileSync(catalogPath, 'utf8'));
}

function pressing(album, index) {
	return [`${album.id.slice(0, 24)}${String(index + 1).padStart(12, '0')}`.slice(0, 36), '', 'XW', album.release_date, 'Digital Media', 10];
}

export function addCatalogToFakes(fakes, catalog = readSampleCatalog()) {
	const movieIDs = new Set(fakes.tmdbMovies.map((item) => item.id));
	for (const movie of catalog.movies) {
		if (movieIDs.has(movie.id)) continue;
		fakes.tmdbMovies.push({ ...movie, dates: movie.release_date ? [[3, movie.release_date], [4, movie.release_date]] : [] });
	}
	const showIDs = new Set(fakes.fakeShows.map((item) => item.tmdb));
	for (const show of catalog.shows) if (!showIDs.has(show.tmdb)) fakes.fakeShows.push(show);
	const artistIDs = new Set(fakes.fakeArtists.map((item) => item.id));
	for (const artist of catalog.artists) {
		if (artistIDs.has(artist.id)) continue;
		fakes.fakeArtists.push({
			id: artist.id, name: artist.name, type: artist.type, country: artist.country, ended: false, disambiguation: 'preview sample catalog',
			albums: artist.albums.map((album, index) => [album.id, album.name, album.type, [], album.release_date, [pressing(album, index)]])
		});
	}
}

async function api(baseURL, path, init = {}) {
	const response = await fetch(`${baseURL}${path}`, init);
	const text = await response.text();
	let data = {};
	try { data = text ? JSON.parse(text) : {}; } catch { data = { text }; }
	return { response, data };
}

export async function seedSampleCatalog(baseURL = 'http://127.0.0.1:18747', catalog = readSampleCatalog()) {
	if (baseURL !== 'http://127.0.0.1:18747') {
		throw new Error('Sample seeding is locked to the disposable backend at http://127.0.0.1:18747.');
	}
	const me = await api(baseURL, '/api/v1/auth/me');
	if (!me.response.ok || me.data.role !== 'owner') throw new Error(`Preview owner access is unavailable (${me.response.status}).`);
	const headers = { 'Content-Type': 'application/json' };
	if (me.data.csrf) headers['X-CSRF-Token'] = me.data.csrf;
	const add = async (path, body) => {
		const result = await api(baseURL, path, { method: 'POST', headers, body: JSON.stringify(body) });
		if (result.response.status !== 201 && result.response.status !== 409) {
			throw new Error(`${path} refused ${body.id}: ${result.response.status} ${result.data.error || ''}`.trim());
		}
		return result.response.status;
	};
	const counts = { movies: 0, shows: 0, artists: 0, existing: 0 };
	for (const movie of catalog.movies) {
		const status = await add('/api/v1/titles', { id: `tmdb:${movie.id}`, keep_up_to_date: false, search_on_add: false });
		status === 201 ? counts.movies++ : counts.existing++;
	}
	for (const show of catalog.shows) {
		const status = await add('/api/v1/shows', { id: `tmdb:${show.tmdb}`, keep_up_to_date: false, seasons: 'all', new_seasons: 'none', details_source: 'tmdb' });
		status === 201 ? counts.shows++ : counts.existing++;
	}
	for (const artist of catalog.artists) {
		const status = await add('/api/v1/artists', { id: `mbid:${artist.id}`, keep_up_to_date: false, albums: 'all', new_albums: 'none', album_types: ['album'], release_statuses: ['official'] });
		status === 201 ? counts.artists++ : counts.existing++;
	}
	return counts;
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
	const counts = await seedSampleCatalog();
	process.stdout.write(`${JSON.stringify(counts)}\n`);
}
