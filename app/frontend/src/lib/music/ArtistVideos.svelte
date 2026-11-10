<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import { api } from '../core/api.ts';
	import { claimPlayer } from '../core/trailer/trailer.ts';
	import { openTheater } from '../core/trailer/theater.svelte.ts';
	import { pickedSources, type HeroVideo } from '../core/trailer/hero-video.ts';

	// An artist's music videos (GET /music/artists/{mbid}?part=videos) as one row of small cards, the same row as a title page's trailers: whole cards, each with the video's own cover (the artist's picture when it cannot load).
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
	const named = (v: Item) => (v.title ?? '').replace(new RegExp(`^${artist.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}\\s*[-–—:|]\\s*`, 'i'), '').trim() || t('music.musicVideo', { artist });
	const length = (s?: number) => (s ? `${Math.floor(s / 60)}:${String(Math.round(s % 60)).padStart(2, '0')}` : '');

	// Pointing at a card or focusing it reads the stream the server keeps (no lookup); only a click asks the server to look that one video up (play=1). A miss is asked again next time, and so is a relayed stream past its end.
	const streams = new Map<string, Promise<HeroVideo | null>>();
	function stream(id: string, click = false): Promise<HeroVideo | null> {
		const k = `${mbid}|${id}|${click ? 'play' : 'kept'}`;
		let p = streams.get(k);
		if (!p) {
			p = api.get<HeroVideo>(`/music/artists/${encodeURIComponent(mbid)}?part=video&id=${encodeURIComponent(id)}${click ? '&play=1' : ''}`).catch(() => null);
			streams.set(k, p);
			void p.then((s) => {
				if (!s) streams.delete(k);
				else if (s.expires_at) setTimeout(() => streams.delete(k), Math.max(0, Date.parse(s.expires_at) - Date.now() - 60_000));
			});
		}
		return p;
	}

	let busy = $state('');
	async function play(v: Item, e: MouseEvent) {
		const el = e.currentTarget as HTMLElement;
		if (busy) return;
		busy = v.id;
		try {
			const kept = await stream(v.id);
			const [source, ...fallbacks] = pickedSources(v.id, kept ?? (await stream(v.id, true)));
			// One video at a time: the hero's own video stops for this one.
			claimPlayer(() => {})();
			const r = el.getBoundingClientRect();
			openTheater({ kind: 'artist', tmdb: mbid, title: named(v), source, fallbacks, time: 0, rect: { left: r.left, top: r.top, width: r.width, height: r.height }, from: 'panel', returnTo: el });
		} finally {
			busy = '';
		}
	}
</script>

{#if list.length}
	<div class="videos" data-testid="artist-videos">
		<Shelf title={t('music.musicVideos')} anchor="artist-videos" item="260px" placeholder="wide" whole>
			{#each list as v (v.id)}
				<button type="button" class="vcard" onclick={(e) => play(v, e)} onpointerenter={() => void stream(v.id)} onfocus={() => void stream(v.id)} aria-busy={busy === v.id || undefined} aria-label={t('music.play', { name: named(v) })}>
					<span class="pic">
						{#if coverOf(v)}<img src={coverOf(v)} alt="" loading="lazy" decoding="async" onerror={() => (noCover[v.id] = true)} />{/if}
						<span class="glyph" aria-hidden="true"><Icon name="play" size={14} /></span>
						{#if v.duration}<span class="len">{length(v.duration)}</span>{/if}
					</span>
					<span class="vname">{named(v)}</span>
					<span class="xs muted">{t('music.musicVideo2')}</span>
				</button>
			{/each}
		</Shelf>
	</div>
{/if}

<style>
	.videos { min-width: 0; }
	.vcard { display: flex; flex-direction: column; gap: 3px; min-width: 0; padding: 0; border: 0; background: none; color: var(--text); text-align: left; cursor: pointer; }
	.pic { position: relative; display: block; aspect-ratio: 16 / 9; margin-bottom: 5px; border-radius: 9px; overflow: hidden; background: var(--surface-2); }
	.pic img { width: 100%; height: 100%; object-fit: cover; transition: transform var(--dur-slow) var(--ease-out); }
	.vcard:hover img { transform: scale(1.04); }
	.glyph { position: absolute; left: 8px; bottom: 8px; display: grid; place-items: center; width: 30px; height: 30px; border-radius: 50%; background: rgb(0 0 0 / 45%); border: 1px solid rgb(255 255 255 / 35%); color: #fff; backdrop-filter: blur(10px); -webkit-backdrop-filter: blur(10px); transition: transform var(--dur) var(--ease-spring), background var(--dur-fast) var(--ease-out); }
	.vcard:hover .glyph { transform: scale(1.1); background: rgb(0 0 0 / 60%); }
	.len { position: absolute; right: 8px; bottom: 8px; padding: 2px 6px; border-radius: 6px; background: rgb(0 0 0 / 60%); color: #fff; font: 600 11.5px/1.3 var(--font-ui); font-variant-numeric: tabular-nums; }
	.vcard:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 10px; }
	.vname { font: 600 14px/1.3 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	@media (prefers-reduced-motion: reduce) { .pic img, .glyph { transition: none; } }
</style>
