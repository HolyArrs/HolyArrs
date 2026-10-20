<script lang="ts">
	import { onMount } from 'svelte';
	import { api, message } from './api.ts';
	import { toasts } from './toasts.svelte.ts';
	import { refreshInbox } from './wishlist.svelte.ts';
	import type { OwnedIndex } from './explore.ts';
	import { defaultRegion } from './top10.ts';
	import { fetchMusicExplore, musicCardHref, musicCardNote, musicImage, type MusicCardData, type MusicExploreAnswer } from './music-trending.ts';
	import type { Ask, LibraryFolder, LookupArtist, MusicSettings, PickRulesView, TitleView } from './types.ts';
	import Shelf from './ui/Shelf.svelte';
	import PosterCard from './ui/PosterCard.svelte';
	import IconButton from './ui/IconButton.svelte';
	import EmptyState from './ui/EmptyState.svelte';
	import AddArtistDialog from './AddArtistDialog.svelte';
	import AskArtistDialog from './AskArtistDialog.svelte';

	// Explore → Music. The server builds every row (personal first, then the
	// charts, similar artists, missing albums and genre rows); genre chips
	// filter them all. Every row says why it is there; albums are square,
	// artists round, and every card is the same size.
	const KEY = 'holyarrs.music-trending';
	let {
		owned: _owned,
		manage,
		mayAsk,
		folders,
		rules,
		onadded
	}: { owned: OwnedIndex; manage: boolean; mayAsk: boolean; folders: LibraryFolder[]; rules: PickRulesView[]; onadded?: (t: TitleView) => void } = $props();

	let region = $state('global');
	let genre = $state('');
	let answer = $state<MusicExploreAnswer | null>(null);
	let chips = $state<string[]>([]);
	let failed = $state(false);
	/** Artists added or asked for here, so their cards update at once. */
	let addedHere = $state(new Map<string, number>());
	let busy = $state('');
	let defaults = $state<MusicSettings | null>(null);
	let addArtist: ReturnType<typeof AddArtistDialog> | undefined = $state();
	let askArtist: ReturnType<typeof AskArtistDialog> | undefined = $state();
	let gone: AbortController | undefined;

	const names = typeof Intl !== 'undefined' && 'DisplayNames' in Intl ? new Intl.DisplayNames(undefined, { type: 'region' }) : null;
	function regionName(r: string): string {
		if (r === 'global') return 'Worldwide';
		try {
			return names?.of(r) ?? r;
		} catch {
			return r;
		}
	}

	async function load() {
		gone?.abort();
		gone = new AbortController();
		const signal = gone.signal;
		try {
			const a = await fetchMusicExplore(genre, region, api, signal);
			if (signal.aborted) return;
			answer = a;
			failed = false;
			// Chips come from the unfiltered answer, so picking one keeps the rest.
			if (!genre) chips = a.genres ?? [];
			if (a.region && a.region !== region) region = a.region;
		} catch {
			if (!signal.aborted) failed = true;
		}
	}

	function pick() {
		try {
			localStorage.setItem(KEY, region);
		} catch {
			/* the choice is a convenience */
		}
		void load();
	}

	function chip(g: string) {
		genre = genre === g ? '' : g;
		void load();
	}

	onMount(() => {
		region = defaultRegion();
		try {
			region = localStorage.getItem(KEY) || region;
		} catch {
			/* nothing saved */
		}
		void load();
		// Genres fill in the background on a first visit: look again once.
		const again = setTimeout(() => {
			if (!genre && chips.length === 0) void load();
		}, 20000);
		if (manage) api.get<MusicSettings>('/settings/music').then((r) => (defaults = r)).catch(() => {});
		return () => {
			clearTimeout(again);
			gone?.abort();
		};
	});

	function live(c: MusicCardData): MusicCardData {
		const id = c.artist_mbid ? addedHere.get(c.artist_mbid) : undefined;
		return id !== undefined && c.kind === 'artist' ? { ...c, title_id: id } : c;
	}

	/** quick looks the artist up by name and opens the add (or ask) sheet for the matching MusicBrainz id. */
	async function quick(c: MusicCardData) {
		const artist = c.artist || c.name;
		if (busy || !c.artist_mbid) return;
		busy = c.key;
		try {
			const r = await api.get<{ artists: LookupArtist[] }>(`/artists/lookup?term=${encodeURIComponent(artist)}`);
			const a = r.artists?.find((x) => x.mbid === c.artist_mbid);
			if (!a) return void toasts.push(`Could not find ${artist}. Try Search.`, 'fail');
			if (a.title_id) {
				addedHere = new Map([...addedHere, [a.mbid, a.title_id]]);
				return void toasts.push(`${a.name} is in your Library already.`, 'ok');
			}
			if (manage) addArtist?.open(a);
			else askArtist?.open(a);
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = '';
		}
	}

	function added(a: LookupArtist, t: TitleView) {
		addedHere = new Map([...addedHere, [a.mbid, t.id]]);
		toasts.push(`${t.name} is in your Library. Albums, Pressings and tracks are being filled in.`, 'ok');
		onadded?.(t);
	}

	function asked(_a: LookupArtist, r: { ask: Ask; joined: boolean }) {
		if (r.ask.state === 'approved') toasts.push(`${r.ask.name} is approved; HolyArrs keeps it up to date.`, 'ok');
		else if (r.ask.state === 'asked' && r.joined) toasts.push(`Someone asked for ${r.ask.name} already; you are on the ask too.`, 'ok');
		else toasts.push(`Asked for ${r.ask.name}. You will hear when it is approved.`, 'ok');
		void refreshInbox();
		void load();
	}

	const hero = $derived(answer?.hero ?? null);
	let heroBroken = $state(false);
	const heroNote = $derived.by(() => {
		if (!hero) return '';
		if (!hero.title_id) return `${hero.artist} · number ${hero.rank ?? 1} on this week's album chart`;
		const out = hero.date ? `out ${new Date(`${hero.date}T00:00:00Z`).toLocaleDateString(undefined, { day: 'numeric', month: 'short', timeZone: 'UTC' })}` : '';
		return ['From an artist you have', hero.artist, out].filter(Boolean).join(' · ');
	});
	const regions = $derived(answer?.regions ?? []);
	const hasCharts = $derived(answer?.rows.some((r) => r.type === 'top_albums' || r.type === 'trending_artists') ?? false);
</script>

{#if answer?.kid}
	<EmptyState icon="music" title="Music is not shown on this account" text="Music has no age ratings, so it stays off for accounts with an age limit." />
{:else if answer}
	<section class="music" data-testid="music-trending">
		{#if chips.length}
			<div class="chips" role="group" aria-label="Genres" data-testid="music-genres">
				<button type="button" class="chip" class:on={genre === ''} aria-pressed={genre === ''} onclick={() => chip('')}>All</button>
				{#each chips as g (g)}
					<button type="button" class="chip" class:on={genre === g} aria-pressed={genre === g} onclick={() => chip(g)}>{g}</button>
				{/each}
			</div>
		{/if}

		{#if hero}
			<a class="hero" href={musicCardHref(hero)} data-testid="music-hero">
				<span class="cover">{#if hero.image && !heroBroken}<img src={musicImage(hero)} alt="" loading="lazy" onerror={() => (heroBroken = true)} />{/if}</span>
				<span class="words">
					<span class="eyebrow">New album</span>
					<strong>{hero.name}</strong>
					<span class="note">{heroNote}</span>
				</span>
			</a>
		{/if}

		{#if regions.length > 1 && hasCharts}
			<div class="pick">
				<select aria-label="Chart region" bind:value={region} onchange={pick}>
					{#each regions as r (r)}<option value={r}>{regionName(r)}</option>{/each}
				</select>
			</div>
		{/if}

		{#each answer.rows as row (row.id)}
			<Shelf title={row.title} subtitle={row.reason} item="var(--explore-item)">
				{#each row.items.map(live) as c (c.key)}
					<PosterCard
						title={c.name}
						href={musicCardHref(c)}
						image={musicImage(c)}
						kind="artist"
						subtitle={musicCardNote(row, c) || undefined}
						status={c.kind === 'artist' && c.title_id && row.type !== 'your_asks' ? 'in_library' : undefined}
						shape={c.kind === 'album' ? 'square' : 'round'}
						fresh={c.fresh}
						music
						actionOnHover
					>
						{#snippet action()}
							{#if !c.title_id && c.artist_mbid && row.type !== 'your_asks' && (manage || mayAsk)}
								<IconButton icon="plus" label={manage ? `Add ${c.artist || c.name}` : `Ask for ${c.artist || c.name}`} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={busy === c.key || undefined} onclick={() => void quick(c)} />
							{/if}
						{/snippet}
					</PosterCard>
				{/each}
			</Shelf>
		{/each}

		{#if answer.rows.length === 0 && !hero}
			<EmptyState icon="music" title={genre ? `Nothing tagged ${genre} yet` : 'Music rows are getting ready'} text={genre ? 'Pick another genre, or All.' : 'Charts and suggestions fill in within a few minutes.'} />
		{/if}
	</section>
{:else if failed}
	<EmptyState icon="music" title="Could not load Music" text="The music rows did not answer. Try again shortly." />
{/if}

{#if manage}
	<AddArtistDialog bind:this={addArtist} {folders} {rules} {defaults} onadded={added} />
{:else if mayAsk}
	<AskArtistDialog bind:this={askArtist} onasked={asked} onrefused={(a, body) => typeof body.title_id === 'number' && (addedHere = new Map([...addedHere, [a.mbid, body.title_id]]))} />
{/if}

<style>
	.music { position: relative; display: flex; flex-direction: column; gap: 32px; }
	.chips { display: flex; gap: 8px; overflow-x: auto; scrollbar-width: none; margin-bottom: -12px; padding: 2px; }
	.chips::-webkit-scrollbar { display: none; }
	.chip { flex: none; font: inherit; font-size: 14px; font-weight: 500; text-transform: capitalize; padding: 6px 14px; border-radius: 999px; border: 1px solid var(--border, transparent); background: var(--surface-2); color: var(--text); cursor: pointer; }
	.chip.on { background: var(--text); color: var(--surface-1, var(--bg)); border-color: transparent; }
	.chip:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.pick { display: flex; justify-content: flex-end; margin-bottom: -20px; }
	.hero { display: flex; gap: 20px; align-items: center; padding: 16px; border-radius: var(--r-card); background: var(--surface-2); color: var(--text); text-decoration: none; max-width: 640px; }
	.hero:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.cover { flex: none; width: 120px; aspect-ratio: 1; border-radius: 10px; overflow: hidden; background: var(--surface-3); }
	.cover img { width: 100%; height: 100%; object-fit: cover; }
	.words { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
	.eyebrow { font: var(--t-caption, 12px/1.3 inherit); font-weight: 600; letter-spacing: 0.06em; text-transform: uppercase; color: var(--text-muted); }
	.words strong { font-size: 22px; line-height: 1.2; }
	.note { color: var(--text-muted); }
	select { font: inherit; font-size: 13px; padding: 4px 8px; border-radius: 8px; border: 1px solid var(--border, transparent); background: var(--surface-2); color: var(--text); max-width: 160px; }
	@media (max-width: 640px) {
		.hero { gap: 14px; padding: 12px; }
		.cover { width: 88px; }
		.words strong { font-size: 18px; }
	}
	@media (max-width: 767px) { .music { gap: 26px; } }
</style>
