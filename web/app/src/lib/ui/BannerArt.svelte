<script lang="ts" module>
	import { api } from '../api.ts';

	// One answer per title for the whole visit; at most four asks at a time
	// so a fast scroll through a big Library never floods the server.
	const known = new Map<string, Promise<string>>();
	const waiting: (() => void)[] = [];
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
	import type { TitleView } from '../types.ts';

	// The wide banner at the start of a Library row: fanart's banner when it
	// has one, else the backdrop cropped wide with the title's logo (or name)
	// over it. Asked for only when the row comes near the screen.
	let { title, eager = false }: { title: TitleView; eager?: boolean } = $props();

	let host = $state<HTMLElement | null>(null);
	let near = $state(false);
	let banner = $state<string | null>(null);
	let loaded = $state(false);
	let broken = $state(false);

	const extrasKind = $derived(title.kind === 'artist' ? 'artist' : title.kind === 'show' ? 'show' : 'movie');
	const extrasId = $derived(title.kind === 'artist' ? (title.artist?.mbid ?? '') : (title.ids?.tmdb ?? ''));
	const fallback = $derived(title.backdrop || title.artwork[0] || '');
	const src = $derived(banner && !broken ? banner : fallback);
	const usingBanner = $derived(!!banner && !broken);

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
	$effect(() => {
		src;
		loaded = false;
	});
</script>

<span class="banner" class:ready={banner !== null && (loaded || !src)} class:wide={usingBanner} bind:this={host} aria-hidden="true">
	{#if banner !== null && src}
		<img {src} alt="" loading="lazy" decoding="async" onload={() => (loaded = true)} onerror={() => (usingBanner ? (broken = true) : (loaded = true))} />
	{/if}
	{#if banner !== null && !usingBanner}
		<span class="veil"></span>
		{#if title.logo}<img class="logo" src={title.logo} alt="" loading="lazy" decoding="async" />{:else}<span class="name">{title.name}</span>{/if}
	{/if}
	<span class="sk"></span>
</span>

<style>
	.banner { position: relative; display: block; width: 100%; aspect-ratio: 37 / 7; overflow: hidden; border-radius: 8px; background: var(--surface-2); isolation: isolate; }
	img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; object-position: center 30%; opacity: 0; transition: opacity 240ms var(--ease-out); }
	.ready img { opacity: 1; }
	.veil { position: absolute; inset: 0; background: linear-gradient(90deg, rgb(0 0 0 / 72%) 0%, rgb(0 0 0 / 35%) 55%, rgb(0 0 0 / 10%) 100%); }
	.logo { inset: 14% auto 14% 5%; width: auto; max-width: 48%; height: 72%; object-fit: contain; object-position: left center; filter: drop-shadow(0 1px 6px rgb(0 0 0 / 60%)); }
	.name { position: absolute; left: 5%; right: 30%; top: 50%; translate: 0 -50%; color: #fff; font: 700 15px/1.15 var(--font-ui); letter-spacing: -0.01em; text-shadow: 0 1px 6px rgb(0 0 0 / 70%); display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
	.sk { position: absolute; inset: 0; z-index: 2; pointer-events: none; background: linear-gradient(100deg, transparent 25%, var(--surface-3) 50%, transparent 75%); transform: translateX(-100%); animation: sweep 1.4s ease-in-out infinite; }
	.ready .sk { display: none; }
	@keyframes sweep { to { transform: translateX(100%); } }
	@media (prefers-reduced-motion: reduce) { .sk { animation: none; transform: none; opacity: .5; } img { transition: none; } }
</style>
