<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { dragScroll } from '#lib/ui/drag-scroll.ts';
	import { Orbit, SkRail } from '#lib/ui/sk/index.ts';
	import Shelf from '#lib/ui/Shelf.svelte';
	import { defaultRegion } from '#lib/ui/region.ts';
	import PosterCard from '#lib/ui/PosterCard.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import { fetchRisingToday, musicCardHref, musicImage, type MusicCardData, type MusicRowData } from './music-trending.ts';
	import { coverFor } from './music-pages.ts';
	import { cardBadges, eyebrow, heroNote, newMusicPath, rowNote, type NewMusicAnswer, type NewMusicCard } from './music-new.ts';
	import type { ArtistSheets, LookupArtist, MusicSettings } from './types.ts';
	import MusicVideoRow from './MusicVideoRow.svelte';
	import MusicBrowseGrid from './MusicBrowseGrid.svelte';
	import { fetchBrowseTiles, type BrowseTileData } from './browse-tiles.ts';
	import { onMount } from 'svelte';
	import { api, message } from '../core/api.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import { refreshInbox } from '../core/inbox.svelte.ts';
	import type { Ask, LibraryFolder, PickRulesView, TitleView } from '../core/types.ts';
	import { to } from '../core/paths.ts';

	// New & Popular > Music: a shelf of large editorial cards for the week's biggest new albums, a compact list of fresh tracks, then album rows. Every action asks for (or adds) music; nothing here plays.

	let { manage, mayAsk, folders, rules, sheets }: { manage: boolean; mayAsk: boolean; folders: LibraryFolder[]; rules: PickRulesView[]; sheets: ArtistSheets } = $props();

	let answer = $state<NewMusicAnswer | null>(null);
	let failed = $state(false);
	let busy = $state('');
	let addedHere = $state(new Map<string, number>());
	let follows = $state(new Map<string, boolean>());
	let defaults = $state<MusicSettings | null>(null);
	let addArtist: ReturnType<ArtistSheets['add']> | undefined = $state();
	let askArtist: ReturnType<ArtistSheets['ask']> | undefined = $state();
	const gone = new AbortController();
	let poll: ReturnType<typeof setTimeout> | undefined;

	// Rising today: asked with the page's rows and shown with them.
	let rising = $state<MusicRowData | null>(null);
	const risingAsk = fetchRisingToday(api, gone.signal);

	async function load(tries = 0) {
		try {
			const a = await api.get<NewMusicAnswer>(`${newMusicPath}?region=${encodeURIComponent(defaultRegion())}`, gone.signal);
			if (!tries) rising = await risingAsk;
			answer = a;
			failed = false;
			if (a.building && tries < 30) poll = setTimeout(() => void load(tries + 1), 3000);
		} catch {
			if (!gone.signal.aborted) failed = true;
		}
	}

	onMount(() => {
		void load();
		if (manage) api.get<MusicSettings>('/settings/music').then((r) => (defaults = r)).catch(() => {});
		return () => {
			clearTimeout(poll);
			gone.abort();
		};
	});

	function live(c: NewMusicCard): NewMusicCard {
		const id = c.artist_mbid ? addedHere.get(c.artist_mbid) : undefined;
		const f = follows.get(c.mbid);
		return { ...c, title_id: c.title_id ?? id, following: f ?? c.following };
	}
	const canAct = (c: NewMusicCard) => !!c.artist_mbid && (manage || mayAsk) && !c.upcoming;

	/** Finds the artist and opens the add sheet, or the ask sheet with this album picked. */
	async function quick(c: NewMusicCard) {
		const artist = c.artist || c.name;
		if (busy || !c.artist_mbid) return;
		busy = c.key;
		try {
			const r = await api.get<{ artists: LookupArtist[] }>(`/artists/lookup?term=${encodeURIComponent(`mbid:${c.artist_mbid}`)}`);
			const a = r.artists?.find((x) => x.mbid === c.artist_mbid);
			if (!a) return void toasts.push(t('music.couldnTFind', { name: artist }), 'fail', { body: t('music.trySearch') });
			if (a.title_id && manage) addedHere = new Map([...addedHere, [a.mbid, a.title_id]]);
			if (manage && !a.title_id) addArtist?.open(a);
			else if (manage) toasts.push(t('music.alreadyInLibrary', { name: a.name }), 'ok', { body: t('music.addedOnNextRefresh') });
			else askArtist?.open(a, c.mbid);
		} catch (e) {
			toasts.push(t('music.couldnTAddArtist'), 'fail', { body: message(e) });
		} finally {
			busy = '';
		}
	}

	async function follow(c: NewMusicCard) {
		const on = !live(c).following;
		follows = new Map([...follows, [c.mbid, on]]);
		try {
			const path = `/metadata/album/${c.mbid}/follow`;
			if (on) await api.put(path, { name: c.name, artist: c.artist ?? '', release_date: c.date ?? '' });
			else await api.del(path);
		} catch (e) {
			follows = new Map([...follows, [c.mbid, !on]]);
			toasts.push(t('music.couldnTUpdateFollow'), 'fail', { body: message(e) });
		}
	}

	function added(_a: LookupArtist, title: TitleView) {
		addedHere = new Map([...addedHere, [_a.mbid, title.id]]);
		toasts.push(t('music.added', { what: title.name }), 'ok', { body: t('music.fillingInAlbumsAnd') });
	}
	function asked(_a: LookupArtist, r: { ask: Ask; joined: boolean }) {
		if (r.ask.state === 'approved') toasts.push(t('music.approved', { what: r.ask.name }), 'ok', { body: t('music.keptUpToDate') });
		else if (r.ask.state === 'asked' && r.joined) toasts.push(t('music.alreadyRequested', { what: r.ask.name }), 'ok', { body: t('music.youReOnThe') });
		else toasts.push(t('music.requested2', { what: r.ask.name }), 'ok', { body: t('music.waitingForApproval') });
		void refreshInbox();
	}

	// The hero shelf: paddles page one screen at a time; the left one shows once scrolled.
	let shelf = $state<HTMLElement>();
	let atStart = $state(true);
	let atEnd = $state(false);
	function edges() {
		if (!shelf) return;
		atStart = shelf.scrollLeft < 4;
		atEnd = shelf.scrollLeft + shelf.clientWidth >= shelf.scrollWidth - 4;
	}
	function page(dir: 1 | -1, el: HTMLElement | undefined = shelf) {
		if (!el) return;
		const still = matchMedia('(prefers-reduced-motion: reduce)').matches;
		el.scrollBy({ left: dir * el.clientWidth, behavior: still ? 'auto' : 'smooth' });
	}
	// Fresh tracks page a whole set of columns at a time, like the hero.
	let trackList = $state<HTMLElement>();
	let tStart = $state(true);
	let tEnd = $state(false);
	function tEdges() {
		if (!trackList) return;
		tStart = trackList.scrollLeft < 4;
		tEnd = trackList.scrollLeft + trackList.clientWidth >= trackList.scrollWidth - 4;
	}
	$effect(() => {
		if (trackList) requestAnimationFrame(tEdges);
	});
	/** See all: one row as a grid, in place of the shelves. */
	let allRow = $state<string | null>(null);
	const all = $derived(answer?.rows.find((r) => r.id === allRow) ?? null);
	function seeAll(id: string | null) {
		allRow = id;
		scrollTo({ top: 0 });
	}
	$effect(() => {
		if (answer?.hero.length) requestAnimationFrame(edges);
	});

	const tracks = $derived(answer?.rows.find((r) => r.type === 'np_music_tracks'));
	const rows = $derived((answer?.rows ?? []).filter((r) => r.type !== 'np_music_tracks'));
	// Explore more: the shared Music tile, the same mix as Explore's Browse (genres, moods, a season, decades, activities), two rows and Browse all. Only tiles with a real album cover come.
	let more = $state<BrowseTileData[]>([]);
	onMount(() => {
		void fetchBrowseTiles(defaultRegion(), api, gone.signal).then(
			(a) => (more = (a.explore ?? []).map((t) => ({ ...t, href: to(t.href) }))),
			() => {}
		);
	});
	const actLabel = (c: NewMusicCard) => (manage ? t('music.add', { name: c.artist || c.name }) : t('music.askFor', { name: c.name }));
</script>

{#snippet albumCard(row: { type: string }, c: NewMusicCard)}
	{@const cv = coverFor(musicImage(c), 200)}
	<PosterCard title={c.name} href={musicCardHref(c)} image={cv.src} kind="artist" subtitle={rowNote(row, c) || undefined} badges={cardBadges(c)} shape="square" music needsArt actionOnHover
		status={c.title_id && row.type === 'np_music_library' ? 'in_library' : undefined}>
		{#snippet action()}
			{#if c.upcoming}
				<IconButton icon={c.following ? 'bellOn' : 'bell'} label={c.following ? t('music.following', { name: c.name }) : t('music.tellMeWhenIs', { name: c.name })} variant="glass" size="lg" aria-pressed={!!c.following} onclick={() => void follow(c)} />
			{:else if canAct(c)}
				<IconButton icon="plus" label={actLabel(c)} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={busy === c.key || undefined} onclick={() => void quick(c)} />
			{/if}
		{/snippet}
	</PosterCard>
{/snippet}

{#if answer?.kid}
	<EmptyState icon="music" title={t('music.musicIsNotShown')} text={t('music.musicHasNoAge')} />
{:else if failed && !answer}
	<EmptyState icon="music" title={t('music.couldNotLoadNew')} text={t('music.theMusicListsDid')} />
{:else if !answer || (answer.building && !answer.rows.length)}
	<div class="loading" aria-busy="true" aria-label={t('music.loadingNewMusic')}>
		<div class="hero-sk">{#each [0, 1, 2] as i (i)}<span class="sk"></span>{/each}</div>
		{#if answer?.building}<Orbit status={t('music.newReleasesAreBeing')} />{/if}
		<SkRail kind="poster" count={9} titleWidth="160px" />
	</div>
{:else if !answer.rows.length && !answer.hero.length}
	<EmptyState icon="music" title={t('music.noNewMusicYet')} text={t('music.newReleasesFillIn')} />
{:else if all}
	<section class="see-all" data-testid="new-music-all">
		<button class="back" type="button" onclick={() => seeAll(null)}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M14.5 6 8.5 12l6 6" /></svg>{t('music.newMusic')}</button>
		<h2>{all.title}</h2>
		<p class="sub">{all.reason}</p>
		<ul class="grid">
			{#each all.items as raw (raw.key)}
				<li>{@render albumCard(all, live(raw))}</li>
			{/each}
		</ul>
	</section>
{:else}
	<section class="music-new" data-testid="new-music">
		{#if answer.hero.length}
			<div class="hero-wrap">
				<div class="hero" use:dragScroll bind:this={shelf} onscroll={edges} role="list" aria-label={t('music.thisWeekSBiggest')} data-testid="new-music-hero">
					{#each answer.hero as raw (raw.key)}
						{@const c = live(raw)}
						<article class="h-card" role="listitem">
							<a class="h-link" href={musicCardHref(c)}>
								<span class="eyebrow">{eyebrow(c)}</span>
								<span class="h-title">{#if c.explicit}<span class="explicit" title={t('music.explicit')} aria-label={t('music.explicit')}>E</span>{/if}{c.name}</span>
								<span class="h-artist">{c.artist}</span>
								<span class="art">
									<img class="wide" src={musicImage({ image: c.artist_image || c.image })} alt="" loading="lazy" onerror={(e) => ((e.currentTarget as HTMLImageElement).style.visibility = 'hidden')} />
									<span class="shade"></span>
									{#if c.image}<img class="cover" src={musicImage(c)} alt="" loading="lazy" onerror={(e) => ((e.currentTarget as HTMLImageElement).hidden = true)} />{/if}
								</span>
								<span class="note-below">{heroNote(c)}</span>
							</a>
							{#if canAct(c)}
								<span class="h-act"><IconButton icon="plus" label={actLabel(c)} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={busy === c.key || undefined} onclick={() => void quick(c)} /></span>
							{/if}
						</article>
					{/each}
				</div>
				<button class="paddle prev" class:show={!atStart} aria-label={t('music.previous')} tabindex={atStart ? -1 : 0} onclick={() => page(-1)}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M14.5 6 8.5 12l6 6" /></svg></button>
				<button class="paddle next" class:show={!atEnd} aria-label={t('music.next')} tabindex={atEnd ? -1 : 0} onclick={() => page(1)}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9.5 6l6 6-6 6" /></svg></button>
			</div>
		{/if}

		{#if tracks}
			<section class="tracks-row" aria-labelledby="np-tracks-h">
				<header class="sh"><h2 id="np-tracks-h"><button type="button" onclick={() => seeAll(tracks.id)}>{tracks.title}<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9.5 6l6 6-6 6" /></svg></button></h2><p>{tracks.reason}</p></header>
				<div class="pager">
					<ol class="tracks" use:dragScroll bind:this={trackList} onscroll={tEdges} data-testid="new-music-tracks">
						{#each tracks.items as raw (raw.key)}
							{@const c = live(raw)}
							<li class="track">
								<img src={musicImage(c)} alt="" loading="lazy" onerror={(e) => ((e.currentTarget as HTMLImageElement).style.visibility = 'hidden')} />
								<a class="t-words" href={musicCardHref(c)}><span class="t-name">{#if c.explicit}<span class="explicit" title={t('music.explicit')} aria-label={t('music.explicit')}>E</span>{/if}{c.name}</span><span class="t-artist">{rowNote(tracks, c)}</span></a>
								{#if canAct(c)}<IconButton icon="more" label={manage ? t('music.add', { name: c.artist ?? '' }) : t('music.askFor', { name: c.name })} size="sm" aria-haspopup="dialog" aria-busy={busy === c.key || undefined} onclick={() => void quick(c)} />{/if}
							</li>
						{/each}
					</ol>
					<button class="paddle prev" class:show={!tStart} aria-label={t('music.previousTracks')} tabindex={tStart ? -1 : 0} onclick={() => page(-1, trackList)}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M14.5 6 8.5 12l6 6" /></svg></button>
					<button class="paddle next" class:show={!tEnd} aria-label={t('music.moreTracks')} tabindex={tEnd ? -1 : 0} onclick={() => page(1, trackList)}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9.5 6l6 6-6 6" /></svg></button>
				</div>
			</section>
		{/if}

		{#each rows as row, ri (row.id)}
			{#if ri === 1 && rising}
				<div class="row" data-row="rising_today">
					<Shelf title={rising.title} subtitle={rising.reason} item="var(--music-circle, var(--album-item))" items={rising.items} key={(c) => c.key} placeholder="round" lines={2}>
						{#snippet card(c: MusicCardData)}
							<PosterCard title={c.name} href={musicCardHref(c)} image={coverFor(musicImage(c), 200).src} kind="artist" subtitle={c.note || undefined} shape="round" music needsArt
								status={c.title_id ? 'in_library' : undefined} />
						{/snippet}
					</Shelf>
				</div>
			{/if}
			{#if row.shape === 'wide'}
			<MusicVideoRow title={row.title} reason={row.reason} items={row.items} />
			{:else}
			<div class="row" data-row={row.type}>
				<Shelf title={row.title} subtitle={row.reason} item="var(--album-item)" items={row.items} key={(c) => c.key} placeholder="square" lines={2} onseeall={() => seeAll(row.id)}>
					{#snippet card(raw: NewMusicCard)}
						{@render albumCard(row, live(raw))}
					{/snippet}
				</Shelf>
			</div>
			{/if}
		{/each}

		{#if more.length}<MusicBrowseGrid title={t('music.exploreMore')} tiles={more} allHref={to('/search?kind=music')} />{/if}
	</section>
{/if}

{#if manage}
	<sheets.add bind:this={addArtist} {folders} {rules} {defaults} onadded={added} />
{:else if mayAsk}
	<sheets.ask bind:this={askArtist} onasked={asked} onrefused={(a, body) => typeof body.title_id === 'number' && (addedHere = new Map([...addedHere, [a.mbid, body.title_id]]))} />
{/if}

<style>
	.music-new { display: flex; flex-direction: column; gap: 48px; --shelf-gap: 20px; --album-item: 200px; }
	@media (max-width: 767px) { .music-new { --album-item: calc((100cqi - 3 * 20px) / 4.3); } }
	@media (max-width: 640px) { .music-new { gap: 36px; --album-item: calc((100cqi - 20px) / 2.3); } }
	.sh h2 { margin: 0; }
	.sh h2 button { display: inline-flex; align-items: center; gap: 2px; padding: 0; border: 0; background: none; color: var(--text); cursor: pointer; font: var(--t-title-3, 600 20px/1.3 var(--font-ui)); }
	.sh h2 svg, .back svg { width: 18px; height: 18px; fill: none; stroke: currentColor; stroke-width: 2.2; stroke-linecap: round; stroke-linejoin: round; color: var(--text-muted); }
	.sh p { margin: 2px 0 10px; color: var(--text-muted); font-size: 14px; }
	.pager { position: relative; }
	.pager:hover .paddle.show { opacity: 1; pointer-events: auto; }
	.pager .paddle { top: 50%; }
	.hero-wrap { position: relative; --h-cols: calc(31.5% - 13.33px); }
	.hero { display: grid; grid-auto-flow: column; grid-auto-columns: var(--h-cols); gap: 20px; overflow-x: auto; overflow-y: hidden; scroll-snap-type: x mandatory; scroll-behavior: smooth; scrollbar-width: none; padding: 4px 2px 10px; }
	.hero::-webkit-scrollbar { display: none; }
	.h-card { position: relative; scroll-snap-align: start; min-width: 0; }
	.h-link { display: flex; flex-direction: column; color: var(--text); text-decoration: none; border-radius: 12px; }
	.h-link:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.eyebrow { font: 600 11px/1.3 var(--font-ui); letter-spacing: 0.06em; text-transform: uppercase; color: var(--text-muted); }
	.h-title { margin-top: 2px; font-size: 17px; line-height: 1.3; font-weight: 400; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.h-artist { font-size: 17px; line-height: 1.3; color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; margin-bottom: 8px; }
	.art { position: relative; display: block; aspect-ratio: 4 / 3; border-radius: 10px; overflow: hidden; background: var(--surface-2); transition: transform var(--dur) var(--ease-out), filter var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out); }
	.wide { width: 100%; height: 100%; object-fit: cover; object-position: center 25%; }
	.shade { position: absolute; inset: 0; background: linear-gradient(to top, rgb(0 0 0 / 0.6), transparent 45%); }
	.note-below { display: block; margin-top: 8px; color: var(--text-muted); font-size: 13px; line-height: 1.3; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.cover { position: absolute; right: 64px; bottom: 10px; width: 60px; height: 60px; border-radius: 4px; object-fit: cover; box-shadow: 0 2px 10px rgb(0 0 0 / 0.4); }
	.h-card:hover .art, .h-link:focus-visible .art { transform: translateY(-3px); filter: brightness(1.08); box-shadow: 0 10px 24px rgb(0 0 0 / 0.22); }
	.h-act { position: absolute; bottom: 16px; right: 12px; opacity: 0; transition: opacity var(--dur-fast) var(--ease-out); }
	.h-card:hover .h-act, .h-card:focus-within .h-act { opacity: 1; }
	.paddle { position: absolute; top: calc(50% + 2.3em); transform: translateY(-50%); width: 36px; height: 36px; border-radius: 50%; border: 0; display: grid; place-items: center; cursor: pointer; background: color-mix(in srgb, var(--surface-1, var(--bg)) 86%, transparent); color: var(--text); box-shadow: 0 2px 10px rgb(0 0 0 / 0.25); opacity: 0; pointer-events: none; transition: opacity var(--dur) var(--ease-out); z-index: 2; }
	.paddle svg { width: 18px; height: 18px; fill: none; stroke: currentColor; stroke-width: 2; stroke-linecap: round; stroke-linejoin: round; }
	.paddle.prev { left: -10px; }
	.paddle.next { right: -10px; }
	.hero-wrap:hover .paddle.show, .paddle.show:focus-visible { opacity: 1; pointer-events: auto; }
	.paddle:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (hover: none) { .h-act { opacity: 1; } }
	@media (max-width: 1023px) { .hero-wrap { --h-cols: calc(48% - 10px); } }
	@media (max-width: 640px) { .hero-wrap { --h-cols: calc(100% / 1.1 - 10px); } .paddle { display: none; } }
	@media (prefers-reduced-motion: reduce) { .hero { scroll-behavior: auto; } .art, .paddle, .h-act { transition: none; } .h-card:hover .art { transform: none; } }
	.see-all h2 { margin: 0; font: var(--t-title-3, 600 20px/1.3 var(--font-ui)); }
	.tracks { list-style: none; margin: 0; padding: 0; display: grid; grid-auto-flow: column; grid-template-rows: repeat(4, 56px); grid-auto-columns: calc(25% - 15px); column-gap: 20px; row-gap: 0; height: 224px; overflow-x: auto; overflow-y: hidden; scroll-snap-type: x mandatory; scroll-behavior: smooth; scrollbar-width: none; }
	@media (prefers-reduced-motion: reduce) { .tracks { scroll-behavior: auto; } }
	.tracks::-webkit-scrollbar { display: none; }
	.track { display: flex; align-items: center; gap: 10px; min-width: 0; height: 56px; padding: 0 8px; box-sizing: border-box; border-radius: 7px; }
	.track:hover { background: rgb(255 255 255 / 0.12); }
	.track:nth-child(4n + 1) { scroll-snap-align: start; }
	.track img { flex: none; width: 44px; height: 44px; border-radius: 4px; object-fit: cover; background: var(--surface-2); }
	.t-words { flex: 1; min-width: 0; display: flex; flex-direction: column; color: var(--text); text-decoration: none; }
	.t-name, .t-artist { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; font-size: 14px; line-height: 1.35; }
	.t-artist { color: var(--text-muted); font-size: 13px; }
	.explicit { display: inline-grid; place-items: center; inline-size: 14px; block-size: 14px; margin-inline-end: 6px; border-radius: 3px; background: var(--text-muted); color: var(--bg, #000); font-size: 10px; font-weight: 600; line-height: 1; vertical-align: 1px; }
	@media (max-width: 1023px) { .tracks { grid-auto-columns: calc(50% - 8px); } }
	@media (max-width: 640px) { .tracks { grid-auto-columns: 88%; } }
	.row { position: relative; min-width: 0; }
	.see-all { display: flex; flex-direction: column; gap: 8px; }
	.back { align-self: flex-start; display: inline-flex; align-items: center; gap: 2px; padding: 0; border: 0; background: none; color: var(--text-muted); cursor: pointer; font-size: 14px; }
	.sub { margin: 0 0 12px; color: var(--text-muted); font-size: 14px; }
	.grid { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: 24px 20px; }
	.loading { display: flex; flex-direction: column; gap: 12px; }
	.hero-sk { display: grid; grid-template-columns: repeat(3, 1fr); gap: 20px; }
	.hero-sk span { aspect-ratio: 4 / 3; border-radius: 10px; margin-top: 60px; }
	@media (max-width: 640px) { .hero-sk { grid-template-columns: 1fr; } .hero-sk span:nth-child(n + 2) { display: none; } }
</style>
