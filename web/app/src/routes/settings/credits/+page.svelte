<script lang="ts">
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';

	type Source = { name: string; gives: string; terms: string; termsURL: string; site: string };

	// Every outside source HolyArrs (or its metadata server) reads from.
	const sources: Source[] = [
		{ name: 'TMDB', gives: 'Movie and show names, dates, people, posters, backdrops and trailer links.', terms: 'TMDB API terms of use', termsURL: 'https://www.themoviedb.org/api-terms-of-use', site: 'https://www.themoviedb.org/' },
		{ name: 'Wikipedia', gives: 'Short bios for people and artists. The text may be shortened.', terms: 'CC BY-SA 4.0', termsURL: 'https://creativecommons.org/licenses/by-sa/4.0/', site: 'https://www.wikipedia.org/' },
		{ name: 'Wikidata', gives: 'Awards, death dates and links between IDs.', terms: 'CC0 1.0', termsURL: 'https://creativecommons.org/publicdomain/zero/1.0/', site: 'https://www.wikidata.org/' },
		{ name: 'MusicBrainz', gives: 'Artists, albums, editions and tracks.', terms: 'CC0 (core data) and CC BY-NC-SA 3.0 (other data)', termsURL: 'https://musicbrainz.org/doc/About/Data_License', site: 'https://musicbrainz.org/' },
		{ name: 'Cover Art Archive', gives: 'Album covers.', terms: 'Each image keeps its uploader’s terms', termsURL: 'https://coverartarchive.org/', site: 'https://coverartarchive.org/' },
		{ name: 'ListenBrainz', gives: 'Music popularity and similar artists.', terms: 'CC0', termsURL: 'https://listenbrainz.org/', site: 'https://listenbrainz.org/' },
		{ name: 'fanart.tv', gives: 'Artist and title artwork.', terms: 'fanart.tv API terms', termsURL: 'https://fanart.tv/', site: 'https://fanart.tv/' },
		{ name: 'Deezer', gives: 'Music popularity, labels and explicit marks.', terms: 'Deezer API terms', termsURL: 'https://developers.deezer.com/termsofuse', site: 'https://www.deezer.com/' },
		{ name: 'iTunes Search and Apple Music RSS', gives: 'Music charts and some album art.', terms: 'Apple Media Services terms', termsURL: 'https://www.apple.com/legal/internet-services/itunes/', site: 'https://www.apple.com/apple-music/' },
		{ name: 'TVmaze', gives: 'Show schedules and episode dates.', terms: 'CC BY-SA 4.0', termsURL: 'https://www.tvmaze.com/api#licensing', site: 'https://www.tvmaze.com/' },
		{ name: 'MDBList', gives: 'Ratings from several sites in one place.', terms: 'MDBList API terms', termsURL: 'https://mdblist.com/', site: 'https://mdblist.com/' },
		{ name: 'Streaming Availability API (Movie of the Night)', gives: 'Which streaming services carry a title.', terms: 'Movie of the Night terms', termsURL: 'https://www.movieofthenight.com/about/api', site: 'https://www.movieofthenight.com/' },
		{ name: 'AniList', gives: 'Anime details.', terms: 'AniList API terms', termsURL: 'https://docs.anilist.co/guide/terms-of-use', site: 'https://anilist.co/' },
		{ name: 'Anime-Lists and Fribb anime-lists', gives: 'Links between anime IDs on different sites.', terms: 'See each project on GitHub', termsURL: 'https://github.com/Fribb/anime-lists', site: 'https://github.com/Anime-Lists/anime-lists' },
		{ name: 'YouTube', gives: 'Trailers.', terms: 'YouTube terms of service', termsURL: 'https://www.youtube.com/t/terms', site: 'https://www.youtube.com/' }
	];
</script>

<svelte:head><title>Credits · HolyArrs</title></svelte:head>

<PageHeader title="Credits" subtitle="HolyArrs shows information from these outside sources. Thank you to everyone who builds and fills them." />

<SettingsCard id="credits-card" title="Data sources" icon="info">
	<p class="small" data-testid="tmdb-notice">This product uses the TMDB API but is not endorsed or certified by TMDB.</p>
	<ul class="sources">
		{#each sources as s (s.name)}
			<li>
				<a href={s.site} target="_blank" rel="noopener noreferrer"><b>{s.name}</b></a>
				<span class="small">{s.gives}</span>
				<span class="xs muted">Licence or terms: <a href={s.termsURL} target="_blank" rel="noopener noreferrer license">{s.terms}</a></span>
			</li>
		{/each}
	</ul>
	<p class="xs muted">Bios from Wikipedia are shared under CC BY-SA 4.0 and may be shortened. Each source keeps its own rights; HolyArrs is not endorsed by any of them.</p>
</SettingsCard>

<SettingsCard title="HolyArrs">
	<p class="small">HolyArrs is free software under the GPL-3.0. Open-source parts it is built with are listed in THIRD_PARTY_NOTICES.md in the source code.</p>
</SettingsCard>

<style>
	.sources {
		list-style: none;
		margin: var(--s-3) 0;
		padding: 0;
		display: grid;
		gap: var(--s-3);
	}
	.sources li {
		display: grid;
		gap: 2px;
		min-width: 0;
		overflow-wrap: anywhere;
	}
</style>
