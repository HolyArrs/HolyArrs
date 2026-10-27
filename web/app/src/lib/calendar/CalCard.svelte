<script lang="ts">
	import { to } from '#lib/paths.ts';
	import WideCard from '#lib/ui/WideCard.svelte';
	import PosterCard from '#lib/ui/PosterCard.svelte';
	import type { CalendarEntry } from '#lib/types.ts';
	import { entryArt, entryDay, entryHref, itemDot } from '#lib/calendar.ts';
	import { dayChip, entryFacts, entryLine, entryTag, entryTitle, movieDates } from '#lib/calendar-looks.ts';

	// One calendar date as the app's own card: a 16:9 WideCard (episode still,
	// movie backdrop; an album's cover blown up behind its own square), or a
	// PosterCard (home releases, album squares) in the shelves.
	let {
		e,
		today,
		all = [],
		shape = 'wide',
		chip = false,
		width
	}: {
		e: CalendarEntry;
		today: string;
		/** Every loaded date, so a movie lists its other dates. */
		all?: CalendarEntry[];
		shape?: 'wide' | 'poster' | 'square';
		/** Put the short date ("Thu 29") on the art. */
		chip?: boolean;
		width?: string;
	} = $props();

	const day = $derived(entryDay(e));
	const tag = $derived(entryTag(e));
	const date = $derived(chip ? dayChip(day, today) : '');
	const badges = $derived([...new Set([date, tag].filter(Boolean))]);
	const href = $derived(to(entryHref(e)));
	const line = $derived(entryLine(e));
	const others = $derived(movieDates(e, all));
	const facts = $derived([entryFacts(e), others && `also ${others}`].filter(Boolean).join(' · '));
	const owned = $derived(itemDot(e, today) === 'have' ? ({ state: 'available', label: 'In the Library' } as const) : null);
	const kind = $derived(e.kind === 'episode' ? 'episode' : e.kind === 'album' ? 'artist' : 'movie');
</script>

{#if shape === 'wide'}
	<WideCard
		title={entryTitle(e)}
		subtitle={line || undefined}
		meta={facts}
		{href}
		image={e.kind === 'album' ? (e.poster ?? null) : entryArt(e)}
		inset={e.kind === 'album' ? (e.poster ?? null) : null}
		{badges}
		{kind}
		{owned}
		{width}
	/>
{:else}
	<PosterCard
		title={entryTitle(e)}
		subtitle={[line, facts].filter(Boolean).join(' · ')}
		{href}
		image={e.poster ?? null}
		{badges}
		kind={kind === 'artist' ? 'artist' : 'movie'}
		shape={shape === 'square' ? 'square' : 'poster'}
		music={e.kind === 'album'}
		{owned}
		{width}
	/>
{/if}
