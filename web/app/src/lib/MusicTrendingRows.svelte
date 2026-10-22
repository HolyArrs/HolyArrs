<script lang="ts">
	import Chip from './ui/Chip.svelte';
	import { onMount } from 'svelte';
	import { api, message } from './api.ts';
	import { toasts } from './toasts.svelte.ts';
	import { refreshInbox } from './wishlist.svelte.ts';
	import type { OwnedIndex } from './explore.ts';
	import { defaultRegion } from './region.ts';
	import { coverFor } from './music-pages.ts';
	import { fetchMusicExplore, musicCardHref, musicCardNote, musicImage, releaseWords, type MusicCardData, type MusicExploreAnswer } from './music-trending.ts';
	import type { Ask, LibraryFolder, LookupArtist, MusicSettings, PickRulesView, TitleView } from './types.ts';
	import Shelf from './ui/Shelf.svelte';
	import PosterCard from './ui/PosterCard.svelte';
	import IconButton from './ui/IconButton.svelte';
	import EmptyState from './ui/EmptyState.svelte';
	import AddArtistDialog from './AddArtistDialog.svelte';
	import AskArtistDialog from './AskArtistDialog.svelte';
	import { EAGER_ROWS, stableRows } from './row-window.ts';

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
	/** The rows' order as first shown this visit (per genre and region), so later answers never move them. */
	let order: string[] = [];
	let orderFor = '';
	/** Whether the first answer had a hero: its space stays (or stays away) for the visit. */
	let heroSlot: boolean | null = null;
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
			const at = `${genre}|${region}`;
			if (at !== orderFor) {
				order = [];
				orderFor = at;
			}
			const kept = stableRows(order, a.rows, (r) => r.id);
			order = kept.order;
			if (heroSlot === null) heroSlot = !!a.hero;
			const keepHero = heroSlot && !a.hero && answer?.hero;
			answer = { ...a, rows: kept.rows, hero: heroSlot ? (a.hero ?? answer?.hero ?? null) : null, hero_alts: keepHero ? answer?.hero_alts : a.hero_alts };
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

	/** The hero candidates: the newest album first, then its stand-ins. */
	const heroes = $derived(answer?.hero ? [answer.hero, ...(answer.hero_alts ?? [])] : []);
	/** The hero shown: the first candidate whose cover loads, else the first one over its artist's picture. */
	let heroPick = $state<{ card: MusicCardData; cover: boolean } | null>(null);
	let heroFor = '';
	function loads(src: string | null): Promise<boolean> {
		if (!src) return Promise.resolve(false);
		return new Promise((done) => {
			const im = new Image();
			im.onload = () => done(im.naturalWidth > 0);
			im.onerror = () => done(false);
			im.src = src;
		});
	}
	$effect(() => {
		const list = heroes;
		const at = list.map((c) => c.key).join('|');
		if (at === heroFor) return;
		heroFor = at;
		if (!list.length) return void (heroPick = null);
		heroPick = null;
		void (async () => {
			for (const c of list) {
				if (await loads(musicImage(c))) {
					if (heroFor === at) heroPick = { card: c, cover: true };
					return;
				}
				if (heroFor !== at) return;
			}
			if (heroFor === at) heroPick = { card: list[0], cover: false };
		})();
	});
	const hero = $derived(heroPick?.card ?? null);
	const heroNote = $derived.by(() => {
		if (!hero) return '';
		if (!hero.title_id) return `${hero.artist} · number ${hero.rank ?? 1} on this week's album chart`;
		return ['From an artist you have', hero.artist, hero.date ? releaseWords(hero.date) : ''].filter(Boolean).join(' · ');
	});
	const regions = $derived(answer?.regions ?? []);
	/** The row whose header carries the chart region: Popular in <country>, else Top albums. */
	const pickRow = $derived(regions.length > 1 ? ((answer?.rows.find((r) => r.type === 'popular_country') ?? answer?.rows.find((r) => r.type === 'top_albums'))?.id ?? '') : '');
</script>

{#if answer?.kid}
	<EmptyState icon="music" title="Music is not shown on this account" text="Music has no age ratings, so it stays off for accounts with an age limit." />
{:else if answer || !failed}
	<section class="music" data-testid="music-trending" aria-busy={!answer || undefined}>
		<!-- The chip bar keeps its height: placeholder chips until the genres are known. -->
		{#if chips.length}
			<div class="chips" role="group" aria-label="Genres" data-testid="music-genres">
				<!-- The same chips as the Movies and Shows genre picker. -->
				<Chip label="All" selected={genre === ''} onclick={() => chip('')} />
				{#each chips as g (g)}
					<span class="cap"><Chip label={g} selected={genre === g} onclick={() => chip(g)} /></span>
				{/each}
			</div>
		{:else}
			<div class="chips" aria-hidden="true">{#each [52, 64, 88, 72, 96, 60] as w (w)}<span class="chip sk-chip" style:width="{w}px"></span>{/each}</div>
		{/if}
		{#if !answer || (heroes.length && !heroPick)}
			<div class="hero sk-hero" aria-hidden="true"><span class="cover"></span></div>
		{/if}
		{#if !answer}
			{#each [0, 1] as r (r)}
				<div class="sk-shelf" aria-hidden="true"><span class="sk-h"></span><div class="sk-tiles">{#each Array.from({ length: 8 }, (_, i) => i) as i (i)}<span class="sk-tile"></span>{/each}</div></div>
			{/each}
		{/if}

		{#if answer && hero && heroPick}
			<a class="hero" class:bare={!heroPick.cover} href={musicCardHref(hero)} data-testid="music-hero">
				{#if heroPick.cover}
					{@const hc = coverFor(musicImage(hero), 120)}
					<span class="cover"><img src={hc.src} srcset={hc.srcset} sizes="120px" alt="" /></span>
				{:else if hero.artist_image}
					<!-- No album cover anywhere: the artist's picture, blurred, behind the title. -->
					<img class="backdrop" src={musicImage({ image: hero.artist_image })} alt="" aria-hidden="true" onerror={(e) => ((e.currentTarget as HTMLImageElement).hidden = true)} />
				{/if}
				<span class="words">
					<span class="eyebrow">New album</span>
					<strong>{hero.name}</strong>
					<span class="note">{heroNote}</span>
				</span>
			</a>
		{/if}

		{#each answer?.rows ?? [] as row, ri (row.id)}
			<div class="row">
			{#if row.id === pickRow}
				<!-- The chart region sits in its row's header. -->
				<div class="pick">
					<select aria-label="Chart region" bind:value={region} onchange={pick}>
						{#each regions as r (r)}<option value={r}>{regionName(r)}</option>{/each}
					</select>
				</div>
			{/if}
			<Shelf title={row.type === 'popular_country' ? `Popular in ${regionName(row.id.split(':')[1] ?? '')}` : row.title} subtitle={row.type === 'popular_country' ? `This week's top albums in ${regionName(row.id.split(':')[1] ?? '')}` : row.reason} item="var(--explore-item)"
				items={row.items} key={(c) => c.key} defer={ri >= EAGER_ROWS} placeholder={row.shape === 'square' ? 'square' : 'round'} lines={row.items.some((c) => musicCardNote(row, c)) ? 2 : 1}>
				{#snippet card(raw: MusicCardData)}
					{@const c = live(raw)}
					{@const cv = coverFor(musicImage(c), 200)}
					<PosterCard
						title={c.name}
						href={musicCardHref(c)}
						image={cv.src}
						srcset={cv.srcset}
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
				{/snippet}
			</Shelf>
			</div>
		{/each}

		{#if answer && answer.rows.length === 0 && !hero}
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
	.chips :global(.ui-chip) { flex: none; }
	.cap { display: contents; text-transform: capitalize; }
	.chip { flex: none; font: inherit; font-size: 14px; font-weight: 500; text-transform: capitalize; padding: 6px 14px; border-radius: 999px; border: 1px solid var(--border, transparent); background: var(--surface-2); color: var(--text); cursor: pointer; }
	.chips { height: 38px; flex: none; }
	.chip { height: 34px; box-sizing: border-box; line-height: 20px; }
	.sk-chip { display: block; padding: 0; border-color: transparent; cursor: default; }
	.sk-hero { height: 152px; box-sizing: border-box; }
	/* A loading row: the header and one row of square cards, at the real row's height. */
	.sk-shelf { display: flex; flex-direction: column; gap: 12px; overflow: hidden; }
	.sk-h { display: block; width: 220px; height: 46px; border-radius: 8px; background: var(--surface-2); }
	.sk-tiles { display: grid; grid-auto-flow: column; grid-auto-columns: var(--explore-item); gap: var(--shelf-gap); padding: 6px 0 14px; }
	.sk-tile { display: block; aspect-ratio: 1; margin-bottom: 45px; border-radius: var(--r-card); background: var(--surface-2); }
	.row { position: relative; min-width: 0; }
	.pick { position: absolute; right: 0; top: 0; z-index: 1; display: flex; gap: 8px; align-items: center; }
	.hero { position: relative; overflow: hidden; }
	.hero.bare { justify-content: flex-start; }
	.backdrop { position: absolute; inset: -24px; width: calc(100% + 48px); height: calc(100% + 48px); object-fit: cover; filter: blur(22px) brightness(0.55) saturate(1.2); transform: scale(1.05); }
	.hero.bare .words { position: relative; color: #fff; }
	.hero.bare .eyebrow, .hero.bare .note { color: rgb(255 255 255 / 0.8); }
	.hero { box-sizing: border-box; height: 152px; display: flex; gap: 20px; align-items: center; padding: 16px; border-radius: var(--r-card); background: var(--surface-2); color: var(--text); text-decoration: none; max-width: 640px; }
	.hero:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.cover { flex: none; width: 120px; aspect-ratio: 1; border-radius: 10px; overflow: hidden; background: var(--surface-3); }
	.cover img { width: 100%; height: 100%; object-fit: cover; }
	.words { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
	.eyebrow { font: var(--t-caption, 12px/1.3 inherit); font-weight: 600; letter-spacing: 0.06em; text-transform: uppercase; color: var(--text-muted); }
	.words strong { font-size: 22px; line-height: 1.2; }
	.note { color: var(--text-muted); }
	select { font: inherit; font-size: 13px; padding: 4px 8px; border-radius: 8px; border: 1px solid var(--border, transparent); background: var(--surface-2); color: var(--text); max-width: 160px; }
	@media (max-width: 640px) {
		.hero, .sk-hero { gap: 14px; padding: 12px; height: 112px; }
		.cover { width: 88px; }
		.words strong { font-size: 18px; }
		.pick { position: static; margin-bottom: 8px; justify-content: flex-end; }
	}
	@media (max-width: 767px) { .music { gap: 26px; } }
</style>
