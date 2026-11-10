<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Avatar from '#lib/ui/Avatar.svelte';
	import { followerNames } from './calendar.ts';
	import type { CalendarEntry } from '#lib/core/types.ts';

	// Who follows a calendar title: up to three small faces and the names ("You, Ana and 1 other"). People the reader may not see are only counted.
	let { e, size = 18 }: { e: Pick<CalendarEntry, 'followers' | 'followers_hidden'>; size?: number } = $props();
	const faces = $derived((e.followers ?? []).slice(0, 3));
	const words = $derived(followerNames(e));
</script>

{#if words}
	<span class="fw" data-testid="cal-followers">
		{#if faces.length}
			<span class="faces" aria-hidden="true">
				{#each faces as p (p.id + p.name)}<span class="face"><Avatar name={p.name} src={p.avatar} {size} /></span>{/each}
			</span>
		{/if}
		<span class="names"><span class="sr-only">{t('discover.followWho.followedBy')} </span>{words}</span>
	</span>
{/if}

<style>
	.fw { display: inline-flex; align-items: center; gap: 6px; min-width: 0; color: var(--text-muted); font: var(--t-footnote); }
	.faces { display: inline-flex; flex: none; }
	.face { display: inline-flex; border-radius: 50%; box-shadow: 0 0 0 2px var(--surface-1); }
	.face + .face { margin-left: -6px; }
	.names { min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
</style>
