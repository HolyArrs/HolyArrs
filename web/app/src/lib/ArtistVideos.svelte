<script lang="ts">
	import Icon from './Icon.svelte';
	import Shelf from './ui/Shelf.svelte';
	import { api } from './api.ts';
	import { claimPlayer } from './trailer.ts';
	import { openTheater } from './theater.svelte.ts';

	// An artist's music videos (GET /music/artists/{mbid}?part=videos) as one row
	// of small cards, the same row as a title page's trailers: whole cards, each
	// with the video's own cover (the artist's picture when it cannot load).
	// Playing one opens the big player on the YouTube embed.
	let { mbid, artist, picture }: { mbid: string; artist: string; picture?: string | null } = $props();

	type Item = { id: string; title?: string; duration?: number };
	let list = $state<Item[]>([]);
	$effect(() => {
		const want = mbid;
		const ctl = new AbortController();
		list = [];
		api.get<{ videos?: Item[] }>(`/music/artists/${encodeURIComponent(want)}?part=videos`, ctl.signal)
			.then((r) => want === mbid && (list = r.videos ?? []))
			.catch(() => {
				/* An extra row: hidden when it cannot be read. */
			});
		return () => ctl.abort();
	});

	let noCover = $state<Record<string, boolean>>({});
	const coverOf = (v: Item) => (noCover[v.id] ? (picture ?? '') : `https://i.ytimg.com/vi/${v.id}/mqdefault.jpg`);
	// The artist's name is usually in the title ("Radiohead - Creep"); the card says it once, above.
	const named = (v: Item) => (v.title ?? '').replace(new RegExp(`^${artist.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}\\s*[-–—:|]\\s*`, 'i'), '').trim() || `${artist} music video`;
	const length = (s?: number) => (s ? `${Math.floor(s / 60)}:${String(Math.round(s % 60)).padStart(2, '0')}` : '');

	function play(v: Item, e: MouseEvent) {
		const el = e.currentTarget as HTMLElement;
		// One video at a time: the hero's own video stops for this one.
		claimPlayer(() => {})();
		const r = el.getBoundingClientRect();
		openTheater({ kind: 'artist', tmdb: mbid, title: named(v), source: { kind: 'embed', key: v.id }, time: 0, rect: { left: r.left, top: r.top, width: r.width, height: r.height }, from: 'panel', returnTo: el });
	}
</script>

{#if list.length}
	<div class="videos" data-testid="artist-videos">
		<Shelf title="Music videos" anchor="artist-videos" item="260px" placeholder="wide" whole>
			{#each list as v (v.id)}
				<button type="button" class="vcard" onclick={(e) => play(v, e)} aria-label="Play {named(v)}">
					<span class="pic">
						{#if coverOf(v)}<img src={coverOf(v)} alt="" loading="lazy" decoding="async" onerror={() => (noCover[v.id] = true)} />{/if}
						<span class="glyph" aria-hidden="true"><Icon name="play" size={14} /></span>
						{#if v.duration}<span class="len">{length(v.duration)}</span>{/if}
					</span>
					<span class="vname">{named(v)}</span>
					<span class="xs muted">Music video</span>
				</button>
			{/each}
		</Shelf>
	</div>
{/if}

<style>
	.videos { min-width: 0; }
	.vcard { display: flex; flex-direction: column; gap: 3px; min-width: 0; padding: 0; border: 0; background: none; color: var(--text); text-align: left; cursor: pointer; }
	.pic { position: relative; display: block; aspect-ratio: 16 / 9; margin-bottom: 5px; border-radius: 9px; overflow: hidden; background: var(--surface-2); }
	.pic img { width: 100%; height: 100%; object-fit: cover; transition: transform 420ms var(--ease-out); }
	.vcard:hover img { transform: scale(1.04); }
	.glyph { position: absolute; left: 8px; bottom: 8px; display: grid; place-items: center; width: 30px; height: 30px; border-radius: 50%; background: rgb(0 0 0 / 45%); border: 1px solid rgb(255 255 255 / 35%); color: #fff; backdrop-filter: blur(10px); -webkit-backdrop-filter: blur(10px); transition: transform 240ms cubic-bezier(0.34, 1.56, 0.64, 1), background 200ms ease; }
	.vcard:hover .glyph { transform: scale(1.1); background: rgb(0 0 0 / 60%); }
	.len { position: absolute; right: 8px; bottom: 8px; padding: 2px 6px; border-radius: 6px; background: rgb(0 0 0 / 60%); color: #fff; font: 600 11.5px/1.3 var(--font-ui); font-variant-numeric: tabular-nums; }
	.vcard:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 10px; }
	.vname { font: 600 14px/1.3 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	@media (prefers-reduced-motion: reduce) { .pic img, .glyph { transition: none; } }
</style>
