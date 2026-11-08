// The fake services for the browser tests, on 127.0.0.1 only: HolyArrs is
// pointed at them (HOLYARRS_PLEX_URL and friends), so the tests never reach
// plex.tv or any other real service. Test-only names and passwords.
// One file per service; this file keeps the names the tests and servers import.
import { startDiscordFake } from './discord.mjs';
import { startIndexerFake } from './indexer.mjs';
import { startJellyfinFake } from './jellyfin.mjs';
import { startMusicBrainzFake } from './musicbrainz.mjs';
import { startOldRadarrFake } from './old-radarr.mjs';
import { startOldSeerrFake } from './old-seerr.mjs';
import { startPlexFake } from './plex.mjs';
import { startQbittorrentFake } from './qbittorrent.mjs';
import { startSabnzbdFake } from './sabnzbd.mjs';
import { startTmdbFake } from './tmdb.mjs';
import { startTraktFake } from './trakt.mjs';
import { startTvmazeFake } from './tvmaze.mjs';

export {
	discordPort, embyUser, fakeInstallToken, indexerKey, indexerPort, jellyKey, jellyfinPort, mbPort, mediaUser,
	oldKey, oldRadarrPort, oldSeerrPort, plexPort, plexServerToken, qbitPort, sabKey, sabPort, subsPort,
	tmdbPort, traktKey, traktPort, tvmazePort
} from './ports.mjs';
export { fakeArtists } from './artists.mjs';
export { fakeShows } from './shows.mjs';
export { fakeArtistVideos, fakeVideoAddress, fakeYouTube, tmdbMovies, videoArtist, youtubeURL } from './movies.mjs';
export { deezerURL } from './musicbrainz.mjs';
export { metadataCalls, metadataEnv } from './tmdb.mjs';
export { albumReleasesFor, animeReleaseHash, movieReleaseHash, searchDelayMs } from './indexer.mjs';
export { makeOldLibrary } from './old-library.mjs';

/** Starts every fake service; the returned function closes them all. */
export function startFakes() {
	const tmdb = startTmdbFake();
	const mb = startMusicBrainzFake();
	const tvmaze = startTvmazeFake();
	const trakt = startTraktFake();
	const plex = startPlexFake();
	const jelly = startJellyfinFake();
	const discord = startDiscordFake();
	const indexer = startIndexerFake();
	const qbit = startQbittorrentFake();
	const sab = startSabnzbdFake();
	const oldRadarr = startOldRadarrFake();
	const oldSeerr = startOldSeerrFake();
	return () => {
		oldRadarr.close();
		oldSeerr.close();
		qbit.close();
		sab.close();
		indexer.close();
		discord.close();
		tmdb.close();
		mb.close();
		tvmaze.close();
		trakt.close();
		plex.close();
		jelly.close();
	};
}
