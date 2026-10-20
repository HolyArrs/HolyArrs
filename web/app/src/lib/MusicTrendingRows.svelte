<script lang="ts">
	import { onMount } from 'svelte';
	import { api, message } from './api.ts';
	import { to } from './paths.ts';
	import { toasts } from './toasts.svelte.ts';
	import { refreshInbox } from './wishlist.svelte.ts';
	import { matchOwned, type OwnedIndex } from './explore.ts';
	import { defaultRegion } from './top10.ts';
	import { fetchMusicTrend, musicTrendCards, newFromYourArtists, type MusicTrendCard, type NewAlbum } from './music-trending.ts';
	import { ymd } from './calendar.ts';
	import type { CalendarEntry } from './types.ts';
	import type { Ask, LibraryFolder, LookupArtist, MusicSettings, PickRulesView, TitleView } from './types.ts';
	import Shelf from './ui/Shelf.svelte';
	import PosterCard from './ui/PosterCard.svelte';
	import IconButton from './ui/IconButton.svelte';
	import AddArtistDialog from './AddArtistDialog.svelte';
	import AskArtistDialog from './AskArtistDialog.svelte';

	// Explore's Music rows, personal first: one hero (the newest album from
	// an artist you have), New from artists you have, then the charts:
	// Trending artists (ListenBrainz) and Top albums (Deezer, per country).
	// Every row says why it is there; artists are round, albums square.
	const KEY = 'holyarrs.music-trending';
	let {
		owned,
		manage,
		mayAsk,
		folders,
		rules,
		onadded
	}: { owned: OwnedIndex; manage: boolean; mayAsk: boolean; folders: LibraryFolder[]; rules: PickRulesView[]; onadded?: (t: TitleView) => void } = $props();

	let region = $state('global');
	let regions = $state<string[]>([]);
	let artists = $state<MusicTrendCard[]>([]);
	let albums = $state<MusicTrendCard[]>([]);
	let fresh = $state<NewAlbum[]>([]);
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
		const [ar, al] = await Promise.all([fetchMusicTrend('artist', region, api, signal), fetchMusicTrend('album', region, api, signal)]);
		if (signal.aborted) return;
		artists = ar ? musicTrendCards(ar) : [];
		albums = al ? musicTrendCards(al) : [];
		const got = ar ?? al;
		if (got?.regions?.length) regions = got.regions;
		if (got && got.region !== region) region = got.region;
	}

	function pick() {
		try {
			localStorage.setItem(KEY, region);
		} catch {
			/* the choice is a convenience */
		}
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
		const now = new Date();
		api.get<{ dates: CalendarEntry[] | null }>(`/calendar?from=${ymd(new Date(now.getTime() - 60 * 86400000))}&to=${ymd(now)}`)
			.then((r) => (fresh = newFromYourArtists(r.dates ?? [], now)))
			.catch(() => {});
		if (manage) api.get<MusicSettings>('/settings/music').then((r) => (defaults = r)).catch(() => {});
		return () => gone?.abort();
	});

	function live(c: MusicTrendCard): MusicTrendCard {
		const id = addedHere.get(c.artistMbid);
		if (id !== undefined) return { ...c, titleId: id, status: 'in_library' };
		return matchOwned(c, owned) as MusicTrendCard;
	}

	// An artist in the Library opens its page; any other card opens the
	// artist search, where it can be added or asked for.
	function href(c: MusicTrendCard): string {
		if (c.titleId !== undefined) return to(`/library/${c.titleId}`);
		return to(`/search?q=${encodeURIComponent(c.artist)}&kind=music`);
	}

	/** quick looks the artist up by name and opens the add (or ask) sheet for the matching MusicBrainz id. */
	async function quick(c: MusicTrendCard) {
		if (busy) return;
		busy = c.key;
		try {
			const r = await api.get<{ artists: LookupArtist[] }>(`/artists/lookup?term=${encodeURIComponent(c.artist)}`);
			const a = r.artists?.find((x) => x.mbid === c.artistMbid);
			if (!a) return void toasts.push(`Could not find ${c.artist}. Try Search.`, 'fail');
			if (a.title_id) {
				addedHere = new Map([...addedHere, [c.artistMbid, a.title_id]]);
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
		else if (r.joined) toasts.push(`Someone asked for ${r.ask.name} already; you are on the ask too.`, 'ok');
		else toasts.push(`Asked for ${r.ask.name}. You will hear when it is approved.`, 'ok');
		void refreshInbox();
	}

	const where = $derived(regionName(region));
</script>

{#snippet cards(list: MusicTrendCard[])}
	{#each list.map(live) as c (c.key)}
		<PosterCard title={c.name} href={href(c)} image={c.image} kind="artist" subtitle={c.subtitle} status={c.status} shape={c.shape} actionOnHover>
			{#snippet action()}
				{#if c.titleId === undefined && (manage || mayAsk)}
					<IconButton icon="plus" label={manage ? `Add ${c.artist}` : `Ask for ${c.artist}`} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={busy === c.key || undefined} onclick={() => void quick(c)} />
				{/if}
			{/snippet}
		</PosterCard>
	{/each}
{/snippet}

{#if fresh.length || artists.length || albums.length}
	<section class="music-trending" data-testid="music-trending">
		{#if fresh.length}
			{@const hero = fresh[0]}
			<a class="hero" href={to(`/library/${hero.titleId}`)} data-testid="music-hero">
				<span class="cover">{#if hero.image}<img src={hero.image} alt="" loading="lazy" />{/if}</span>
				<span class="words">
					<span class="eyebrow">New album</span>
					<strong>{hero.album}</strong>
					<span class="note">{hero.artist} · out {new Date(hero.date).toLocaleDateString(undefined, { day: 'numeric', month: 'short' })}</span>
				</span>
			</a>
			{#if fresh.length > 1}
				<Shelf title="New from artists you have" subtitle="Albums out in the last 60 days" item="var(--explore-item)">
					{#each fresh.slice(1) as a (a.key)}
						<PosterCard title={a.album} href={to(`/library/${a.titleId}`)} image={a.image} kind="artist" subtitle={a.artist} shape="square" />
					{/each}
				</Shelf>
			{/if}
		{/if}
		{#if regions.length > 1}
			<div class="pick">
				<select aria-label="Region" bind:value={region} onchange={pick}>
					{#each regions as r (r)}<option value={r}>{regionName(r)}</option>{/each}
				</select>
			</div>
		{/if}
		{#if artists.length}
			<Shelf title="Trending artists" subtitle={`Most played this week · ${where}`} item="var(--explore-item)">{@render cards(artists)}</Shelf>
		{/if}
		{#if albums.length}
			<Shelf title="Top albums" subtitle={`Chart this week · ${where}`} item="var(--explore-item)">{@render cards(albums)}</Shelf>
		{/if}
	</section>
{/if}

{#if manage}
	<AddArtistDialog bind:this={addArtist} {folders} {rules} {defaults} onadded={added} />
{:else if mayAsk}
	<AskArtistDialog bind:this={askArtist} onasked={asked} onrefused={(a, body) => typeof body.title_id === 'number' && (addedHere = new Map([...addedHere, [a.mbid, body.title_id]]))} />
{/if}

<style>
	.music-trending { position: relative; display: flex; flex-direction: column; gap: 32px; }
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
	@media (max-width: 767px) { .music-trending { gap: 26px; } }
</style>
