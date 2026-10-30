<script lang="ts">
	// What a music or TV ask is made of, folded under one line that opens:
	// an artist ask's albums ("12 albums · 8 singles & EPs", one row per
	// album with its other editions under a small "3 editions" note), or a
	// show ask's seasons (each opens to its episodes). While the ask waits,
	// the approver can untick albums or seasons; `pick` says what is kept
	// (null: all of it, as asked).
	import { slide } from 'svelte/transition';
	import { DUR, easeOut } from '../ui/transitions.ts';
	import Icon from '#lib/Icon.svelte';
	import { appPicture, coverFor } from '../music/music-pages.ts';
	import { albumCounts, albumGroups, songsHead, askAlbums, askSeasons, askedWords, picksChanged, picksHead, seasonPick, type PartsPick, type SeasonPick } from './ask-parts.ts';
	import { albumSongs, artistIdOf, askArtist, musicKinds, songKey, storedParts, type AlbumSong } from './details.svelte.ts';
	import type { ApprovalsCtx } from './approvals.ts';
	import type { Ask } from '../types.ts';

	let { ctx, a, editable = false, unfolded = false, pick = $bindable({ seasons: null, albums: null, empty: false }) }: { ctx: ApprovalsCtx; a: Ask; editable?: boolean; /** Open from the start, with the first season's episodes showing (the Approvals Stage). */ unfolded?: boolean; pick?: PartsPick } = $props();

	const music = $derived(a.kind === 'artist');
	const artist = $derived(music ? askArtist(a) : null);
	const kinds = $derived(music ? musicKinds() : null);
	const cover = (id: string) => appPicture(`/api/v1/music-trending/art/album/${id}`) ?? '';
	const rows = $derived(music && artist ? albumGroups(askAlbums(a, artist.albums, kinds ?? undefined, cover)) : []);
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

	// The approver's song ticks, by album row (absent: every song); only albums in the Library have stored songs.
	let songTicks = $state<Record<string, string[]>>({});
	let openAlbum = $state('');
	$effect(() => {
		void a.id;
		songTicks = {};
	});
	type AlbumPick = { row: (typeof rows)[number]; list: AlbumSong[] | null; ticked: string[]; tick: 'all' | 'some' | 'none' };
	// The lists stored when the ask was made: opening a season or an album shows them at once.
	const kept = $derived(storedParts(a));
	const artistId = $derived(music ? artistIdOf(a) : '');
	const albumPicks = $derived(
		rows.map((r): AlbumPick => {
			// Stored songs first; an album not stored yet asks the album page once it is opened.
			const list = kept?.albums[r.main.mbid] ?? (kept && openAlbum === r.key ? albumSongs(r.main.mbid, artistId) : null);
			const all = list?.map(songKey) ?? [];
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
	function tickSong(p: AlbumPick, id: string) {
		const base = p.tick === 'none' ? [] : p.ticked;
		const t = base.includes(id) ? base.filter((x) => x !== id) : [...base, id];
		songTicks = { ...songTicks, [p.row.key]: t };
		const l = new Set(left);
		l.delete(p.row.key);
		left = l;
	}
	const songLen = (ms?: number) => (ms ? `${Math.floor(ms / 60000)}:${String(Math.round((ms % 60000) / 1000)).padStart(2, '0')}` : '');

	// The approver's episode ticks, by season (absent: as asked).
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
	const shortDay = (d?: string) => (d && /^\d{4}-\d{2}-\d{2}/.test(d) ? new Date(`${d.slice(0, 10)}T12:00:00`).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' }) : '');

	let open = $state(false);
	let openSeason = $state(0);
	$effect(() => {
		if (!unfolded) return;
		open = true;
		if (!openSeason && seasons.length) openSeason = seasons[0].number;
	});
	let openEditions = $state('');
	const reduced = () => typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
	const slideIn = () => ({ duration: reduced() ? 0 : DUR.base, easing: easeOut });
	const today = new Date().toISOString().slice(0, 10);
	const keptRows = $derived(rows.filter((r) => !left.has(r.key)));
	const head = $derived.by(() => {
		if (music) {
			if (albumsLoading) return 'Loading the albums…';
			if (!rows.length) return a.album_words ? a.album_words[0].toUpperCase() + a.album_words.slice(1) : 'Every album';
			const kept = albumPicks.filter((p) => p.tick !== 'none');
			return songsHead(albumCounts(left.size ? kept.map((p) => p.row) : rows), albumPicks.map((p) => ({ all: p.list?.length ?? null, ticked: p.ticked.length })));
		}
		return picksHead(picks);
	});
	const listId = $derived(`parts-${a.id}`);
	const shown = $derived(music ? rows.length > 0 || albumsLoading : seasons.length > 0);
</script>

{#if shown}
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
								<div class="line rowtap" role="button" tabindex="-1" onclick={(e) => { if (!(e.target as HTMLElement).closest('input, button')) openAlbum = openAlbum === r.key ? '' : r.key; }} onkeydown={() => {}}>
									{#if editable}<input class="tick" type="checkbox" checked={p.tick === 'all'} indeterminate={p.tick === 'some'} aria-label="Keep {r.main.name}" onchange={() => tickAlbum(p)} />{/if}
									<span class="cover">{#if r.main.image}<img src={coverFor(r.main.image, 32).src} alt="" loading="lazy" decoding="async" onerror={(e) => ((e.currentTarget as HTMLImageElement).style.display = 'none')} />{/if}</span>
									<span class="t">{r.main.name}</span>
									{#if r.editions.length}
										<button type="button" class="more" aria-expanded={openEditions === r.key} onclick={() => (openEditions = openEditions === r.key ? '' : r.key)}>{r.editions.length + 1} editions</button>
									{/if}
									<span class="y">{p.list?.length ? (p.tick === 'all' ? `${p.list.length} songs` : `${p.ticked.length} of ${p.list.length}`) + (r.main.year ? ` · ${r.main.year}` : '') : (r.main.year ?? '')}</span>
									<button type="button" class="open" aria-expanded={openAlbum === r.key} aria-label="Songs of {r.main.name}" onclick={() => (openAlbum = openAlbum === r.key ? '' : r.key)}><span class="chev" class:up={openAlbum === r.key}><Icon name="chevronDown" size={14} /></span></button>
								</div>
								{#if openAlbum === r.key}
									<div transition:slide={slideIn()}>
										{#if p.list === null}
											<p class="hint">Loading the songs…</p>
										{:else if !p.list.length}
											<p class="hint" data-testid="songs-none">No song list for this album yet.</p>
										{:else}
											<ol class="eps" aria-label="Songs of {r.main.name}">
												{#each p.list as t (songKey(t))}
													{@const on = p.ticked.includes(songKey(t))}
													<li class:off={!on} data-testid="ask-song">
														<label class="ep">
															{#if editable}<input class="tick" type="checkbox" checked={on} aria-label="Song {t.number}" onchange={() => tickSong(p, songKey(t))} />{/if}
															<span class="n">{t.disc > 1 ? `${t.disc}-` : ''}{t.number}</span>
															<span class="t">{t.name || `Track ${t.number}`}</span>
															<span class="y">{songLen(t.length_ms)}</span>
														</label>
													</li>
												{/each}
											</ol>
										{/if}
									</div>
								{/if}
								{#if openEditions === r.key}
									<ul class="eds" transition:slide={slideIn()} aria-label="Editions of {r.main.name}">
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
								<div class="line rowtap" role="button" tabindex="-1" onclick={(e) => { if (!(e.target as HTMLElement).closest('input, button')) openSeason = openSeason === s.number ? 0 : s.number; }} onkeydown={() => {}}>
									{#if editable}<input class="tick" type="checkbox" checked={p.tick === 'all'} indeterminate={p.tick === 'some'} aria-label="Season {s.number}: tick every episode" onchange={() => tickSeason(p)} />{/if}
									<button type="button" class="season" aria-expanded={openSeason === s.number} onclick={() => (openSeason = openSeason === s.number ? 0 : s.number)}>
										<span class="t">Season {s.number}{#if words}<span class="sub"> · {words}</span>{/if}</span>
										<span class="y">{p.tick === 'none' ? 'Left out' : p.ticked.length !== p.asked.length || p.tick === 'some' ? `${p.ticked.length} ticked` : ''}</span>
										<span class="chev" class:up={openSeason === s.number}><Icon name="chevronDown" size={14} /></span>
									</button>
								</div>
								{#if openSeason === s.number}
									<div transition:slide={slideIn()}>
										{#if list === null}
											<p class="hint">Loading season {s.number}…</p>
										{:else if !list.length}
											<p class="hint">No episode list for season {s.number} yet.</p>
										{:else}
											<ol class="eps" aria-label="Season {s.number} episodes">
												{#each list as e (e.number)}
													{@const on = p.ticked.includes(e.number)}
													{@const notAsked = !p.asked.includes(e.number)}
													{@const later = !!e.air_day && e.air_day.slice(0, 10) > today}
													<li class:later class:off={!on} data-testid="ask-episode">
														<label class="ep">
															{#if editable}<input class="tick" type="checkbox" checked={on} aria-label="Episode {e.number}" onchange={() => tickEpisode(p, e.number)} />{/if}
															<span class="n">E{e.number}</span>
															<span class="t">{e.name || `Episode ${e.number}`}</span>
															{#if notAsked}<span class="tag" data-testid="not-asked">not asked</span>{/if}
															<span class="y">{later ? 'airs ' : ''}{shortDay(e.air_day)}</span>
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
	/* Songs and episodes: slim 28px lines, two columns when there is room, so a whole album fits at a glance. */
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
</style>
