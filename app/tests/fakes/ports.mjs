// Ports, test-only keys and people of the fake services (shared by every fakes/ file and the tests).
export const plexPort = Number(process.env.HOLYARRS_E2E_PLEX_PORT ?? 18742);
export const jellyfinPort = Number(process.env.HOLYARRS_E2E_JELLYFIN_PORT ?? 18743);
export const tmdbPort = Number(process.env.HOLYARRS_E2E_TMDB_PORT ?? 18744);
export const traktPort = Number(process.env.HOLYARRS_E2E_TRAKT_PORT ?? 18745);
/** A fake TVMaze (the second show details source). */
export const tvmazePort = Number(process.env.HOLYARRS_E2E_TVMAZE_PORT ?? 18740);
/** A fake MusicBrainz (artists, albums, Pressings and tracks). */
export const mbPort = Number(process.env.HOLYARRS_E2E_MB_PORT ?? 18739);
/** A fake Discord: it keeps what is posted to it; GET /posts reads them back. */
export const discordPort = Number(process.env.HOLYARRS_E2E_DISCORD_PORT ?? 18746);
/** A fake Torznab Source and a plain rss feed (test-only key). */
export const indexerPort = Number(process.env.HOLYARRS_E2E_INDEXER_PORT ?? 18747);
export const indexerKey = ['stand', 'in', 'source'].join('-');
/** The only Trakt key the fake Trakt takes (test-only). */
export const traktKey = ['stand', 'in', 'trakt'].join('-');
/** A fake qBittorrent and a fake SABnzbd (test-only key) for the Downloader tests. */
export const qbitPort = Number(process.env.HOLYARRS_E2E_QBIT_PORT ?? 18748);
export const sabPort = Number(process.env.HOLYARRS_E2E_SAB_PORT ?? 18749);
export const sabKey = ['stand', 'in', 'sab', 'key'].join('-');
// The lab's fake Subtitle site (cmd/holyarrs-fakesubs), started by server.mjs.
export const subsPort = Number(process.env.HOLYARRS_E2E_SUBS_PORT ?? 18738);
export const mediaUser = { name: 'mum', password: ['e2e', 'media', 'pw'].join('-') };
/** The fake Emby lives under /emby on the fake Jellyfin's port, with its own person. */
export const embyUser = { name: 'gran', password: ['e2e', 'emby', 'pw'].join('-') };
/** The only API key the fake Jellyfin takes for the media server link (test-only). */
export const jellyKey = ['stand', 'in', 'jf'].join('-');
/** A fake old Radarr and a fake old Seerr for the move tests (test-only key). */
export const oldRadarrPort = Number(process.env.HOLYARRS_E2E_OLD_RADARR_PORT ?? 18736);
export const oldSeerrPort = Number(process.env.HOLYARRS_E2E_OLD_SEERR_PORT ?? 18737);
export const oldKey = ['stand', 'in', 'old', 'app'].join('-');
/** The only token the fake Plex takes as a media server (test-only). */
export const plexServerToken = ['stand', 'in', 'plex', 'server'].join('-');

// The fake metadata server's one installation credential.
export const fakeInstallToken = ['hai', 'stand', 'in', 'install'].join('_');
