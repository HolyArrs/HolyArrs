<script lang="ts">
	// The Choose step's two big cards' pictures. Apps: HolyArrs in the middle, the apps it imports from around it on dashed lines. Fresh: a tilted wall of the sign-in page's posters.
	import { to } from '#lib/core/paths.ts';
	import { api } from '#lib/core/api.ts';
	import { theme } from '#lib/accounts/index.ts';
	import { ownShots, sized, type ArtShot } from '#lib/accounts/index.ts';

	let { kind }: { kind: 'apps' | 'fresh' } = $props();

	// The five main apps on an oval round the mark, as on the Bring my apps screen.
	const RING = ['radarr', 'sonarr', 'lidarr', 'seerr', 'bazarr'];
	const spots = RING.map((k, i) => {
		const a = (i / RING.length) * Math.PI * 2 - Math.PI / 2;
		return { k, x: 50 + Math.cos(a) * 38, y: 50 + Math.sin(a) * 36, i };
	});
	const logo = (k: string) => to(`/apps/${k === 'radarr' && theme.value === 'light' ? 'radarr-light' : k}.${k === 'bazarr' ? 'png' : 'svg'}`);
	const mark = $derived(to(theme.value === 'light' ? '/brand/mark-color.svg' : '/brand/mark-color-on-dark.svg'));

	// The same posters the sign-in backdrop shows (one read, the smallest size it serves), twelve slots.
	let posters = $state<string[]>([]);
	$effect(() => {
		if (kind !== 'fresh') return;
		void api
			.get<{ shots: ArtShot[] }>('/public/signin-art')
			.then((r) => {
				const list = [...new Set(ownShots(r.shots, true).map((s) => (s.poster ? sized(s.poster.url, 342) : '')).filter(Boolean))];
				if (list.length >= 3) posters = Array.from({ length: 12 }, (_, i) => list[i % list.length]);
			})
			.catch(() => {});
	});
</script>

<span class="art" data-k={kind} aria-hidden="true">
	{#if kind === 'apps'}
		<svg class="wires" viewBox="0 0 100 100" preserveAspectRatio="none">
			{#each spots as s (s.k)}<path d="M50 50L{s.x} {s.y}" />{/each}
		</svg>
		{#each spots as s (s.k)}
			<span class="tile" style="--x: {s.x}%; --y: {s.y}%; --d: {s.i * -0.7}s"><img src={logo(s.k)} alt="" /></span>
		{/each}
		<span class="tile hub"><img src={mark} alt="" /></span>
	{:else}
		<span class="wall">
			{#each posters.length ? posters : Array.from({ length: 12 }, () => '') as src, i (i)}
				{#if src}<img {src} alt="" decoding="async" />{:else}<i></i>{/if}
			{/each}
		</span>
	{/if}
</span>

<style>
	.art { position: absolute; inset: 0; display: block; overflow: hidden; pointer-events: none; container-type: size; --tile-bg: rgb(28 29 33 / 0.9); --tile-line: rgb(255 255 255 / 0.12); --wire: rgb(255 255 255 / 0.28); --shade: rgb(0 0 0 / 0.35); }
	:global(.welcome.over-art[data-theme='light']) .art { --tile-bg: #fff; --tile-line: rgb(0 0 0 / 0.08); --wire: rgb(0 0 0 / 0.25); --shade: rgb(0 0 0 / 0.12); }

	/* Apps: a constellation. */
	.wires { position: absolute; inset: 0; width: 100%; height: 100%; }
	.wires path { fill: none; stroke: var(--wire); stroke-width: 1.5; vector-effect: non-scaling-stroke; stroke-dasharray: 4 6; animation: flow 3s linear infinite; }
	.tile { --s: min(20cqh, 15cqw); position: absolute; width: var(--s); aspect-ratio: 1; left: var(--x); top: var(--y); translate: -50% -50%; display: grid; place-items: center; border-radius: 26%; background: var(--tile-bg); border: 1px solid var(--tile-line); box-shadow: 0 6px 20px var(--shade); animation: bob 6s ease-in-out var(--d, 0s) infinite; }
	.tile img { width: 58%; height: 58%; object-fit: contain; }
	.hub { --s: min(32cqh, 24cqw); left: 50%; top: 50%; z-index: 1; animation: none; }
	.hub img { width: 62%; height: 62%; }
	@keyframes flow { to { stroke-dashoffset: -20; } }
	@keyframes bob { 50% { transform: translateY(-4px); } }

	/* Fresh: a poster wall turned a little, running past every edge. */
	.wall { position: absolute; inset: -30cqh -30cqw; display: grid; grid-template-columns: repeat(6, 1fr); gap: 2.4cqw; align-content: center; transform: rotate(-12deg); }
	.wall > * { display: block; width: 100%; aspect-ratio: 2 / 3; object-fit: cover; border-radius: 10px; background: rgb(var(--ink-rgb, 255 255 255) / 0.08); box-shadow: 0 10px 28px var(--shade); }
	.wall > :nth-child(odd) { transform: translateY(18%); }
	@container (orientation: portrait) { .wall { grid-template-columns: repeat(4, 1fr); } }

	/* A short box (phone): the ring flattens so no app is cut at the top or bottom. */
	@container (max-height: 160px) { .tile { --s: min(28cqh, 13cqw); top: calc(50% + (var(--y) - 50%) * 0.7); } .hub { --s: min(40cqh, 18cqw); top: 50%; } .wires { scale: 1 0.7; } }

	@media (prefers-reduced-motion: reduce) {
		.wires path, .tile { animation: none; }
	}
</style>
