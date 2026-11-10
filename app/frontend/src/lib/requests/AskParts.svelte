<script lang="ts">
	// What a music or TV ask is made of, folded under one line that opens: an artist ask's albums ("12 albums · 8 singles & EPs", one row per album with its other editions under a small "3 editions" note), or a show ask's seasons and episodes.
	import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import { albumCounts, albumGroups, songsHead, askAlbums, askSeasons, askedWords, picksChanged, picksHead, seasonPick, type PartsPick, type SeasonPick } from './ask-parts.ts';
	import { albumSongs, artistIdOf, askArtist, musicKinds, songKey, storedParts, type AlbumSong } from './details.svelte.ts';
	import type { BoardCtx } from './board.ts';
	import { appPicture, coverFor } from '../music/index.ts';
	import { slide } from 'svelte/transition';
	import { DUR, easeOut } from '../core/transitions.ts';
	import type { Ask } from '../core/types.ts';

	let { ctx, a, editable = false, unfolded = false, layout = 'list', pick = $bindable({ seasons: null, albums: null, empty: false }) }: { ctx: BoardCtx; a: Ask; editable?: boolean; /** Open from the start, with the first season's episodes showing (the Requests Stage). */ unfolded?: boolean; /** Rows that open (list), or two panes that fill their box (split: the Requests sheet). */ layout?: 'list' | 'split'; pick?: PartsPick } = $props();

	const music = $derived(a.kind === 'artist');
	const artist = $derived(music ? askArtist(a) : null);
	const kinds = $derived(music ? musicKinds() : null);
	const cover = (id: string) => appPicture(`/api/v1/music-trending/art/album/${id}`) ?? '';
	// Read only (an answered ask): every album of the artist, so the ones left out show as left out.
	const approved = $derived(!editable && music && a.state !== 'asked' && a.albums?.length ? a.albums : null);
	const rows = $derived(music && artist ? albumGroups(approved ? mergeAlbums(askAlbums(a, artist.albums, kinds ?? undefined, cover), askAlbums({ albums: [] }, artist.albums, kinds ?? undefined, cover)) : askAlbums(a, artist.albums, kinds ?? undefined, cover)) : []);
	function mergeAlbums<T extends { mbid: string }>(kept: T[], all: T[]): T[] {
		const ids = new Set(kept.map((x) => x.mbid));
		return [...kept, ...all.filter((x) => !ids.has(x.mbid))];
	}
	const albumsLoading = $derived(music && !a.albums?.length && artist?.albums === null);
	const info = $derived(a.kind === 'show' ? ctx.info(a) : null);
	const seasons = $derived(a.kind === 'show' && info ? askSeasons(a, info.seasons) : []);

	// What the approver left out, by album row key or season number.
	let left = $state<Set<string | number>>(new Set());
	$effect(() => {
		void a.id;
		left = new Set();
	});
	function toggle(k: string | number) {
		const next = new Set(left);
		if (next.has(k)) next.delete(k);
		else next.add(k);
		left = next;
	}
	$effect(() => {
		if (music) {
			const kept = albumPicks.filter((p) => p.tick !== 'none');
			const partly = albumPicks.filter((p) => p.tick === 'some');
			const changed = left.size > 0 || partly.length > 0;
			pick = {
				seasons: null,
				albums: changed && kept.length ? kept.flatMap((p) => p.row.ids) : null,
				songs: partly.length ? partly.map((p) => ({ album: p.row.main.mbid, recordings: p.ticked })) : null,
				empty: rows.length > 0 && kept.length === 0
			};
		} else {
			const whole = picks.filter((p) => p.tick === 'all').map((p) => p.number);
			const single = picks.filter((p) => p.tick === 'some').map((p) => ({ season: p.number, episodes: p.ticked }));
			const changed = picksChanged(picks);
			pick = {
				seasons: changed && (whole.length || single.length) ? whole : null,
				episodes: changed && single.length ? single : null,
				albums: null,
				empty: picks.length > 0 && picks.every((p) => p.tick === 'none')
			};
		}
	});

	// The approver's song ticks, by album row (absent means every song); only albums in the Library have stored songs.
	let songTicks = $state<Record<string, string[]>>({});
	let openAlbum = $state('');
	$effect(() => {
		void a.id;
		songTicks = {};
	});
	type AlbumPick = { row: (typeof rows)[number]; list: AlbumSong[] | null; ticked: string[]; tick: 'all' | 'some' | 'none' };
	// The lists stored when the ask was made, so opening a season or an album shows them at once.
	const kept = $derived(storedParts(a));
	const artistId = $derived(music ? artistIdOf(a) : '');
	const albumPicks = $derived(
		rows.map((r): AlbumPick => {
			// Stored songs first; an album not stored yet asks the album page once it is opened.
			const list = kept?.albums[r.main.mbid] ?? (kept && openAlbum === r.key ? albumSongs(r.main.mbid, artistId) : null);
			const all = list?.map(songKey) ?? [];
			if (approved) {
				// What was approved: albums not kept are left out; a kept album's stored songs are its ticks.
				const w = approved.find((x) => r.ids.includes(x.mbid));
				if (!w) return { row: r, list, ticked: [], tick: 'none' };
				if (!w.songs?.length || !list) return { row: r, list, ticked: all, tick: 'all' };
				const t = all.filter((x) => w.songs!.includes(x));
				return { row: r, list, ticked: t, tick: t.length < all.length ? 'some' : 'all' };
			}
			if (left.has(r.key)) return { row: r, list, ticked: [], tick: 'none' };
			const t = songTicks[r.key]?.filter((x) => all.includes(x));
			if (!list || !t) return { row: r, list, ticked: all, tick: 'all' };
			return { row: r, list, ticked: t, tick: !t.length ? 'none' : t.length < all.length ? 'some' : 'all' };
		})
	);
	function tickAlbum(p: AlbumPick) {
		const next = { ...songTicks };
		delete next[p.row.key];
		songTicks = next;
		const l = new Set(left);
		if (p.tick === 'all') l.add(p.row.key);
		else l.delete(p.row.key);
		left = l;
	}
	// All or None for one album: every song kept, or the album left out.
	function setAlbum(p: AlbumPick, on: boolean) {
		const next = { ...songTicks };
		delete next[p.row.key];
		songTicks = next;
		const l = new Set(left);
		if (on) l.delete(p.row.key);
		else l.add(p.row.key);
		left = l;
	}
	function tickSong(p: AlbumPick, id: string) {
		const base = p.tick === 'none' ? [] : p.ticked;
		const t = base.includes(id) ? base.filter((x) => x !== id) : [...base, id];
		songTicks = { ...songTicks, [p.row.key]: t };
		const l = new Set(left);
		l.delete(p.row.key);
		left = l;
	}
	// Round the whole length first, so 179.6 s reads 3:00, never 2:60.
	const songLen = (ms?: number) => {
		if (!ms) return '';
		const sec = Math.round(ms / 1000);
		return `${Math.floor(sec / 60)}:${String(sec % 60).padStart(2, '0')}`;
	};

	// The approver's episode ticks, by season (absent means as asked).
	let ticks = $state<Record<number, number[]>>({});
	$effect(() => {
		void a.id;
		ticks = {};
	});
	// A season's episode list: asked when the list opens, or when the season count is missing.
	const listOf = (s: { number: number; episodes: number }) => kept?.seasons[String(s.number)] ?? (kept && (openSeason === s.number || !s.episodes) ? ctx.episodes(a.tmdb_id, s.number) : null);
	const picks = $derived(seasons.map((s) => seasonPick(s, listOf(s), ticks[s.number])));
	function tickSeason(p: SeasonPick) {
		const next = { ...ticks, [p.number]: p.tick === 'all' ? [] : [...p.all] };
		if (!p.all.length && p.tick !== 'all') delete next[p.number];
		ticks = next;
	}
	function tickEpisode(p: SeasonPick, n: number) {
		const next = p.ticked.includes(n) ? p.ticked.filter((x) => x !== n) : [...p.ticked, n].sort((x, y) => x - y);
		ticks = { ...ticks, [p.number]: next };
	}
	const shortDay = (d?: string) => (d && /^\d{4}-\d{2}-\d{2}/.test(d) ? new Date(`${d.slice(0, 10)}T12:00:00`).toLocaleDateString(appLocale(), { month: 'short', day: 'numeric', year: 'numeric' }) : '');

	let open = $state(false);
	let openSeason = $state(0);
	$effect(() => {
		if (!unfolded) return;
		open = true;
		// Opens on the first season that was asked for; a read-only list never opens on one left out.
		if (!openSeason && seasons.length) openSeason = (seasons.find((x) => a.seasons?.includes(x.number)) ?? seasons[0]).number;
	});
	$effect(() => {
		if (layout === 'split' && music && !openAlbum && rows.length) openAlbum = (albumPicks.find((p) => p.tick !== 'none') ?? albumPicks[0]).row.key;
	});
	let openEditions = $state('');
	const reduced = () => typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
	const slideIn = () => ({ duration: reduced() ? 0 : DUR.base, easing: easeOut });
	// The local day (late evening must not count as tomorrow), as lifecycle.ts does.
	const today = (() => {
		const d = new Date();
		return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
	})();
	// A declined ask had nothing approved, so its marks say what was asked for.
	const markOn = $derived(a.state === 'turned_down' ? t('requests.parts.askedFor') : t('requests.parts.approved'));
	const markOff = $derived(a.state === 'turned_down' ? t('requests.parts.notAskedFor') : t('requests.parts.leftOut'));
	// Phones show the albums as a sideways strip: keep the open one in view.
	let strip = $state<HTMLElement>();
	$effect(() => {
		void openAlbum;
		const el = strip?.querySelector<HTMLElement>('.sp-open[aria-pressed="true"]');
		if (el && strip && strip.scrollWidth > strip.clientWidth + 1) el.scrollIntoView({ block: 'nearest', inline: 'nearest' });
	});
	const keptRows = $derived(rows.filter((r) => !left.has(r.key)));
	const head = $derived.by(() => {
		if (music) {
			if (albumsLoading) return t('requests.parts.loadingAlbums');
			if (!rows.length) return a.album_words ? a.album_words[0].toUpperCase() + a.album_words.slice(1) : t('requests.askedFor.allAlbums');
			const kept = albumPicks.filter((p) => p.tick !== 'none');
			return songsHead(albumCounts(left.size ? kept.map((p) => p.row) : rows), albumPicks.map((p) => ({ all: p.list?.length ?? null, ticked: p.ticked.length })));
		}
		return picksHead(picks);
	});
	const listId = $derived(`parts-${a.id}`);

	// Album kinds, as the left pane's small headings.
	const kindLabel = (k: string) => (['albums', 'singles', 'live', 'compilations', 'other'].includes(k) ? t(`requests.parts.kindHead.${k}`) : k);
	const seasonTitle = (n: number) => (n === 0 ? t('requests.parts.specials') : t('requests.parts.season', { n }));
	const shown = $derived(music ? rows.length > 0 || albumsLoading : seasons.length > 0);
</script>

{#if shown && layout === 'split'}
	<div class="split" data-testid="ask-parts">
		<!-- A plain list: each row's tick sits beside a real button that opens that season or album. -->
		<ul class="sp-left" aria-label={music ? t('requests.parts.albumsLabel') : t('requests.parts.seasonsLabel')} bind:this={strip}>
			{#if music}
				{#each albumPicks as p (p.row.key)}
					{@const r = p.row}
					{@const first = albumPicks.find((x) => x.row.kind === r.kind) === p}
					{#if first}<li class="sp-kind" aria-hidden="true">{kindLabel(r.kind)}</li>{/if}
					<li class="sp-row" class:on={openAlbum === r.key} class:off={p.tick === 'none'} data-testid="ask-album" data-tick={p.tick}>
						{#if editable}<label class="sp-tk"><input class="tick" type="checkbox" checked={p.tick === 'all'} indeterminate={p.tick === 'some'} aria-label={t('requests.parts.tick', { name: r.main.name })} onchange={() => tickAlbum(p)} /></label>{/if}
						<button type="button" class="sp-open" aria-pressed={openAlbum === r.key} onclick={() => (openAlbum = r.key)}>
							<span class="sp-cov">{#if r.main.image}<img src={coverFor(r.main.image, 64).src} alt="" loading="lazy" decoding="async" onerror={(e) => ((e.currentTarget as HTMLImageElement).style.visibility = 'hidden')} />{/if}</span>
							<span class="sp-t"><span class="sp-n">{r.main.name}</span><span class="sp-s">{r.main.year ?? ''}{p.list?.length ? ` · ${t('requests.parts.someSongs', { kept: p.tick === 'all' ? p.list.length : p.ticked.length, all: p.list.length })}` : ''}</span></span>
							<Icon name="chevronRight" size={14} />
						</button>
					</li>
				{/each}
			{:else}
				{#each picks as p, i (p.number)}
					{@const s = seasons[i]}
					<li class="sp-row" class:on={openSeason === s.number} class:off={p.tick === 'none'} data-testid="ask-season" data-tick={p.tick}>
						{#if editable}<label class="sp-tk"><input class="tick" type="checkbox" checked={p.tick === 'all'} indeterminate={p.tick === 'some'} aria-label={s.number === 0 ? t('requests.parts.tickSpecials') : t('requests.parts.tickSeason', { n: s.number })} onchange={() => tickSeason(p)} /></label>{/if}
						<button type="button" class="sp-open" aria-pressed={openSeason === s.number} onclick={() => (openSeason = s.number)}>
							<span class="sp-num">{s.number === 0 ? t('requests.parts.sp') : s.number}</span>
							<span class="sp-t"><span class="sp-n">{seasonTitle(s.number)}</span><span class="sp-s">{p.tick === 'none' ? t('requests.parts.leftOut') : !(s.episodes || p.all.length) ? t('requests.parts.notOut') : t('requests.parts.someEpisodes', { kept: p.ticked.length || s.episodes, n: s.episodes || p.all.length })}</span></span>
							<Icon name="chevronRight" size={14} />
						</button>
					</li>
				{/each}
			{/if}
		</ul>
		<div class="sp-right">
			{#if music}
				{@const p = albumPicks.find((x) => x.row.key === openAlbum)}
				{#if p}
					<header class="sp-h">
						<span class="sp-ht">{p.row.main.name}</span>
						{#if editable && p.list?.length}
							<button type="button" class="sp-a" aria-label={t('requests.parts.tickSongs', { name: p.row.main.name })} onclick={() => setAlbum(p, true)}>{t('requests.parts.all')}</button>
							<button type="button" class="sp-a" aria-label={t('requests.parts.untickSongs', { name: p.row.main.name })} onclick={() => setAlbum(p, false)}>{t('requests.parts.none')}</button>
						{/if}
					</header>
					{#if p.list === null}<p class="hint">{t('requests.parts.loadingSongs')}</p>
					{:else if !p.list.length}<p class="hint">{t('requests.parts.noSongs')}</p>
					{:else}
						<ol class="sp-list" aria-label={t('requests.parts.songsOf', { name: p.row.main.name })}>
							{#each p.list as sg (songKey(sg))}
								{@const on = p.ticked.includes(songKey(sg))}
								<li class:off={!on} data-testid="ask-song">
									<label class="sp-item">
										{#if editable}<input class="tick" type="checkbox" checked={on} aria-label={t('requests.parts.songTick', { n: sg.number, name: sg.name || t('requests.parts.songN', { n: sg.number }) })} onchange={() => tickSong(p, songKey(sg))} />{:else}<span class="mk" class:no={!on} role="img" aria-label={on ? markOn : markOff}><Icon name={on ? 'check' : 'dash'} size={14} /></span>{/if}
										<span class="n">{sg.disc > 1 ? `${sg.disc}-` : ''}{sg.number}</span>
										<span class="t">{sg.name || t('requests.parts.songN', { n: sg.number })}</span>
										<span class="y">{songLen(sg.length_ms)}</span>
									</label>
								</li>
							{/each}
						</ol>
					{/if}
				{/if}
			{:else}
				{@const i = seasons.findIndex((x) => x.number === openSeason)}
				{@const s = seasons[i]}
				{@const p = picks[i]}
				{#if s && p}
					{@const list = listOf(s)}
					<header class="sp-h">
						<span class="sp-ht">{seasonTitle(s.number)}</span>
						{#if editable && p.all.length}
							<button type="button" class="sp-a" aria-label={t('requests.parts.tickAllIn', { where: seasonTitle(s.number) })} onclick={() => (ticks = { ...ticks, [p.number]: [...p.all] })}>{t('requests.parts.all')}</button>
							<button type="button" class="sp-a" aria-label={t('requests.parts.untickAllIn', { where: seasonTitle(s.number) })} onclick={() => (ticks = { ...ticks, [p.number]: [] })}>{t('requests.parts.none')}</button>
						{/if}
					</header>
					{#if list === null}<p class="hint">{t('requests.parts.loadingSeason', { n: s.number })}</p>
					{:else if !list.length}<p class="hint">{t('requests.parts.noEpisodes', { n: s.number })}</p>
					{:else}
						<ol class="sp-list" aria-label={t('requests.parts.seasonEpisodes', { n: s.number })}>
							{#each list as e (e.number)}
								{@const on = p.ticked.includes(e.number)}
								{@const later = !!e.air_day && e.air_day.slice(0, 10) > today}
								<li class:off={!on} class:later data-testid="ask-episode">
									<label class="sp-item">
										{#if editable}<input class="tick" type="checkbox" checked={on} aria-label={t('requests.parts.episodeTick', { n: e.number, name: e.name || t('requests.parts.episodeN', { n: e.number }) })} onchange={() => tickEpisode(p, e.number)} />{:else}<span class="mk" class:no={!on} role="img" aria-label={on ? markOn : markOff}><Icon name={on ? 'check' : 'dash'} size={14} /></span>{/if}
										<span class="n">E{e.number}</span>
										<span class="t">{e.name || t('requests.parts.episodeN', { n: e.number })}</span>
										{#if !p.asked.includes(e.number)}<span class="tag">{t('requests.parts.notAskedFor')}</span>{/if}
										<span class="y">{later ? t('requests.parts.airs', { date: shortDay(e.air_day) }) : shortDay(e.air_day)}</span>
									</label>
								</li>
							{/each}
						</ol>
					{/if}
				{/if}
			{/if}
		</div>
	</div>
{:else if shown}
	<div class="parts" data-testid="ask-parts">
		<button type="button" class="head" aria-expanded={open} aria-controls={listId} disabled={albumsLoading} onclick={() => (open = !open)} data-testid="ask-parts-toggle">
			<Icon name={music ? 'music' : 'tv'} size={16} />
			<span class="hw">{head}</span>
			{#if !albumsLoading}<span class="chev" class:up={open}><Icon name="chevronDown" size={16} /></span>{/if}
		</button>
		{#if open}
			<div class="list" id={listId} transition:slide={slideIn()}>
				<ul>
					{#if music}
						{#each albumPicks as p (p.row.key)}
							{@const r = p.row}
							<li class:off={p.tick === 'none'} data-testid="ask-album" data-tick={p.tick}>
								<!-- The whole row opens and closes the song list; the tick and the buttons keep their own job. -->
								<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
								<div class="line rowtap" onclick={(e) => { if (!(e.target as HTMLElement).closest('input, button')) openAlbum = openAlbum === r.key ? '' : r.key; }}>
									{#if editable}<input class="tick" type="checkbox" checked={p.tick === 'all'} indeterminate={p.tick === 'some'} aria-label={t('requests.parts.tick', { name: r.main.name })} onchange={() => tickAlbum(p)} />{/if}
									<span class="cover">{#if r.main.image}<img src={coverFor(r.main.image, 32).src} alt="" loading="lazy" decoding="async" onerror={(e) => ((e.currentTarget as HTMLImageElement).style.display = 'none')} />{/if}</span>
									<span class="t">{r.main.name}</span>
									{#if r.editions.length}
										<button type="button" class="more" aria-expanded={openEditions === r.key} onclick={() => (openEditions = openEditions === r.key ? '' : r.key)}>{t('requests.parts.editions', { n: r.editions.length + 1 })}</button>
									{/if}
									<span class="y">{p.list?.length ? (p.tick === 'all' ? t(`requests.parts.songs.${p.list.length === 1 ? 'one' : 'other'}`, { n: p.list.length }) : t('requests.parts.ofCount', { kept: p.ticked.length, all: p.list.length })) + (r.main.year ? ` · ${r.main.year}` : '') : (r.main.year ?? '')}</span>
									<button type="button" class="open" aria-expanded={openAlbum === r.key} aria-label={t('requests.parts.songsOf', { name: r.main.name })} onclick={() => (openAlbum = openAlbum === r.key ? '' : r.key)}><span class="chev" class:up={openAlbum === r.key}><Icon name="chevronDown" size={14} /></span></button>
								</div>
								{#if openAlbum === r.key}
									<div transition:slide={slideIn()}>
										{#if p.list === null}
											<p class="hint">{t('requests.parts.loadingSongs')}</p>
										{:else if !p.list.length}
											<p class="hint" data-testid="songs-none">{t('requests.parts.noSongs')}</p>
										{:else}
											<ol class="eps" aria-label={t('requests.parts.songsOf', { name: r.main.name })}>
												{#each p.list as sg (songKey(sg))}
													{@const on = p.ticked.includes(songKey(sg))}
													<li class:off={!on} data-testid="ask-song">
														<label class="ep">
															{#if editable}<input class="tick" type="checkbox" checked={on} aria-label={t('requests.parts.songN', { n: sg.number })} onchange={() => tickSong(p, songKey(sg))} />{/if}
															<span class="n">{sg.disc > 1 ? `${sg.disc}-` : ''}{sg.number}</span>
															<span class="t">{sg.name || t('requests.parts.songN', { n: sg.number })}</span>
															<span class="y">{songLen(sg.length_ms)}</span>
														</label>
													</li>
												{/each}
											</ol>
										{/if}
									</div>
								{/if}
								{#if openEditions === r.key}
									<ul class="eds" transition:slide={slideIn()} aria-label={t('requests.parts.editionsOf', { name: r.main.name })}>
										{#each r.editions as e (e.mbid)}<li><span class="t">{e.name}</span><span class="y">{e.year ?? ''}</span></li>{/each}
									</ul>
								{/if}
							</li>
						{/each}
					{:else}
						{#each picks as p, i (p.number)}
							{@const s = seasons[i]}
							{@const list = openSeason === s.number ? listOf(s) : null}
							{@const words = askedWords(p)}
							<li class:off={p.tick === 'none'} data-testid="ask-season" data-tick={p.tick}>
								<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
								<div class="line rowtap" onclick={(e) => { if (!(e.target as HTMLElement).closest('input, button')) openSeason = openSeason === s.number ? 0 : s.number; }}>
									{#if editable}<input class="tick" type="checkbox" checked={p.tick === 'all'} indeterminate={p.tick === 'some'} aria-label={s.number === 0 ? t('requests.parts.tickSpecials') : t('requests.parts.tickSeason', { n: s.number })} onchange={() => tickSeason(p)} />{/if}
									<button type="button" class="season" aria-expanded={openSeason === s.number} onclick={() => (openSeason = openSeason === s.number ? 0 : s.number)}>
										<span class="t">{t('requests.parts.season', { n: s.number })}{#if words}<span class="sub"> · {words}</span>{/if}</span>
										<span class="y">{p.tick === 'none' ? t('requests.parts.leftOut') : p.ticked.length !== p.asked.length || p.tick === 'some' ? t('requests.parts.ticked', { n: p.ticked.length }) : ''}</span>
										<span class="chev" class:up={openSeason === s.number}><Icon name="chevronDown" size={14} /></span>
									</button>
								</div>
								{#if openSeason === s.number}
									<div transition:slide={slideIn()}>
										{#if list === null}
											<p class="hint">{t('requests.parts.loadingSeason', { n: s.number })}</p>
										{:else if !list.length}
											<p class="hint">{t('requests.parts.noEpisodes', { n: s.number })}</p>
										{:else}
											<ol class="eps" aria-label={t('requests.parts.seasonEpisodes', { n: s.number })}>
												{#each list as e (e.number)}
													{@const on = p.ticked.includes(e.number)}
													{@const notAsked = !p.asked.includes(e.number)}
													{@const later = !!e.air_day && e.air_day.slice(0, 10) > today}
													<li class:later class:off={!on} data-testid="ask-episode">
														<label class="ep">
															{#if editable}<input class="tick" type="checkbox" checked={on} aria-label={t('requests.parts.episodeN', { n: e.number })} onchange={() => tickEpisode(p, e.number)} />{/if}
															<span class="n">E{e.number}</span>
															<span class="t">{e.name || t('requests.parts.episodeN', { n: e.number })}</span>
															{#if notAsked}<span class="tag" data-testid="not-asked">{t('requests.parts.notAskedFor')}</span>{/if}
															<span class="y">{later ? t('requests.parts.airs', { date: shortDay(e.air_day) }) : shortDay(e.air_day)}</span>
														</label>
													</li>
												{/each}
											</ol>
										{/if}
									</div>
								{/if}
							</li>
						{/each}
					{/if}
				</ul>
			</div>
		{/if}
	</div>
{/if}

<style>
	/* Two panes that fill the box they are given; each scrolls on its own. */
	.split { display: grid; grid-template-columns: minmax(220px, 300px) minmax(0, 1fr); height: 100%; min-height: 0; border-radius: 12px; box-shadow: inset 0 0 0 1px var(--line); overflow: hidden; }
	.sp-left, .sp-right { min-height: 0; overflow-y: auto; overscroll-behavior: contain; scrollbar-width: thin; }
	.sp-left { padding: 6px; border-right: 1px solid var(--line); background: color-mix(in srgb, var(--surface-2) 45%, transparent); }
	.sp-kind { margin: 8px 8px 4px; color: var(--text-muted); font: var(--t-caption); font-weight: 700; letter-spacing: 0.04em; text-transform: uppercase; }
	.sp-kind:first-child { margin-top: 2px; }
	.sp-row { display: flex; align-items: center; min-height: 44px; border-radius: 10px; color: var(--text-muted); cursor: pointer; transition: background 160ms var(--ease-out), color 160ms var(--ease-out); }
	.sp-row:hover { background: var(--surface-2); }
	.sp-row.on { background: var(--surface-1); color: var(--text); box-shadow: 0 0 0 1px var(--line), 0 1px 3px rgb(0 0 0 / 0.08); }
	.sp-row:has(.sp-open:focus-visible) { box-shadow: var(--focus-ring); }
	.sp-row .tick { width: 18px; height: 18px; margin: 0; }
	.sp-tk { flex: none; display: grid; place-items: center; align-self: stretch; min-width: 32px; padding-left: 4px; cursor: pointer; }
	/* Touch: the label around the tick is the 44px target. */
	@media (pointer: coarse) { .sp-tk { min-width: 44px; min-height: 44px; } }
	.sp-open { flex: 1; min-width: 0; display: flex; align-items: center; gap: 10px; align-self: stretch; padding: 4px 8px; border: 0; border-radius: inherit; background: none; color: inherit; font: inherit; text-align: left; cursor: pointer; }
	.sp-open:focus-visible { outline: none; }
	.sp-row.off .sp-t { opacity: 0.5; }
	.sp-cov { flex: none; width: 34px; height: 34px; border-radius: 6px; overflow: hidden; background: var(--surface-3); }
	.sp-cov img { width: 100%; height: 100%; object-fit: cover; display: block; }
	.sp-num { flex: none; display: grid; place-items: center; width: 34px; height: 34px; border-radius: 8px; background: var(--surface-3); color: var(--text); font: 800 15px/1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.sp-t { flex: 1; min-width: 0; display: flex; flex-direction: column; }
	.sp-n { color: var(--text); font: var(--t-footnote); font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.sp-s { font: var(--t-caption); font-variant-numeric: tabular-nums; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.sp-right { display: flex; flex-direction: column; }
	.sp-h { position: sticky; top: 0; z-index: 1; display: flex; align-items: center; gap: 6px; padding: 10px 12px; border-bottom: 1px solid var(--line); background: color-mix(in srgb, var(--bg) 90%, transparent); -webkit-backdrop-filter: blur(12px); backdrop-filter: blur(12px); }
	.sp-ht { flex: 1; min-width: 0; font: var(--t-subhead); font-weight: 700; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.sp-a { min-height: 36px; padding: 0 10px; border: 0; border-radius: 999px; background: var(--surface-2); color: var(--text); font: var(--t-caption); font-weight: 600; cursor: pointer; }
	.sp-a:hover { background: var(--surface-3); }
	.sp-a:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (pointer: coarse) { .sp-a { min-height: 44px; min-width: 44px; } }
	.sp-list { margin: 0; padding: 4px 6px 8px; list-style: none; columns: 2 280px; column-gap: 12px; }
	.sp-list li { break-inside: avoid; }
	.sp-item { display: flex; align-items: center; gap: 8px; min-height: 34px; padding: 0 6px; border-radius: 8px; font: var(--t-footnote); cursor: pointer; }
	.sp-item:hover { background: var(--surface-2); }
	.sp-item .n { flex: none; width: 26px; color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.sp-item .t { flex: 1; min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.sp-item .y { flex: none; color: var(--text-muted); font: var(--t-caption); font-variant-numeric: tabular-nums; }
	.sp-list li.off .t { opacity: 0.5; }
	.sp-list .tick { width: 16px; height: 16px; }
	@media (pointer: coarse) {
		.sp-item { min-height: 44px; }
		.ep.ep { min-height: 44px; }
		.more.more { min-height: 36px; padding: 0 12px; }
		.sp-tk { width: 44px; min-height: 44px; padding-left: 0; }
	}
	@media (max-width: 767px) {
		.split { grid-template-columns: minmax(0, 1fr); grid-template-rows: auto minmax(0, 1fr); }
		.sp-left { display: flex; gap: 6px; overflow-x: auto; overflow-y: hidden; border-right: 0; border-bottom: 1px solid var(--line); scrollbar-width: none; }
		.sp-left::-webkit-scrollbar { display: none; }
		/* A strip cut at the right edge fades out there, so it reads as more to swipe. */
		.sp-left { padding-right: 28px; -webkit-mask-image: linear-gradient(to right, #000 calc(100% - 36px), transparent); mask-image: linear-gradient(to right, #000 calc(100% - 36px), transparent); }
		.sp-kind { display: none; }
		.sp-row { flex: none; max-width: 220px; }
		.sp-row :global(svg:last-child) { display: none; }
		.sp-list { columns: 1; }
	}
	.parts { border-radius: var(--r-card); background: var(--surface-2); overflow: hidden; }
	.head { display: flex; align-items: center; gap: 8px; width: 100%; min-height: 44px; padding: 0 12px; border: 0; background: none; color: var(--text); font: var(--t-subhead); font-weight: 600; text-align: left; cursor: pointer; font-variant-numeric: tabular-nums; }
	.head:disabled { cursor: default; color: var(--text-muted); }
	.head:hover:not(:disabled) { background: var(--surface-3); }
	.head:focus-visible, .season:focus-visible, .more:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.hw { flex: 1; min-width: 0; overflow-wrap: anywhere; }
	.chev { display: inline-flex; flex: none; color: var(--text-muted); transition: transform var(--dur-fast) var(--ease-out); }
	.chev.up { transform: rotate(180deg); }
	.list { border-top: 1px solid var(--line); }
	.hint { padding: 8px 12px 0; color: var(--text-muted); font: var(--t-footnote); }
	ul, ol { margin: 0; padding: 0; list-style: none; }
	.list > ul { max-height: 520px; overflow-y: auto; overscroll-behavior: contain; padding: 4px 0; }
	.list > ul > li + li { border-top: 1px solid var(--line); }
	.line { display: flex; align-items: center; gap: 10px; min-height: 44px; padding: 0 12px; font: var(--t-footnote); }
	.off .t, .off .cover { opacity: 0.45; }
	.list > ul > li.off > .line .t { text-decoration: line-through; }
	.tick { width: 18px; height: 18px; flex: none; accent-color: var(--accent-fill); }
	.cover { width: 32px; height: 32px; flex: none; border-radius: 4px; overflow: hidden; background: var(--surface-3); }
	.cover img { width: 100%; height: 100%; object-fit: cover; display: block; }
	.t { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.y { flex: none; color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.more { flex: none; min-height: 24px; padding: 0 8px; border: 0; border-radius: var(--r-pill); background: var(--surface-1); color: var(--text-muted); font: var(--t-caption); font-weight: 600; cursor: pointer; }
	.more[aria-expanded='true'] { background: var(--accent-soft); color: var(--accent-text); }
	.eds { padding: 0 12px 8px 54px; }
	.eds li { display: flex; gap: 10px; padding: 4px 0; color: var(--text-muted); font: var(--t-caption); }
	.open { flex: none; display: inline-flex; align-items: center; justify-content: center; width: 32px; height: 32px; padding: 0; border: 0; border-radius: 6px; background: none; color: inherit; cursor: pointer; }
	.open:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.season { flex: 1; min-width: 0; display: flex; align-items: center; gap: 10px; min-height: 44px; padding: 0; border: 0; background: none; color: inherit; font: inherit; text-align: left; cursor: pointer; border-radius: 6px; }
	.eps { padding: 0 12px 8px; }
	.eps li { display: flex; align-items: baseline; gap: 10px; padding: 5px 0; font: var(--t-footnote); }
	.eps li + li { border-top: 1px solid var(--line); }
	.eps .n { flex: none; width: 32px; color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.eps .t { white-space: normal; overflow-wrap: anywhere; }
	.eps .later .t { color: var(--text-muted); }
	.eps li { padding: 0; }
	.ep { display: flex; flex: 1; min-width: 0; align-items: center; gap: 8px; min-height: 28px; cursor: pointer; }
	.eps { columns: 2 260px; column-gap: 24px; }
	.eps li { break-inside: avoid; }
	.eps li + li { border-top: 0; }
	.eps .tick { width: 15px; height: 15px; }
	.eps .n { width: 22px; }
	.eps .t { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.eps li.off .t { opacity: 0.55; text-decoration: none; }
	.sub { color: var(--text-muted); font-weight: 400; }
	.tag { flex: none; padding: 1px 6px; border-radius: var(--r-pill); background: var(--surface-3); color: var(--text-muted); font: var(--t-caption); white-space: nowrap; }
	@media (max-width: 420px) { .eps .y { font-size: 11px; } .eps .n { width: 28px; } }
	@media (prefers-reduced-motion: reduce) { .chev { transition: none; } }
	.rowtap { cursor: pointer; }
	.rowtap:hover { background: var(--surface-2); }
	.mk { display: inline-grid; place-items: center; width: 18px; height: 18px; flex: none; color: var(--ok-text, var(--ok)); }
	.mk.no { color: var(--text-muted); }
</style>
