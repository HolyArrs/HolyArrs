<!-- Banner strip for a title: lazy loads extra art and falls back to a backdrop. -->
<script lang="ts" module>
	import { api } from '../core/api.ts';

	// One request per title, shared by every banner that asks for it.
	const known = new Map<string, Promise<string>>();
	const waiting: (() => void)[] = [];
	// Four at a time so a fast scroll does not flood the server.
	let running = 0;
	function slot(): Promise<void> {
		if (running < 4) {
			running++;
			return Promise.resolve();
		}
		return new Promise((go) => waiting.push(go));
	}
	function done() {
		const next = waiting.shift();
		if (next) next();
		else running--;
	}
	// Failures give an empty banner so the fallback art shows instead.
	export function bannerFor(kind: string, id: string): Promise<string> {
		const key = `${kind}:${id}`;
		let p = known.get(key);
		if (!p) {
			p = slot()
				.then(() => api.get<{ banner?: string }>(`/extras/${kind}/${encodeURIComponent(id)}`))
				.then((r) => r.banner ?? '')
				.catch(() => '')
				.finally(done);
			known.set(key, p);
		}
		return p;
	}
</script>

<script lang="ts">
	import type { TitleView } from '../core/types.ts';
	import { bannerAspect, bannerSrc } from '#lib/ui/library-art.ts';

	let { title, eager = false, width = 0 }: { title: TitleView; eager?: boolean; width?: number } = $props();

	let host = $state<HTMLElement | null>(null);
	let near = $state(false);
	let banner = $state<string | null>(null);
	let loadedSrc = $state('');
	let broken = $state(false);
	let cw = $state(0);

	const extrasKind = $derived(title.kind === 'artist' ? 'artist' : title.kind === 'show' ? 'show' : 'movie');
	const extrasId = $derived(title.kind === 'artist' ? (title.artist?.mbid ?? '') : (title.ids?.tmdb ?? ''));
	const fallback = $derived(title.backdrop || title.artwork[0] || '');
	const shown = $derived(width || cw);
	const src = $derived(bannerSrc(banner && !broken ? banner : fallback, shown));
	const logoSrc = $derived(bannerSrc(title.logo ?? '', shown * 0.5));
	const usingBanner = $derived(!!banner && !broken);
	const loaded = $derived(!!src && loadedSrc === src);

	// Wait until the banner is near the viewport before asking for art.
	$effect(() => {
		if (eager) {
			near = true;
			return;
		}
		if (!host || typeof IntersectionObserver === 'undefined') {
			near = true;
			return;
		}
		const io = new IntersectionObserver((es) => {
			if (es.some((e) => e.isIntersecting)) {
				near = true;
				io.disconnect();
			}
		}, { rootMargin: '400px 0px' });
		io.observe(host);
		return () => io.disconnect();
	});

	// The live flag drops a late answer after the title changed.
	$effect(() => {
		if (!near) return;
		const id = extrasId;
		if (!id) {
			banner = '';
			return;
		}
		let live = true;
		void bannerFor(extrasKind, id).then((b) => {
			if (live) banner = b;
		});
		return () => {
			live = false;
		};
	});
</script>

<span class="banner" class:ready={banner !== null && (loaded || !src)} class:wide={usingBanner} class:music={title.kind === 'artist'} style:--banner-ratio={bannerAspect(title.kind)} bind:this={host} bind:clientWidth={cw} aria-hidden="true">
	{#if banner !== null && src}
		<img {src} alt="" decoding="async" onload={(e) => (loadedSrc = (e.currentTarget as HTMLImageElement).getAttribute('src') ?? '')} onerror={() => (usingBanner ? (broken = true) : (loadedSrc = src))} />
	{/if}
	{#if banner !== null && !usingBanner}
		<span class="veil"></span>
		{#if title.logo}<img class="logo" src={logoSrc} alt="" loading="lazy" decoding="async" />{:else}<span class="name">{title.name}</span>{/if}
	{/if}
	<span class="sk"></span>
</span>

<style>
	.banner { position: relative; display: block; width: 100%; aspect-ratio: var(--banner-ratio, 1000 / 185); overflow: hidden; border-radius: var(--banner-r, 6px); background: var(--surface-2); isolation: isolate; }
	img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; object-position: center 30%; opacity: 0; transition: opacity var(--dur) var(--ease-out), filter var(--dur-fast, 160ms) var(--ease-out); }
	.ready img { opacity: 1; }

	/* Album art keeps faces a little lower than film banners. */
	.music img:not(.logo) { object-position: center 35%; }
	.veil { position: absolute; inset: 0; background: linear-gradient(90deg, rgb(0 0 0 / 72%) 0%, rgb(0 0 0 / 35%) 55%, rgb(0 0 0 / 10%) 100%); }
	.logo { inset: 14% auto 14% 5%; width: auto; max-width: 48%; height: 72%; object-fit: contain; object-position: left center; filter: drop-shadow(0 1px 6px rgb(0 0 0 / 60%)); }
	.name { position: absolute; left: 5%; right: 30%; top: 50%; translate: 0 -50%; color: #fff; font: 700 clamp(15px, 1.3vw, 22px)/1.15 var(--font-ui); letter-spacing: -0.01em; text-shadow: 0 1px 6px rgb(0 0 0 / 70%); display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
	/* Shimmer shows until the image is ready. */
	.sk { position: absolute; inset: 0; z-index: 2; pointer-events: none; background: linear-gradient(100deg, transparent 25%, var(--surface-3) 50%, transparent 75%); transform: translateX(-100%); animation: sweep 1.4s ease-in-out infinite; }
	.ready .sk { display: none; }
	@keyframes sweep { to { transform: translateX(100%); } }
	@media (prefers-reduced-motion: reduce) { .sk { animation: none; transform: none; opacity: .5; } img { transition: none; } }
</style>
