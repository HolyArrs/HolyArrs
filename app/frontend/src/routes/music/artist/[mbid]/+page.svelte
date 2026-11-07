<script lang="ts">
	import { page } from '$app/state';
	import MusicArtistPage from '#lib/music/MusicArtistPage.svelte';
</script>

<MusicArtistPage mbid={page.params.mbid ?? ''} />
