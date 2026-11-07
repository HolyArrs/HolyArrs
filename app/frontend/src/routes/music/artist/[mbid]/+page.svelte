<script lang="ts">
	import { page } from '$app/state';
	import { ArtistOverview as MusicArtistPage } from '#lib/music/index.ts';
</script>

<MusicArtistPage mbid={page.params.mbid ?? ''} />
