# Third-party notices

HolyArrs is free software under the GPL-3.0 (see `LICENSE`). It shows information from outside services and is built with open-source parts. Each keeps its own rights.

## Data sources

| Source | What HolyArrs uses it for | Licence or terms |
| --- | --- | --- |
| [TMDB](https://www.themoviedb.org/) | Movie and show details, people, posters, backdrops, trailer links | [TMDB API terms](https://www.themoviedb.org/api-terms-of-use) |
| [Wikipedia](https://www.wikipedia.org/) | Short bios for people and artists (text may be shortened) | [CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/) |
| [Wikidata](https://www.wikidata.org/) | Awards, death dates, ID links | [CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/) |
| [Wikimedia Commons](https://commons.wikimedia.org/) | People's photos when TMDB has none (each photo's author and licence are on its Commons page) | Per file, mostly [CC BY-SA](https://creativecommons.org/licenses/by-sa/4.0/), [CC BY](https://creativecommons.org/licenses/by/4.0/) or public domain |
| [TVmaze](https://www.tvmaze.com/) | People's photos when TMDB and Wikimedia Commons have none | [TVmaze API licence (CC BY-SA)](https://www.tvmaze.com/api#licensing) |
| [MusicBrainz](https://musicbrainz.org/) | Artists, albums, editions, tracks | [MusicBrainz data licence](https://musicbrainz.org/doc/About/Data_License) (CC0 core data) |
| [Cover Art Archive](https://coverartarchive.org/) | Album covers | Each image keeps its uploader's terms |
| [ListenBrainz](https://listenbrainz.org/) | Music popularity, similar artists | CC0 |
| [fanart.tv](https://fanart.tv/) | Artist and title artwork | fanart.tv API terms |
| [Deezer](https://www.deezer.com/) | Music popularity, labels, explicit marks | [Deezer API terms](https://developers.deezer.com/termsofuse) |
| iTunes Search and Apple Music RSS | Music charts, some album art | [Apple Media Services terms](https://www.apple.com/legal/internet-services/itunes/) |
| [TVmaze](https://www.tvmaze.com/) | Show schedules, episode dates | [CC BY-SA 4.0](https://www.tvmaze.com/api#licensing) |
| [MDBList](https://mdblist.com/) | Ratings gathered from several sites | MDBList API terms |
| [Streaming Availability API](https://www.movieofthenight.com/about/api) (Movie of the Night) | Which streaming services carry a title | Movie of the Night terms |
| [AniList](https://anilist.co/) | Anime details | [AniList API terms](https://docs.anilist.co/guide/terms-of-use) |
| [Anime-Lists](https://github.com/Anime-Lists/anime-lists) and [Fribb anime-lists](https://github.com/Fribb/anime-lists) | Links between anime IDs on different sites | See each project |
| [YouTube](https://www.youtube.com/) | Trailers | [YouTube terms](https://www.youtube.com/t/terms) |

This product uses the TMDB API but is not endorsed or certified by TMDB. None of the sources above endorse HolyArrs.

## Open-source parts

Direct dependencies, from `go.mod` and `web/app/package.json`:

| Package | Licence |
| --- | --- |
| github.com/coreos/go-oidc/v3 | Apache-2.0 |
| github.com/go-jose/go-jose/v4 | Apache-2.0 |
| github.com/nwaples/rardecode/v2 (unpacking compressed RAR files) | BSD-2-Clause |
| github.com/bodgit/sevenzip (7-Zip archives) | BSD-3-Clause |
| github.com/bodgit/plumbing (used by the 7-Zip reader) | BSD-3-Clause |
| github.com/bodgit/windows (used by the 7-Zip reader) | BSD-3-Clause |
| github.com/therootcompany/xz (xz archives) | CC0-1.0 |
| github.com/ulikunitz/xz (LZMA in 7-Zip archives) | BSD-3-Clause |
| github.com/klauspost/compress (compression in 7-Zip archives) | BSD-3-Clause (some parts MIT and Apache-2.0) |
| github.com/pierrec/lz4/v4 (LZ4 in 7-Zip archives) | BSD-3-Clause |
| github.com/andybalholm/brotli (Brotli in 7-Zip archives) | MIT |
| github.com/stangelandcl/ppmd (PPMd in 7-Zip archives) | MIT |
| github.com/spf13/afero (file access for the 7-Zip reader) | Apache-2.0 |
| go4.org (multi-part 7-Zip reading) | Apache-2.0 |
| github.com/hashicorp/golang-lru/v2 (a small cache in the 7-Zip reader; source: https://github.com/hashicorp/golang-lru) | MPL-2.0 |
| golang.org/x/crypto, x/image, x/oauth2, x/sys, x/text | BSD-3-Clause |
| modernc.org/sqlite | BSD-3-Clause |
| hls.js | Apache-2.0 |
| @fontsource/anton (Anton font) | SIL OFL 1.1 |

Full licence texts for the sign-in parts and the archive readers are in [`docs/third-party-notices.md`](docs/third-party-notices.md).
