<script lang="ts">
	// Open in Plex, Jellyfin or Emby for a Library title: one button per media
	// server that has it (Jellyfin and Emby only with an address browsers reach). Shown only to people who
	// manage the Library (the route needs manage_library). Nothing shows while
	// it loads, when no server has the title, or when asking fails.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import ActionButton from '#lib/ui/ActionButton.svelte';
	import { api } from '#lib/core/api.ts';
	import type { WatchLink } from './types.ts';

	let { titleId }: { titleId: number } = $props();

	let links = $state<WatchLink[]>([]);

	$effect(() => {
		const id = titleId;
		links = [];
		api.get<{ links: WatchLink[] }>(`/media-servers/watch-links?title=${id}`).then(
			(r) => {
				if (id === titleId) links = r.links;
			},
			() => {}
		);
	});

	const kindName = { plex: 'Plex', jellyfin: 'Jellyfin', emby: 'Emby' } as const;
	// Two servers of one kind are told apart by their names.
	const nameOf = (l: WatchLink) => (links.filter((o) => o.kind === l.kind).length > 1 ? l.server : kindName[l.kind]);
</script>

{#each links as l (l.url)}
	<ActionButton icon="play" pill href={l.url} target="_blank" label={t('mediaservers.openIn', { name: nameOf(l) })} data-testid="watch-link" />
{/each}
