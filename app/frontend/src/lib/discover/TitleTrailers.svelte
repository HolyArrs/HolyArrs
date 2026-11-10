<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import ArtPlaceholder from '#lib/ui/ArtPlaceholder.svelte';
	import { sized } from '../core/tmdb-image.ts';
	import { claimPlayer, pickTrailer, type Video } from '../core/trailer/trailer.ts';
	import { missReason, trailerSources, videoReason, type TrailerSource } from '../core/trailer/autoplay.svelte.ts';
	import { openTheater } from '../core/trailer/theater.svelte.ts';

	// A title's trailers and teasers (YouTube, as TMDB lists them) as one row of small cards, main trailer first. Each card shows the video's own cover, or one of the title's backdrops when that cannot load.
	let {
		kind,
		tmdb,
		title,
		videos = [],
		art = [],
		item = '260px'
	}: {
		kind: 'movie' | 'show';
		tmdb: string;
		title: string;
		videos?: Video[];
		art?: (string | null | undefined)[];
		/** About how wide a card is; the row fits whole cards. */
		item?: string;
	} = $props();

	const main = $derived(pickTrailer(videos));
	const list = $derived(main ? [main, ...videos.filter((v) => v.key !== main.key)] : videos);
	const pics = $derived(art.filter((u): u is string => !!u));
	let noCover = $state<Record<string, boolean>>({});
	// A cover's state per video: loaded, or failed even after the fallback picture.
	let coverState = $state<Record<string, 'ok' | 'bad'>>({});
	const coverOf = (v: Video, i: number) => (noCover[v.key] ? (pics.length ? sized(pics[i % pics.length], 'backdrop', 300) : '') : `https://i.ytimg.com/vi/${v.key}/mqdefault.jpg`);
	const kindWord = (v: Video) => (v.type === 'Teaser' ? t('discover.titleTrailers.teaser') : t('discover.titleTrailers.trailer'));
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

	let busy = $state('');
	async function play(v: Video, e: MouseEvent) {
		const el = e.currentTarget as HTMLElement;
		if (busy) return;
		busy = v.key;
		try {
			// The main trailer can stream straight from the server's pick; the others play in the YouTube player.
			let list: TrailerSource[] = [];
			const isMain = !!main && v.key === main.key;
			if (isMain) list = await trailerSources(kind, tmdb, videos);
			// A main trailer with no stream left plays in the YouTube player, with the server's reason shown if that refuses it too.
			const why = isMain ? missReason(kind, tmdb) : '';
			// When the server said none of the trailers plays here, the player says why at once.
			const [source, ...fallbacks]: TrailerSource[] = list.length ? list : [why ? { kind: 'embed', key: v.key, why, told: videoReason(why) || undefined } : { kind: 'embed', key: v.key }];
			// One video at a time: the hero's muted trailer stops for this one.
			claimPlayer(() => {})();
			const r = el.getBoundingClientRect();
			openTheater({ kind, tmdb, title, what: kindWord(v).toLowerCase(), source, fallbacks, time: 0, rect: { left: r.left, top: r.top, width: r.width, height: r.height }, from: 'panel', returnTo: el });
		} finally {
			busy = '';
		}
	}
</script>

{#snippet card(v: Video, i: number)}
	<button type="button" class="tcard" onclick={(e) => play(v, e)} aria-busy={busy === v.key || undefined} aria-label={t('discover.titleTrailers.play', { v: named(v) })}>
		<span class="pic">
			{#if !coverOf(v, i) || coverState[v.key] !== 'ok'}<ArtPlaceholder name={!coverOf(v, i) || coverState[v.key] === 'bad' ? named(v) : ''} shape="wide" loading={!!coverOf(v, i) && !coverState[v.key]} />{/if}
			{#if coverOf(v, i) && coverState[v.key] !== 'bad'}<img src={coverOf(v, i)} alt="" loading="lazy" decoding="async" onload={() => (coverState[v.key] = 'ok')} onerror={() => (noCover[v.key] ? (coverState[v.key] = 'bad') : (noCover[v.key] = true))} />{/if}
			<span class="glyph" aria-hidden="true"><Icon name="play" size={14} /></span>
		</span>
		<span class="tname">{named(v)}</span>
		<span class="xs muted">{sub(v)}</span>
	</button>
{/snippet}

{#if list.length}
	<div class="trailers" data-testid="title-trailers">
		<Shelf title={t('discover.titleTrailers.trailers')} anchor="title-trailers" {item} placeholder="wide" whole>
			{#each list as v, i (v.key)}{@render card(v, i)}{/each}
		</Shelf>
	</div>
{/if}

<style>
	.trailers { min-width: 0; }
	.tcard { display: flex; flex-direction: column; gap: 3px; min-width: 0; padding: 0; border: 0; background: none; color: var(--text); text-align: left; cursor: pointer; }
	.pic { position: relative; display: block; aspect-ratio: 16 / 9; margin-bottom: 5px; border-radius: 9px; overflow: hidden; background: var(--surface-2); }
	.pic img { position: relative; width: 100%; height: 100%; object-fit: cover; transition: transform var(--dur-slow) var(--ease-out); }
	.tcard:hover img { transform: scale(1.04); }
	.glyph { position: absolute; left: 8px; bottom: 8px; display: grid; place-items: center; width: 30px; height: 30px; border-radius: 50%; background: rgb(0 0 0 / 45%); border: 1px solid rgb(255 255 255 / 35%); color: #fff; backdrop-filter: blur(10px); -webkit-backdrop-filter: blur(10px); transition: transform var(--dur) var(--ease-spring), background var(--dur-fast) var(--ease-out); }
	.tcard:hover .glyph { transform: scale(1.1); background: rgb(0 0 0 / 60%); }
	.tcard:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 10px; }
	.tname { font: 600 14px/1.3 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	@media (prefers-reduced-motion: reduce) { .pic img, .glyph { transition: none; } }
</style>
