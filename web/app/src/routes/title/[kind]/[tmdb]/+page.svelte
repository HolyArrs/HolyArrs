<script lang="ts">
	import { page } from '$app/state';
	import TitlePage from '#lib/TitlePage.svelte';

	// Every movie and show opens the one title page (lib/TitlePage.svelte),
	// whether it is in the Library or not; /library/<id> renders it too.
	const kind = $derived(page.params.kind ?? '');
	const tmdb = $derived(page.params.tmdb ?? '');
</script>

<TitlePage {kind} {tmdb} />
