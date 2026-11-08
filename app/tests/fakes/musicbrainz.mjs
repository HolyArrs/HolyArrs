import { createServer } from 'node:http';
import { mbPort } from './ports.mjs';
import { send, squash } from './http.mjs';
import { fakeArtists, mbid } from './artists.mjs';

/** A fake Deezer artist search (artist pictures), served by the fake MusicBrainz under /deezer. */
export const deezerURL = `http://127.0.0.1:${mbPort}/deezer`;
// A tiny made-up PNG (2x2, one colour) served as the artist picture.
const artistPicture = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAIAAAD91JpzAAAAFklEQVR4nGNk+M/AwMDAxMDAwMDAAAAZ5wEEtUNqgwAAAABJRU5ErkJggg==', 'base64');

/** @param {any} a */
function mbArtist(a, score = 100) {
	return { id: a.id, name: a.name, 'sort-name': a.name, type: a.type, country: a.country, disambiguation: a.disambiguation, score, 'life-span': { ended: a.ended } };
}

/** A release's tracks get ids of their own; recordings are shared by every Pressing of an album. @param {any} p @param {number} albumN @param {string} albumId @param {string} albumName */
function mbRelease(p, albumN, albumId, albumName) {
	const [id, note, country, date, format, count] = p;
	const n = Number(id.slice(-12));
	const tracks = Array.from({ length: count }, (_, i) => ({
		id: mbid(`d${id.slice(1, 8)}`, n * 100 + i + 1),
		position: i + 1,
		title: `Song ${albumN}.${i + 1}`,
		length: 180000 + i * 15000,
		recording: { id: mbid(`e${albumId.slice(1, 8)}`, albumN * 100 + i + 1), length: 180000 + i * 15000 }
	}));
	return { id, title: albumName, status: 'Official', country, date, disambiguation: note, media: [{ position: 1, format, tracks }] };
}

/** Starts the fake MusicBrainz (and the fake Deezer under /deezer). */
export function startMusicBrainzFake() {
	// Fake MusicBrainz: artist search, an artist, its release groups and
	// each release group's releases with media and tracks.
	const mb = createServer((req, res) => {
		const url = new URL(req.url ?? '/', 'http://x');
		// Fake Deezer: the first made-up artist has a picture, the others
		// have none (an empty picture id, as Deezer answers).
		if (url.pathname === '/deezer/search/artist') {
			const q = squash(url.searchParams.get('q') ?? '');
			const data = fakeArtists
				.map((a, i) => ({ a, i }))
				.filter(({ a }) => q && squash(a.name).includes(q))
				.map(({ a, i }) => {
					const pic = i === 0 ? `${deezerURL}/images/artist/${i + 1}/1000x1000.png` : `${deezerURL}/images/artist//1000x1000.png`;
					return { id: i + 1, name: a.name, picture_xl: pic, picture_big: pic, picture_medium: pic };
				});
			return send(res, 200, { data, total: data.length });
		}
		if (/^\/deezer\/images\/artist\/\d+\/1000x1000\.png$/.test(url.pathname)) {
			res.writeHead(200, { 'content-type': 'image/png', 'content-length': artistPicture.length });
			return res.end(artistPicture);
		}
		if (url.pathname === '/ws/2/artist') {
			const q = squash(url.searchParams.get('query') ?? '');
			return send(res, 200, { artists: fakeArtists.filter((a) => q && squash(a.name).includes(q)).map((a) => mbArtist(a)) });
		}
		const one = url.pathname.match(/^\/ws\/2\/artist\/([0-9a-f-]+)$/);
		if (one) {
			const a = fakeArtists.find((x) => x.id === one[1]);
			return a ? send(res, 200, mbArtist(a)) : send(res, 404, { error: 'Not Found' });
		}
		if (url.pathname === '/ws/2/release-group') {
			const a = fakeArtists.find((x) => x.id === url.searchParams.get('artist'));
			const groups = (a?.albums ?? []).map((al) => ({ id: al[0], title: al[1], 'primary-type': al[2], 'secondary-types': al[3], 'first-release-date': al[4], disambiguation: '' }));
			return send(res, 200, { 'release-group-count': groups.length, 'release-groups': groups });
		}
		if (url.pathname === '/ws/2/release') {
			const rg = url.searchParams.get('release-group');
			for (const a of fakeArtists) {
				const i = a.albums.findIndex((al) => al[0] === rg);
				if (i >= 0) {
					const al = a.albums[i];
					const releases = /** @type {any[]} */ (al[5]).map((p) => mbRelease(p, i + 1, al[0], al[1]));
					return send(res, 200, { 'release-count': releases.length, releases });
				}
			}
			return send(res, 200, { 'release-count': 0, releases: [] });
		}
		send(res, 404, { error: 'Not Found' });
	});
	mb.listen(mbPort, '127.0.0.1');
	return mb;
}
