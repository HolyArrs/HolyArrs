// Builds the disposable Intercom preview's real-name sample catalog.
// Public metadata only: TVMaze, Apple's Search API, and the checked movie list.
import { createHash } from 'node:crypto';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const repo = resolve(here, '../../..');
const output = resolve(here, 'sample-catalog.json');
const verifiedMovies = resolve(repo, '.omx/artifacts/visual-ralph/intercom/verified-tmdb-movies.json');

async function json(url) {
	const response = await fetch(url, { headers: { 'User-Agent': 'HolyArrs-preview-catalog/1.0' } });
	if (!response.ok) throw new Error(`${new URL(url).host} answered ${response.status}`);
	return response.json();
}

function uuid(seed) {
	const h = createHash('sha256').update(seed).digest('hex').slice(0, 32).split('');
	h[12] = '4';
	h[16] = ['8', '9', 'a', 'b'][Number.parseInt(h[16], 16) % 4];
	return `${h.slice(0, 8).join('')}-${h.slice(8, 12).join('')}-${h.slice(12, 16).join('')}-${h.slice(16, 20).join('')}-${h.slice(20).join('')}`;
}

function plain(value = '') {
	return String(value).replace(/<[^>]*>/g, ' ').replace(/\s+/g, ' ').trim();
}

async function mapLimit(items, limit, fn) {
	const out = new Array(items.length);
	let next = 0;
	await Promise.all(Array.from({ length: Math.min(limit, items.length) }, async () => {
		for (;;) {
			const index = next++;
			if (index >= items.length) return;
			out[index] = await fn(items[index], index);
		}
	}));
	return out;
}

function readMovies() {
	if (!existsSync(verifiedMovies)) throw new Error(`Verified movie input is missing: ${verifiedMovies}`);
	const source = JSON.parse(readFileSync(verifiedMovies, 'utf8'));
	const list = Array.isArray(source) ? source : source.movies;
	if (!Array.isArray(list) || list.length < 50) throw new Error('Verified movie input needs at least 50 entries.');
	return list.slice(0, 96).map((movie, index) => ({
		id: Number(movie.id) || 9_900_000 + index + 1,
		title: movie.title,
		year: Number(movie.year) || undefined,
		release_date: Number(movie.year) ? `${Number(movie.year)}-01-01` : '',
		imdb_id: movie.imdb_id || movie.imdb || `tt99${String(index + 1).padStart(6, '0')}`,
		overview: movie.overview || movie.extract || [movie.genres?.join(', '), movie.cast?.slice(0, 3).join(', ')].filter(Boolean).join(' · '),
		artwork: String(movie.artwork || movie.thumbnail || movie.poster_url || '').replace(/^http:/, 'https:')
	}));
}

async function readShows() {
	const page = await json('https://api.tvmaze.com/shows?page=0');
	const chosen = page
		.filter((show) => show.image?.original && show.externals?.thetvdb && show.premiered)
		.sort((a, b) => (b.weight || 0) - (a.weight || 0))
		.slice(0, 60);
	return mapLimit(chosen, 6, async (show, index) => {
		let episodes = [];
		try {
			episodes = (await json(`https://api.tvmaze.com/shows/${show.id}/episodes`)).slice(0, 8);
		} catch {
			// A title remains useful in the preview even when its episode list is unavailable.
		}
		return {
			tmdb: 9_800_000 + index + 1,
			tvmaze: show.id,
			tvdb: show.externals.thetvdb,
			imdb: show.externals.imdb || `tt98${String(index + 1).padStart(6, '0')}`,
			name: show.name,
			first: show.premiered,
			status: show.status === 'Ended' ? 'Ended' : 'Returning Series',
			network: show.network?.name || show.webChannel?.name || 'Streaming',
			overview: plain(show.summary) || `${show.genres?.join(', ') || 'Series'} from TVMaze.`,
			artwork: show.image.original,
			episodes: episodes.filter((ep) => ep.season && ep.number && ep.airdate).map((ep) => [ep.season, ep.number, ep.name, ep.airdate])
		};
	});
}

async function readArtists() {
	const terms = ['rock', 'jazz', 'soul', 'electronic', 'indie', 'country', 'classical', 'hip hop'];
	const answers = await mapLimit(terms, 4, (term) => json(`https://itunes.apple.com/search?term=${encodeURIComponent(term)}&entity=album&limit=50`));
	const byArtist = new Map();
	for (const answer of answers) {
		for (const album of answer.results || []) {
			if (!album.artistName || !album.collectionName || !album.artworkUrl100) continue;
			const key = album.artistName.toLowerCase();
			const current = byArtist.get(key) || { name: album.artistName, albums: [] };
			if (!current.albums.some((item) => item.name === album.collectionName)) {
				current.albums.push({
					id: uuid(`album:${album.collectionId}:${album.collectionName}`),
					name: album.collectionName,
					type: 'Album',
					release_date: String(album.releaseDate || '2020-01-01').slice(0, 10),
					artwork: album.artworkUrl100.replace(/100x100(?:bb)?/, '600x600bb')
				});
			}
			byArtist.set(key, current);
		}
	}
	return [...byArtist.values()].filter((artist) => artist.albums.length).slice(0, 30).map((artist) => ({
		id: uuid(`artist:${artist.name}`),
		name: artist.name,
		type: '',
		country: '',
		artwork: artist.albums[0].artwork,
		albums: artist.albums.slice(0, 4)
	}));
}

const [shows, artists] = await Promise.all([readShows(), readArtists()]);
const catalog = { generated_at: new Date().toISOString(), sources: ['TVMaze', 'Apple Search API', 'public TMDB datasets'], movies: readMovies(), shows, artists };
if (catalog.movies.length < 50 || catalog.shows.length < 60 || catalog.artists.length < 30) {
	throw new Error(`Catalog is too small: ${catalog.movies.length} movies, ${catalog.shows.length} shows, ${catalog.artists.length} artists.`);
}
writeFileSync(output, `${JSON.stringify(catalog, null, 2)}\n`);
process.stdout.write(`Wrote ${catalog.movies.length} movies, ${catalog.shows.length} shows and ${catalog.artists.length} artists to ${output}\n`);
