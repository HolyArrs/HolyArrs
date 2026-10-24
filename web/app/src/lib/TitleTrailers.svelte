<script lang="ts" module>
	/** Where a title's trailers show while Luis compares them: a row of cards, one featured beside a list, or small pills in the info strip. */
	export type TrailerLayout = 'row' | 'feature' | 'chips';
</script>

<script lang="ts">
	import Icon from './Icon.svelte';
	import Shelf from './ui/Shelf.svelte';
	import { sized } from './tmdb-image.ts';
	import { pickTrailer, type Video } from './trailer.ts';
	import { trailerSources, type TrailerSource } from './autoplay.svelte.ts';
	import { openTheater } from './theater.svelte.ts';

	// A title's trailers and teasers (YouTube, as TMDB lists them), the main
	// trailer first. Each card shows one of the title's own backdrops (pictures
	// come only from TMDB); playing one opens the big player.
	let {
		kind,
		tmdb,
		title,
		videos = [],
		art = [],
		layout = 'row'
	}: { kind: 'movie' | 'show'; tmdb: string; title: string; videos?: Video[]; art?: (string | null | undefined)[]; layout?: TrailerLayout } = $props();

	const main = $derived(pickTrailer(videos));
	const list = $derived(main ? [main, ...videos.filter((v) => v.key !== main.key)] : videos);
	const pics = $derived(art.filter((u): u is string => !!u));
	const picOf = (i: number) => (pics.length ? sized(pics[i % pics.length], 'backdrop', 480) : '');
	const kindWord = (v: Video) => (v.type === 'Teaser' ? 'Teaser' : 'Trailer');
	const langWord = (v: Video) => {
		if (!v.language || v.language === 'en') return '';
		try {
			return new Intl.DisplayNames(['en'], { type: 'language' }).of(v.language) ?? '';
		} catch {
			return '';
		}
	};
	const sub = (v: Video) => [kindWord(v), v.official ? 'Official' : '', langWord(v)].filter(Boolean).join(' · ');

	let busy = $state('');
	async function play(v: Video, e: MouseEvent) {
		const el = e.currentTarget as HTMLElement;
		if (busy) return;
		busy = v.key;
		try {
			// The main trailer can stream straight from the server's pick; the others play in the YouTube player.
			let source: TrailerSource | undefined;
			if (main && v.key === main.key) source = (await trailerSources(kind, tmdb, videos))[0];
			source ??= { kind: 'embed', key: v.key };
			const r = el.getBoundingClientRect();
			openTheater({ kind, tmdb, title, source, time: 0, rect: { left: r.left, top: r.top, width: r.width, height: r.height }, from: 'panel', returnTo: el });
		} finally {
			busy = '';
		}
	}
	const named = (v: Video) => v.name || `${title} ${kindWord(v).toLowerCase()}`;
</script>

{#snippet card(v: Video, i: number, big = false)}
	<button type="button" class="tcard" class:big onclick={(e) => play(v, e)} aria-busy={busy === v.key || undefined} aria-label="Play {named(v)}">
		<span class="pic">
			{#if picOf(i)}<img src={picOf(i)} alt="" loading="lazy" decoding="async" />{/if}
			<span class="glyph" aria-hidden="true"><Icon name="play" size={big ? 26 : 18} /></span>
		</span>
		<span class="tname">{named(v)}</span>
		<span class="xs muted">{sub(v)}</span>
	</button>
{/snippet}

{#if list.length}
	{#if layout === 'chips'}
		<div class="chips-row" data-testid="title-trailers">
			<span class="lbl">Trailers</span>
			<ul class="tchips">
				{#each list as v (v.key)}
					<li><button type="button" class="tchip" onclick={(e) => play(v, e)} aria-busy={busy === v.key || undefined}><Icon name="play" size={13} />{named(v)}</button></li>
				{/each}
			</ul>
		</div>
	{:else if layout === 'feature'}
		<section class="tp-section" aria-labelledby="h-trailers" id="title-trailers" data-testid="title-trailers">
			<h2 id="h-trailers" class="tp-h2">Trailers</h2>
			<div class="feature" class:solo={list.length === 1}>
				{@render card(list[0], 0, true)}
				{#if list.length > 1}
					<ul class="side">
						{#each list.slice(1) as v, i (v.key)}
							<li>
								<button type="button" class="srow" onclick={(e) => play(v, e)} aria-busy={busy === v.key || undefined}>
									<span class="spic">{#if picOf(i + 1)}<img src={picOf(i + 1)} alt="" loading="lazy" decoding="async" />{/if}<span class="glyph sm" aria-hidden="true"><Icon name="play" size={14} /></span></span>
									<span class="stext"><span class="tname">{named(v)}</span><span class="xs muted">{sub(v)}</span></span>
								</button>
							</li>
						{/each}
					</ul>
				{/if}
			</div>
		</section>
	{:else}
		<div data-testid="title-trailers">
			<Shelf title="Trailers" anchor="title-trailers" item="min(320px, 78vw)" placeholder="wide">
				{#each list as v, i (v.key)}{@render card(v, i)}{/each}
			</Shelf>
		</div>
	{/if}
{/if}

<style>
	.tcard { display: flex; flex-direction: column; gap: 4px; min-width: 0; padding: 0; border: 0; background: none; color: var(--text); text-align: left; cursor: pointer; }
	.pic, .spic { position: relative; display: block; aspect-ratio: 16 / 9; border-radius: 10px; overflow: hidden; background: var(--surface-2); }
	.pic { margin-bottom: 6px; }
	.pic img, .spic img { width: 100%; height: 100%; object-fit: cover; filter: brightness(.82); transition: transform 420ms var(--ease-out), filter 240ms ease; }
	.tcard:hover img, .srow:hover img { transform: scale(1.04); filter: brightness(.95); }
	.glyph { position: absolute; left: 50%; top: 50%; display: grid; place-items: center; width: 48px; height: 48px; margin: -24px 0 0 -24px; border-radius: 50%; background: rgb(255 255 255 / 22%); border: 1px solid rgb(255 255 255 / 40%); color: #fff; backdrop-filter: blur(12px); -webkit-backdrop-filter: blur(12px); transition: transform 240ms cubic-bezier(0.34, 1.56, 0.64, 1), background 200ms ease; }
	.big .glyph { width: 68px; height: 68px; margin: -34px 0 0 -34px; }
	.glyph.sm { width: 32px; height: 32px; margin: -16px 0 0 -16px; }
	.tcard:hover .glyph, .srow:hover .glyph { transform: scale(1.08); background: rgb(255 255 255 / 32%); }
	.tcard:focus-visible, .srow:focus-visible, .tchip:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 12px; }
	.tname { font: 600 15px/1.3 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.big .tname { font-size: 17px; }

	/* Featured: the main trailer big, the others in a list beside it. */
	.feature { display: grid; grid-template-columns: minmax(0, 1.7fr) minmax(260px, 1fr); gap: 20px; align-items: start; }
	.feature.solo { grid-template-columns: minmax(0, 720px); }
	.side { display: flex; flex-direction: column; gap: 10px; max-height: 420px; overflow-y: auto; margin: 0; padding: 0; list-style: none; scrollbar-width: thin; }
	.srow { display: grid; grid-template-columns: 136px minmax(0, 1fr); align-items: center; gap: 12px; width: 100%; padding: 0; border: 0; background: none; color: var(--text); text-align: left; cursor: pointer; }
	.stext { display: flex; flex-direction: column; gap: 3px; min-width: 0; }

	/* Chips: one line of small pills in the info strip. */
	.chips-row { display: flex; flex-wrap: wrap; align-items: center; gap: 10px 14px; font: 500 14px/1 var(--font-ui); }
	.lbl { color: var(--text-muted); }
	.tchips { display: flex; flex-wrap: wrap; gap: 8px; margin: 0; padding: 0; list-style: none; }
	.tchip { display: inline-flex; align-items: center; gap: 7px; min-height: 36px; max-width: 280px; padding: 0 14px 0 12px; border: 0; border-radius: 999px; background: var(--surface-2); color: var(--text); font: 500 14px/1 var(--font-ui); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; cursor: pointer; transition: background var(--dur-fast) var(--ease-out); }
	.tchip:hover { background: var(--surface-3, var(--surface-2)); }

	@media (max-width: 899px) { .feature { grid-template-columns: minmax(0, 1fr); } .side { max-height: none; } }
	@media (prefers-reduced-motion: reduce) { .pic img, .spic img, .glyph { transition: none; } }
</style>
