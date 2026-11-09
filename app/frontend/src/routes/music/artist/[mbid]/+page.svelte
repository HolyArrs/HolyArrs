<script lang="ts">
	import { artistSheets } from '#lib/requests/index.ts';
	import { ArtistOverview as MusicArtistPage } from '#lib/music/index.ts';
	import { page } from '$app/state';
</script>

<MusicArtistPage mbid={page.params.mbid ?? ''} sheets={artistSheets} />
