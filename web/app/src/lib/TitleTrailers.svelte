<script lang="ts">
	import Icon from './Icon.svelte';
	import Shelf from './ui/Shelf.svelte';
	import IconButton from './ui/IconButton.svelte';
	import { reducedMotion } from './motion.ts';
	import { sized } from './tmdb-image.ts';
	import { claimPlayer, pickTrailer, type Video } from './trailer.ts';
	import { trailerSources, type TrailerSource } from './autoplay.svelte.ts';
	import { openTheater } from './theater.svelte.ts';

	// A title's trailers and teasers (YouTube, as TMDB lists them) as one row
	// of small cards, the main trailer first. Each card shows the video's own
	// cover; when that cannot load, one of the title's backdrops. Playing one
	// opens the big player.
	let {
		kind,
		tmdb,
		title,
		videos = [],
		art = [],
		item = '220px',
		framed = false
	}: {
		kind: 'movie' | 'show';
		tmdb: string;
		title: string;
		videos?: Video[];
		art?: (string | null | undefined)[];
		item?: string;
		/** In a rounded panel with a small label, matching the info panel beside it (the cards scroll inside it). */
		framed?: boolean;
	} = $props();

	const main = $derived(pickTrailer(videos));
	const list = $derived(main ? [main, ...videos.filter((v) => v.key !== main.key)] : videos);
	const pics = $derived(art.filter((u): u is string => !!u));
	let noCover = $state<Record<string, boolean>>({});
	const coverOf = (v: Video, i: number) => (noCover[v.key] ? (pics.length ? sized(pics[i % pics.length], 'backdrop', 300) : '') : `https://i.ytimg.com/vi/${v.key}/mqdefault.jpg`);
	const kindWord = (v: Video) => (v.type === 'Teaser' ? 'Teaser' : 'Trailer');
	const langWord = (v: Video) => {
		if (!v.language || v.language === 'en') return '';
		try {
			return new Intl.DisplayNames(['en'], { type: 'language' }).of(v.language) ?? '';
		} catch {
			return '';
		}
	};
	const sub = (v: Video) => [kindWord(v), langWord(v)].filter(Boolean).join(' · ');
	const named = (v: Video) => v.name || `${title} ${kindWord(v).toLowerCase()}`;

	// Framed: pages of whole cards (two, three when the panel is wide). Nothing
	// shows cut at the edge; swiping or the arrows move one page and stop there.
	const GAP = 12;
	let row = $state<HTMLElement>();
	let per = $state(2);
	let page = $state(0);
	const pages = $derived(Math.max(1, Math.ceil(list.length / per)));
	const fillers = $derived((per - (list.length % per)) % per);
	$effect(() => {
		const el = row;
		if (!el) return;
		const measure = () => {
			per = el.clientWidth >= 600 ? 3 : 2;
			const step = el.clientWidth + GAP;
			page = Math.min(pages - 1, Math.round(el.scrollLeft / step));
		};
		measure();
		const ro = new ResizeObserver(measure);
		ro.observe(el);
		el.addEventListener('scroll', measure, { passive: true });
		return () => {
			ro.disconnect();
			el.removeEventListener('scroll', measure);
		};
	});
	function turn(dir: 1 | -1) {
		if (!row) return;
		const to = Math.max(0, Math.min(pages - 1, page + dir));
		row.scrollTo({ left: to * (row.clientWidth + GAP), behavior: reducedMotion() ? 'auto' : 'smooth' });
	}

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
			// One video at a time: the hero's muted trailer stops for this one.
			claimPlayer(() => {})();
			const r = el.getBoundingClientRect();
			openTheater({ kind, tmdb, title, source, time: 0, rect: { left: r.left, top: r.top, width: r.width, height: r.height }, from: 'panel', returnTo: el });
		} finally {
			busy = '';
		}
	}
</script>

{#snippet card(v: Video, i: number)}
	<button type="button" class="tcard" class:snap={i % per === 0} onclick={(e) => play(v, e)} aria-busy={busy === v.key || undefined} aria-label="Play {named(v)}">
		<span class="pic">
			{#if coverOf(v, i)}<img src={coverOf(v, i)} alt="" loading="lazy" decoding="async" onerror={() => (noCover[v.key] = true)} />{/if}
			<span class="glyph" aria-hidden="true"><Icon name="play" size={14} /></span>
		</span>
		<span class="tname">{named(v)}</span>
		<span class="xs muted">{sub(v)}</span>
	</button>
{/snippet}

{#if list.length && framed}
	<section class="tpanel" aria-labelledby="h-trailers" id="title-trailers" data-testid="title-trailers">
		<div class="thead">
			<h2 class="cap" id="h-trailers">Trailers<span class="n">{list.length}</span></h2>
			{#if pages > 1}
				<div class="pager">
					<span class="pos" aria-live="polite">{page + 1} / {pages}</span>
					<IconButton icon="chevronLeft" label="Previous trailers" variant="tinted" size="sm" tooltip={false} disabled={page === 0} onclick={() => turn(-1)} />
					<IconButton icon="chevronRight" label="Next trailers" variant="tinted" size="sm" tooltip={false} disabled={page >= pages - 1} onclick={() => turn(1)} />
				</div>
			{/if}
		</div>
		<div class="trow" bind:this={row} style="--per: {per}; --gap: {GAP}px">
			{#each list as v, i (v.key)}{@render card(v, i)}{/each}
			{#each { length: fillers } as _, i (i)}<span class="fill" aria-hidden="true"></span>{/each}
		</div>
	</section>
{:else if list.length}
	<div class="trailers" data-testid="title-trailers">
		<Shelf title="Trailers" anchor="title-trailers" {item} placeholder="wide">
			{#each list as v, i (v.key)}{@render card(v, i)}{/each}
		</Shelf>
	</div>
{/if}

<style>
	.trailers { min-width: 0; }
	.tcard { display: flex; flex-direction: column; gap: 3px; min-width: 0; padding: 0; border: 0; background: none; color: var(--text); text-align: left; cursor: pointer; }
	.pic { position: relative; display: block; aspect-ratio: 16 / 9; margin-bottom: 5px; border-radius: 9px; overflow: hidden; background: var(--surface-2); }
	.pic img { width: 100%; height: 100%; object-fit: cover; transition: transform 420ms var(--ease-out); }
	.tcard:hover img { transform: scale(1.04); }
	.glyph { position: absolute; left: 8px; bottom: 8px; display: grid; place-items: center; width: 30px; height: 30px; border-radius: 50%; background: rgb(0 0 0 / 45%); border: 1px solid rgb(255 255 255 / 35%); color: #fff; backdrop-filter: blur(10px); -webkit-backdrop-filter: blur(10px); transition: transform 240ms cubic-bezier(0.34, 1.56, 0.64, 1), background 200ms ease; }
	.tcard:hover .glyph { transform: scale(1.1); background: rgb(0 0 0 / 60%); }
	.tcard:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 10px; }
	.tname { font: 600 14px/1.3 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	/* Framed: the info panel's look (surface, line, radius, small capital label). */
	.tpanel { display: flex; flex-direction: column; gap: 14px; height: 100%; min-width: 0; padding: 18px 20px; border-radius: 22px; background: var(--surface-1); border: 1px solid var(--line); }
	.cap { display: flex; align-items: center; gap: 8px; margin: 0; color: var(--text-muted); font: 600 11.5px/1.2 var(--font-ui); letter-spacing: .06em; text-transform: uppercase; }
	.n { padding: 2px 7px; border-radius: 999px; background: var(--surface-2); letter-spacing: 0; }
	.thead { display: flex; align-items: center; justify-content: space-between; gap: 10px; min-height: 30px; }
	.pager { display: inline-flex; align-items: center; gap: 6px; }
	.pos { margin-right: 4px; color: var(--text-muted); font: 600 12px/1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.pager :global(.ui-ibtn:disabled) { opacity: .35; cursor: default; }
	/* Exactly --per whole cards across; a page snaps at its first card and swipes stop page by page. */
	.trow { display: grid; grid-auto-flow: column; grid-auto-columns: calc((100% - (var(--per) - 1) * var(--gap)) / var(--per)); gap: var(--gap); overflow-x: auto; overscroll-behavior-x: contain; scroll-snap-type: x mandatory; scrollbar-width: none; }
	.trow::-webkit-scrollbar { display: none; }
	.trow > .snap { scroll-snap-align: start; scroll-snap-stop: always; }
	.tpanel .pic { border-radius: 12px; }
	@media (prefers-reduced-motion: reduce) { .pic img, .glyph { transition: none; } }
</style>
