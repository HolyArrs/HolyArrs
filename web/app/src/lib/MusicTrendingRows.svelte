<script lang="ts">
	import Chip from './ui/Chip.svelte';
	import { onMount } from 'svelte';
	import { api, message } from './api.ts';
	import { toasts } from './toasts.svelte.ts';
	import { refreshInbox } from './wishlist.svelte.ts';
	import type { OwnedIndex } from './explore.ts';
	import { defaultRegion } from './region.ts';
	import { coverFor } from './music/music-pages.ts';
	import { heroArtists, heroLine } from './music/page-hero.ts';
	import { isNewRelease } from './music/music-new.ts';
	import { fetchMusicExplore, fetchMusicExploreMore, fetchMusicPage, genreHref, musicCardHref, musicCardNote, musicImage, musicStatus, releaseWords, songAskHref, type MusicCardData, type MusicExploreAnswer, type MusicPageKind } from './music/music-trending.ts';
	import MusicGenreTiles from './ui/MusicGenreTiles.svelte';
	import MusicBrowseGrid from './ui/MusicBrowseGrid.svelte';
	import MusicVideoRow from './ui/MusicVideoRow.svelte';
	import { to } from './paths.ts';
	import SongLine from './ui/SongLine.svelte';
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
		onadded,
		slug = '',
		kind = 'genre',
		genres = $bindable([])
	}: { owned: OwnedIndex; manage: boolean; mayAsk: boolean; folders: LibraryFolder[]; rules: PickRulesView[]; onadded?: (t: TitleView) => void; /** A Music genre page: that genre's rows instead of Explore's. */ slug?: string; /** The page kind for slug (genre, mood, season, decade, activity). */ kind?: MusicPageKind; /** The genre tiles, for Explore's Genres button. */ genres?: StripGenre[] } = $props();
	type StripGenre = { key: string; name: string; href: string; art: string; images?: string[]; hue: number };

	let region = $state('global');
	let genre = $state('');
	let answer = $state<MusicExploreAnswer | null>(null);
	// Genres show as the Movies and Shows category strip under the hero, not as a row.
	const stripGenres = $derived<StripGenre[]>((answer?.rows ?? []).filter((r) => r.shape === 'tile').flatMap((r) => r.items).map((c) => ({ key: c.key, name: c.name, href: genreHref(c), art: c.image ? (musicImage(c) ?? '') : '', hue: c.hue ?? 0, images: c.images ?? [] })));
	$effect(() => { genres = stripGenres.length ? stripGenres : browseTiles.map((t) => ({ key: t.key, name: t.name, href: t.href, art: t.cover, hue: t.hue, images: t.cover ? [t.cover] : [] })); });
	// music_lists_v1: the genres row comes as the Browse grid (colour tiles with a tilted cover).
	const browseTiles = $derived((answer?.rows ?? []).filter((r) => r.shape === 'grid').flatMap((r) => r.items).map((c) => ({ key: c.key, name: c.name, href: c.href ? to(c.href) : genreHref(c), hue: c.hue ?? 0, cover: c.cover || (c.image ? (musicImage(c) ?? '') : '') })));
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
	/** music_rows_v7: the next rows' cursor, whether a batch is in flight, and batches read this visit. */
	let next = $state('');
	let more = $state(false);
	let batches = 0;
	let sentinel: HTMLDivElement | undefined = $state();

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
			const a = slug ? await fetchMusicPage(kind, slug, region, api, signal) : await fetchMusicExplore(genre, region, api, signal);
			if (signal.aborted) return;
			const at = `${kind}|${slug}|${genre}|${region}`;
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
			// music_rows_v7: read again the batches already shown (the plan is fixed for the day).
			next = a.next ?? '';
			const had = batches;
			batches = 0;
			while (next && batches < had && !signal.aborted) await moreRows(signal);
			// Chips come from the unfiltered answer, so picking one keeps the rest.
			if (!genre) chips = a.genres ?? [];
			if (a.region && a.region !== region) region = a.region;
		} catch {
			if (!signal.aborted) failed = true;
		}
	}

	/** moreRows appends the next 6 rows of the day's plan (one batch in flight). */
	async function moreRows(signal = gone?.signal) {
		if (!next || more || !answer) return;
		more = true;
		try {
			const m = await fetchMusicExploreMore(next, region, api, signal);
			if (signal?.aborted || !answer) return;
			const have = new Set(answer.rows.map((r) => r.id));
			const rows = [...answer.rows, ...m.rows.filter((r) => !have.has(r.id))];
			order = rows.map((r) => r.id);
			answer = { ...answer, rows };
			next = m.next ?? '';
			batches++;
		} catch {
			// A cursor from an older plan (restart, new day): start over from page 1.
			if (!signal?.aborted) {
				next = '';
				batches = 0;
				void load();
			}
		} finally {
			more = false;
		}
	}

	// One viewport before the end, read the next batch.
	$effect(() => {
		if (!sentinel || !next || typeof IntersectionObserver === 'undefined') return;
		const io = new IntersectionObserver((e) => e.some((x) => x.isIntersecting) && void moreRows(), { rootMargin: '0px 0px 100% 0px' });
		io.observe(sentinel);
		return () => io.disconnect();
	});

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
			if (!slug && !genre && chips.length === 0) void load();
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

	/** The Spotlight row (Apple "New" style): the hero first, then its stand-ins. */
	const heroes = $derived(answer?.hero ? [answer.hero, ...(answer.hero_alts ?? [])] : []);
	/** The page hero's collage: the Spotlight and first row's album covers (already stored). */
	const heroCovers = $derived(
		[...heroes, ...(answer?.rows[0]?.items ?? [])]
			.filter((c) => c.kind === 'album' && c.image)
			.slice(0, 5)
			.map((c) => coverFor(musicImage(c), 250).src as string)
	);
	const bandHue = $derived(((((answer?.tile?.hue ?? 0) % 24) + 24) % 24) * 15);
	const bandKind = $derived(({ genre: 'Genre', mood: 'Mood', season: 'Season', decade: 'Decade', activity: 'Activity' } as Record<string, string>)[(answer?.tile?.key ?? 'genre').split(':')[0]] ?? 'Genre');
	/** Spotlight covers that failed to load: the card falls back to its artist's picture. */
	let noCover = $state<Record<string, boolean>>({});
	/** Spotlight artist photos that loaded at a usable size (400 px wide or more). */
	let photoOk = $state<Record<string, boolean>>({});
	function kicker(c: MusicCardData): string {
		// "New album" only within 60 days of release; otherwise the real reason.
		if (c.kicker && (!/^new\b/i.test(c.kicker) || isNewRelease(c.date))) return c.kicker;
		return isNewRelease(c.date) ? 'New album' : c.rank ? 'Top album' : 'Popular now';
	}
	function caption(c: MusicCardData): string {
		if (!c.title_id) return c.rank ? `Number ${c.rank} on this week's album chart` : '';
		return ['From an artist you have', c.date ? releaseWords(c.date) : ''].filter(Boolean).join(' · ');
	}
	const regions = $derived(answer?.regions ?? []);
	/** The row whose header carries the chart region: Popular in <country>, else Top albums. */
	const pickRow = $derived(regions.length > 1 ? ((answer?.rows.find((r) => r.type === 'popular_country') ?? answer?.rows.find((r) => r.type === 'top_albums'))?.id ?? '') : '');
</script>

{#if answer?.kid}
	<EmptyState icon="music" title="Music is not shown on this account" text="Music has no age ratings, so it stays off for accounts with an age limit." />
{:else if answer || !failed}
	<section class="music" class:v2={answer?.v2} data-testid="music-trending" aria-busy={!answer || undefined}>
		{#if slug}
			<!-- A genre page's band: its tile's hue and grey art, the genre's name. -->
			<!-- The page hero: the tile's hue as a gradient behind a tilted collage of the page's top covers (preloaded), its words and leading artists, one stat line. -->
			<div class="band" style:--hue={`hsl(${bandHue} 52% 36%)`} style:--hue2={`hsl(${(bandHue + 40) % 360} 60% 18%)`} data-testid="music-genre-band">
				<div class="collage" aria-hidden="true">
					{#each heroCovers as src (src)}<img {src} alt="" decoding="async" onerror={(e) => ((e.currentTarget as HTMLImageElement).hidden = true)} />{/each}
				</div>
				<div class="btext">
					<span class="bkind">{bandKind}</span>
					<h1>{answer?.tile?.name ?? ''}</h1>
					{#if answer?.rows.length}
						<p class="bline">{heroLine(answer?.tile?.key ?? '', answer?.tile?.name ?? '', heroArtists(heroes, answer.rows))}</p>
						<p class="bstat">{answer.rows.length} rows · updated today{#if heroes[0]} · Top pick: {heroes[0].name}{/if}</p>
					{/if}
				</div>
			</div>
		{/if}
		<!-- The chip bar keeps its height: placeholder chips until the genres are known. The v2 rows have genre tiles instead. -->
		{#if answer?.v2}
			<!-- no chips -->
		{:else if chips.length}
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
		{#if !answer}
			<div class="sk-spot" aria-hidden="true">{#each [0, 1, 2, 3] as i (i)}<span class="sk-card"><span class="sk-k"></span><span class="sk-t"></span><span class="sk-img"></span></span>{/each}</div>
		{/if}
		{#if !answer}
			{#each [0, 1] as r (r)}
				<div class="sk-shelf" aria-hidden="true"><span class="sk-h"></span><div class="sk-tiles">{#each Array.from({ length: 8 }, (_, i) => i) as i (i)}<span class="sk-tile"></span>{/each}</div></div>
			{/each}
		{/if}

		{#if answer && heroes.length}
			<div class="spot" data-testid="music-hero">
				<Shelf title="Spotlight" item="var(--spot-item)">
					{#each heroes as c (c.key)}
						{@const cv = coverFor(musicImage(c), 430)}
						{@const art = c.artist_image ? musicImage({ image: c.artist_image }) : null}
						<a class="feature" href={musicCardHref(c)}>
							<span class="kicker">{kicker(c)}</span>
							<strong class="ftitle">{c.name}</strong>
							<span class="fsub">{c.artist ?? ''}</span>
							<span class="frame">
								{#if cv.src && !noCover[c.key]}
									<!-- The square cover on a blurred, darker copy of itself fills the wide frame. -->
									<img class="blur" src={coverFor(musicImage(c), 120).src} alt="" aria-hidden="true" loading="lazy" decoding="async" />
									<img class="sq" src={cv.src} srcset={cv.srcset} sizes="240px" alt="" loading="lazy" decoding="async" onerror={() => (noCover[c.key] = true)} />
								{/if}
								{#if art}
									<!-- The artist's photo, when there is one, over all of it; the cover then sits small in the corner (as New & Popular). -->
									<img class="photo" class:in={photoOk[c.key]} src={art} alt="" loading="lazy" decoding="async" onload={(e) => ((e.currentTarget as HTMLImageElement).naturalWidth >= 400 && (photoOk[c.key] = true))} />
								{/if}
								{#if photoOk[c.key] && cv.src && !noCover[c.key]}<img class="mini" src={coverFor(musicImage(c), 60).src} alt="" loading="lazy" decoding="async" />{/if}
							</span>
							{#if caption(c)}<span class="cap-below">{caption(c)}</span>{/if}
						</a>
					{/each}
				</Shelf>
			</div>
		{/if}

		{#if stripGenres.length}<MusicGenreTiles genres={stripGenres} allHref={to('/search?kind=music')} />{/if}
		{#if browseTiles.length}<MusicBrowseGrid tiles={browseTiles} allHref={to('/search?kind=music')} />{/if}

		{#each (answer?.rows ?? []).filter((r) => r.shape !== 'tile' && r.shape !== 'grid') as row, ri (row.id)}
			{#if row.shape === 'wide'}
			<MusicVideoRow title={row.title} reason={row.reason} items={row.items} />
			{:else if row.shape === 'song'}
			<section class="row plain" data-testid="music-row-{row.type}">
				<h2 class="rhead">{row.title}</h2>
				<div class="songs">
					{#each row.items as c (c.key)}
						{@const sc = coverFor(c.image ? (c.image.startsWith('/') ? musicImage(c) : c.image) : null, 44)}
						<SongLine title={c.track || c.name} artist={c.artist ?? ''} image={sc.src} explicit={c.explicit} href={songAskHref(c)} askHref={songAskHref(c)} />
					{/each}
				</div>
			</section>
			{:else}
			<div class="row">
			{#if row.id === pickRow}
				<!-- The chart region sits in its row's header. -->
				<div class="pick">
					<select aria-label="Chart region" bind:value={region} onchange={pick}>
						{#each regions as r (r)}<option value={r}>{regionName(r)}</option>{/each}
					</select>
				</div>
			{/if}
			<Shelf title={row.type === 'popular_country' ? `Popular in ${regionName(row.id.split(':')[1] ?? '')}` : row.title} subtitle={row.type === 'popular_country' ? `This week's top albums in ${regionName(row.id.split(':')[1] ?? '')}` : row.reason} item={answer?.v2 && row.shape === 'round' ? 'var(--music-circle, 152px)' : 'var(--explore-item)'}
				items={row.items} key={(c) => c.key} defer={ri >= EAGER_ROWS} placeholder={row.shape === 'square' ? 'square' : 'round'} lines={row.items.some((c) => musicCardNote(row, c)) ? 2 : 1}>
				{#snippet card(raw: MusicCardData)}
					{@const c = live(raw)}
					{@const cv = coverFor(musicImage(c), 200)}
					<PosterCard
						title={c.name}
						href={musicCardHref(c)}
						image={cv.src}
						srcset={cv.srcset}
						fallback={c.kind === 'album' && c.artist_image ? musicImage({ image: c.artist_image }) : null}
						kind="artist"
						subtitle={musicCardNote(row, c) || undefined}
						badges={c.explicit ? ['E'] : []}
						status={answer?.v2 ? (row.type !== 'your_asks' ? musicStatus(c) : undefined) : c.kind === 'artist' && c.title_id && row.type !== 'your_asks' ? 'in_library' : undefined}
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
			{/if}
		{/each}

		{#if next}
			<!-- music_rows_v7: the next batch loads one viewport before the end; skeletons hold its place. -->
			<div class="more" bind:this={sentinel} aria-busy={more || undefined}>
				{#if more}
					{#each [0, 1, 2, 3, 4, 5] as r (r)}
						<div class="sk-shelf" aria-hidden="true"><span class="sk-h"></span><div class="sk-tiles">{#each Array.from({ length: 8 }, (_, i) => i) as i (i)}<span class="sk-tile"></span>{/each}</div></div>
					{/each}
				{/if}
			</div>
		{/if}

		{#if answer?.thin}
			<!-- Row templates: too little music for this page yet; never a near-empty page. -->
			<div class="thin" data-testid="music-thin">
				<p class="thin-title">Not enough music here yet</p>
				{#if answer.similar?.length}
					<p class="thin-text">Try one of these instead:</p>
					<ul>
						{#each answer.similar as s (s.key)}<li><a href={to(s.href)}>{s.name}</a></li>{/each}
					</ul>
				{/if}
			</div>
		{:else if answer && answer.rows.length === 0 && !heroes.length}
			<EmptyState icon="music" title={slug ? `Nothing charting in ${answer.tile?.name ?? 'this genre'} yet` : genre ? `Nothing tagged ${genre} yet` : 'Music rows are getting ready'} text={genre ? 'Pick another genre, or All.' : 'Charts and suggestions fill in within a few minutes.'} />
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
	/* A loading row: the header and one row of square cards, at the real row's height. */
	.sk-shelf { display: flex; flex-direction: column; gap: 12px; overflow: hidden; }
	.sk-h { display: block; width: 220px; height: 46px; border-radius: 8px; background: var(--surface-2); }
	.sk-tiles { display: grid; grid-auto-flow: column; grid-auto-columns: var(--explore-item); gap: var(--shelf-gap); padding: 6px 0 14px; }
	.sk-tile { display: block; aspect-ratio: 1; margin-bottom: 45px; border-radius: var(--r-card); background: var(--surface-2); }
	.row { position: relative; min-width: 0; }
	.more { display: flex; flex-direction: column; gap: 32px; min-height: 1px; }
	/* music_rows_v2: one card size (200), song lines 3 columns. */
	@media (min-width: 768px) { .music.v2 { --explore-item: 200px; } }
	/* nowrap: the global .row utility wraps, and a wrapping column flexbox
	   sizes the tile grid at its 0 px max-content width, so its rows collapse. */
	.plain { display: flex; flex-direction: column; flex-wrap: nowrap; align-items: stretch; gap: 12px; }
	.rhead { margin: 0; font-size: 17px; font-weight: 700; }
	/* A Music genre page's band: the tile's look (hue, grey art at .55, radius 10), wider. */
	.band { position: relative; display: flex; align-items: flex-end; min-height: 260px; padding: 28px 32px; border-radius: 14px; overflow: hidden; background: linear-gradient(135deg, var(--hue), var(--hue2)); color: #fff; isolation: isolate; }
	.band::after { content: ''; position: absolute; inset: 0; z-index: -1; background: linear-gradient(90deg, rgb(0 0 0 / .55) 0%, rgb(0 0 0 / .2) 55%, transparent 100%), linear-gradient(0deg, rgb(0 0 0 / .45), transparent 60%); }
	.collage { position: absolute; z-index: -2; top: 50%; left: 45%; right: -6%; display: flex; justify-content: flex-end; gap: 14px; overflow: hidden; transform: translateY(-50%) rotate(-8deg); -webkit-mask-image: linear-gradient(90deg, transparent 0%, #000 30%); mask-image: linear-gradient(90deg, transparent 0%, #000 30%); padding: 30px 0; }
	.collage img { width: clamp(110px, 14vw, 190px); aspect-ratio: 1; object-fit: cover; border-radius: 8px; box-shadow: 0 12px 30px rgb(0 0 0 / .45); opacity: .9; }
	.collage img:nth-child(even) { transform: translateY(26px); }
	.btext { position: relative; max-width: min(640px, 44%); display: flex; flex-direction: column; gap: 6px; }
	.bkind { font: 600 12px/1 var(--font-ui); letter-spacing: .08em; text-transform: uppercase; opacity: .8; }
	.band h1 { margin: 0; font-size: clamp(34px, 5vw, 60px); font-weight: 800; line-height: 1.02; letter-spacing: -.02em; text-shadow: 0 2px 8px rgb(0 0 0 / .35); }
	.bline { margin: 4px 0 0; font: 500 clamp(14px, 1.2vw, 17px)/1.4 var(--font-ui); opacity: .95; }
	.bstat { margin: 0; font: 400 13px/1.3 var(--font-ui); opacity: .75; }
	@media (max-width: 600px) {
		.band { min-height: 300px; padding: 20px; }
		.collage { top: 8%; left: auto; right: -18%; transform: rotate(-8deg); -webkit-mask-image: none; mask-image: none; padding: 0; overflow: visible; }
		.collage img { width: 96px; }
		.btext { max-width: 100%; }
	}
	.songs { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 0 16px; }
	@media (max-width: 1023px) { .songs { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
	@media (max-width: 640px) { .songs { grid-template-columns: minmax(0, 1fr); } }
	.pick { position: absolute; right: 0; top: 0; z-index: 1; display: flex; gap: 8px; align-items: center; }
	/* Spotlight: Apple "New"-style feature cards, label and title above a wide 16:9 picture. */
	.music { --spot-item: min(86vw, 430px); }
	.spot :global(.ui-shelf-head), .spot :global(header) { margin-bottom: 2px; }
	.feature { display: flex; flex-direction: column; min-width: 0; color: var(--text); text-decoration: none; border-radius: 14px; }
	.feature:focus-visible { outline: none; }
	.feature:focus-visible .frame { box-shadow: var(--focus-ring); }
	.kicker { font: 700 11px/1.2 var(--font-ui); letter-spacing: .08em; text-transform: uppercase; color: var(--text-muted); }
	.ftitle { margin-top: 4px; font-size: 20px; line-height: 1.2; font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.fsub { font-size: 15px; line-height: 1.3; color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.frame { position: relative; display: block; margin-top: 10px; aspect-ratio: 4 / 3; border-radius: 14px; overflow: hidden; isolation: isolate; background: var(--surface-2); transition: transform .35s cubic-bezier(.16, 1, .3, 1), box-shadow .35s; }
	.feature:hover .frame { transform: translateY(-2px); box-shadow: 0 14px 28px -16px rgb(0 0 0 / .7); }
	.blur { position: absolute; inset: 0; width: 100%; height: 100%; transform: scale(1.35); object-fit: cover; filter: blur(28px) saturate(1.3) brightness(.7); z-index: -1; }
	.sq { position: absolute; top: 50%; left: 50%; height: 78%; aspect-ratio: 1; translate: -50% -50%; object-fit: cover; border-radius: 8px; box-shadow: 0 12px 30px -8px rgb(0 0 0 / .65); }
	.photo { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; object-position: center 25%; opacity: 0; transition: opacity .3s ease; }
	.photo.in { opacity: 1; }
	.mini { position: absolute; right: 12px; bottom: 10px; z-index: 1; width: 56px; height: 56px; border-radius: 4px; object-fit: cover; box-shadow: 0 2px 10px rgb(0 0 0 / .45); }
	.cap-below { display: block; margin-top: 8px; color: var(--text-muted); font-size: 13px; line-height: 1.3; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.sk-spot { display: grid; grid-auto-flow: column; grid-auto-columns: var(--spot-item); gap: var(--shelf-gap, 16px); overflow: hidden; padding-top: 58px; }
	.sk-card { display: flex; flex-direction: column; gap: 6px; }
	.sk-k, .sk-t, .sk-img { display: block; border-radius: 6px; background: var(--surface-2); }
	.sk-k { width: 80px; height: 12px; }
	.sk-t { width: 60%; height: 40px; }
	.sk-img { aspect-ratio: 4 / 3; border-radius: 14px; margin-top: 4px; }
	@media (prefers-reduced-motion: reduce) { .frame { transition: none; } .feature:hover .frame { transform: none; } }
	select { font: inherit; font-size: 13px; padding: 4px 8px; border-radius: 8px; border: 1px solid var(--border, transparent); background: var(--surface-2); color: var(--text); max-width: 160px; }
	@media (max-width: 640px) {
		.ftitle { font-size: 18px; }
		.pick { position: static; margin-bottom: 8px; justify-content: flex-end; }
	}
	@media (max-width: 767px) { .music { gap: 26px; } }
	.thin { display: flex; flex-direction: column; gap: 8px; padding: 24px 0; }
	.thin-title { font: var(--t-title3); margin: 0; }
	.thin-text { margin: 0; color: var(--text-2, inherit); }
	.thin ul { display: flex; flex-wrap: wrap; gap: 8px; list-style: none; margin: 0; padding: 0; }
	.thin a { display: inline-flex; align-items: center; min-height: 44px; padding: 0 16px; border-radius: 999px; background: var(--surface-2, rgba(127, 127, 127, .15)); color: inherit; text-decoration: none; }
	.thin a:hover { text-decoration: underline; }
</style>
