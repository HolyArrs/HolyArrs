<script lang="ts">
	import { artistSheets } from '../../../request-sheets.ts';
	import { page } from '$app/state';
	import { ArtistOverview as MusicArtistPage } from '#lib/music/index.ts';
</script>

<MusicArtistPage mbid={page.params.mbid ?? ''} sheets={artistSheets} />
