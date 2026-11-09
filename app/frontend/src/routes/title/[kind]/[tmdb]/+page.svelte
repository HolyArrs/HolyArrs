<script lang="ts">
	import { TitlePage } from '#lib/discover/index.ts';
	import { page } from '$app/state';

	// Every movie and show opens the one title page (lib/discover TitlePage), whether it is in the Library or not; /library/<id> renders it too.
	const kind = $derived(page.params.kind ?? '');
	const tmdb = $derived(page.params.tmdb ?? '');
</script>

{#key `${kind}/${tmdb}`}<TitlePage {kind} {tmdb} />{/key}
