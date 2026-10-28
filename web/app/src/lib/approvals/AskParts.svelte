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
	import { appPicture } from '../music/music-pages.ts';
	import { albumCounts, albumGroups, askAlbums, askSeasons, keptWords, type PartsPick } from './ask-parts.ts';
	import { askArtist, musicKinds } from './details.svelte.ts';
	import type { ApprovalsCtx } from './approvals.ts';
	import type { Ask } from '../types.ts';

	let { ctx, a, editable = false, pick = $bindable({ seasons: null, albums: null, empty: false }) }: { ctx: ApprovalsCtx; a: Ask; editable?: boolean; pick?: PartsPick } = $props();

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
			const kept = rows.filter((r) => !left.has(r.key));
			pick = { seasons: null, albums: left.size && kept.length ? kept.flatMap((r) => r.ids) : null, empty: rows.length > 0 && kept.length === 0 };
		} else {
			const kept = seasons.filter((s) => !left.has(s.number));
			pick = { seasons: left.size && kept.length ? kept.map((s) => s.number) : null, albums: null, empty: seasons.length > 0 && kept.length === 0 };
		}
	});

	let open = $state(false);
	let openSeason = $state(0);
	let openEditions = $state('');
	const reduced = () => typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
	const slideIn = () => ({ duration: reduced() ? 0 : DUR.base, easing: easeOut });
	const today = new Date().toISOString().slice(0, 10);
	const keptRows = $derived(rows.filter((r) => !left.has(r.key)));
	const keptSeasons = $derived(seasons.filter((s) => !left.has(s.number)));
	const head = $derived.by(() => {
		if (music) {
			if (albumsLoading) return 'Loading the albums…';
			if (!rows.length) return a.album_words ? a.album_words[0].toUpperCase() + a.album_words.slice(1) : 'Every album';
			return left.size ? `${albumCounts(keptRows)} · ${keptRows.length} of ${rows.length} kept` : albumCounts(rows);
		}
		const total = keptSeasons.reduce((n, s) => n + s.episodes, 0);
		return keptWords(keptSeasons.length, seasons.length, 'season', total ? `${total} episodes` : '');
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
				{#if editable}<p class="hint">Untick what not to approve.</p>{/if}
				<ul>
					{#if music}
						{#each rows as r (r.key)}
							<li class:off={left.has(r.key)} data-testid="ask-album">
								<div class="line">
									{#if editable}<input class="tick" type="checkbox" checked={!left.has(r.key)} aria-label="Keep {r.main.name}" onchange={() => toggle(r.key)} />{/if}
									<span class="cover">{#if r.main.image}<img src={r.main.image} alt="" loading="lazy" decoding="async" />{/if}</span>
									<span class="t">{r.main.name}</span>
									{#if r.editions.length}
										<button type="button" class="more" aria-expanded={openEditions === r.key} onclick={() => (openEditions = openEditions === r.key ? '' : r.key)}>{r.editions.length + 1} editions</button>
									{/if}
									<span class="y">{r.main.year ?? ''}</span>
								</div>
								{#if openEditions === r.key}
									<ul class="eds" transition:slide={slideIn()} aria-label="Editions of {r.main.name}">
										{#each r.editions as e (e.mbid)}<li><span class="t">{e.name}</span><span class="y">{e.year ?? ''}</span></li>{/each}
									</ul>
								{/if}
							</li>
						{/each}
					{:else}
						{#each seasons as s (s.number)}
							{@const list = openSeason === s.number ? ctx.episodes(a.tmdb_id, s.number) : null}
							<li class:off={left.has(s.number)} data-testid="ask-season">
								<div class="line">
									{#if editable}<input class="tick" type="checkbox" checked={!left.has(s.number)} aria-label="Keep season {s.number}" onchange={() => toggle(s.number)} />{/if}
									<button type="button" class="season" aria-expanded={openSeason === s.number} onclick={() => (openSeason = openSeason === s.number ? 0 : s.number)}>
										<span class="t">Season {s.number}</span>
										<span class="y">{left.has(s.number) ? 'Left out' : s.episodes ? `${s.episodes} episodes` : ''}</span>
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
													{@const later = !!e.air_day && e.air_day.slice(0, 10) > today}
													<li class:later><span class="n">E{e.number}</span><span class="t">{e.name || `Episode ${e.number}`}</span>{#if later && e.air_day}<span class="y">airs {e.air_day.slice(5, 10).replace('-', '/')}</span>{/if}</li>
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
	.list > ul { max-height: 320px; overflow-y: auto; overscroll-behavior: contain; padding: 4px 0; }
	.list > ul > li + li { border-top: 1px solid var(--line); }
	.line { display: flex; align-items: center; gap: 10px; min-height: 44px; padding: 0 12px; font: var(--t-footnote); }
	.off .t, .off .cover { opacity: 0.45; }
	.off .t { text-decoration: line-through; }
	.tick { width: 18px; height: 18px; flex: none; accent-color: var(--accent-fill); }
	.cover { width: 32px; height: 32px; flex: none; border-radius: 4px; overflow: hidden; background: var(--surface-3); }
	.cover img { width: 100%; height: 100%; object-fit: cover; display: block; }
	.t { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.y { flex: none; color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.more { flex: none; min-height: 24px; padding: 0 8px; border: 0; border-radius: var(--r-pill); background: var(--surface-1); color: var(--text-muted); font: var(--t-caption); font-weight: 600; cursor: pointer; }
	.more[aria-expanded='true'] { background: var(--accent-soft); color: var(--accent-text); }
	.eds { padding: 0 12px 8px 54px; }
	.eds li { display: flex; gap: 10px; padding: 4px 0; color: var(--text-muted); font: var(--t-caption); }
	.season { flex: 1; min-width: 0; display: flex; align-items: center; gap: 10px; min-height: 44px; padding: 0; border: 0; background: none; color: inherit; font: inherit; text-align: left; cursor: pointer; border-radius: 6px; }
	.eps { padding: 0 12px 8px; }
	.eps li { display: flex; align-items: baseline; gap: 10px; padding: 5px 0; font: var(--t-footnote); }
	.eps li + li { border-top: 1px solid var(--line); }
	.eps .n { flex: none; width: 32px; color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.eps .t { white-space: normal; overflow-wrap: anywhere; }
	.eps .later .t { color: var(--text-muted); }
	@media (prefers-reduced-motion: reduce) { .chev { transition: none; } }
</style>
