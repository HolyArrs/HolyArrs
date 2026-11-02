<script lang="ts">
	// The preview pickers at the bottom of Status: page background and section
	// surface, 1 to 13 each (?bg=, ?surface=), plus pixel headings (?pixel=1). They follow the address.
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { bgOf } from './status.ts';

	const bgs = ['Glow', 'Aurora', 'Grid', 'Pinstripe', 'Spotlight', 'Contours', 'Mesh', 'Carbon', 'Paper', 'Stars', 'CRT', 'Pixel grid', 'Horizon'];
	const surfaces = ['Raised', 'Flat', 'Frosted', 'Sunken', 'Relief', 'Slab', 'Brackets', 'Gradient edge', 'Paper', 'Dashed', 'Window', 'Bevel', 'Console'];
	const bg = $derived(bgOf(page.url.searchParams.get('bg')));
	const surface = $derived(bgOf(page.url.searchParams.get('surface')));
	const pixel = $derived(page.url.searchParams.get('pixel') === '1');

	function pick(key: 'bg' | 'surface' | 'pixel', n: number) {
		const u = new URL(page.url.href);
		if (key === 'pixel' && !n) u.searchParams.delete('pixel');
		else u.searchParams.set(key, String(n));
		void goto(u.pathname + u.search);
	}
</script>

<nav class="pills" aria-label="Status preview" data-testid="status-look-pills">
	<div class="row" role="group" aria-label="Background">
		<span class="lab">Background</span>
		{#each bgs as name, i (name)}
			<button type="button" class="pl" aria-pressed={bg === i + 1} title={name} onclick={() => pick('bg', i + 1)}>{i + 1} · {name}</button>
		{/each}
	</div>
	<div class="row" role="group" aria-label="Surface">
		<span class="lab">Surface</span>
		{#each surfaces as name, i (name)}
			<button type="button" class="pl" aria-pressed={surface === i + 1} title={name} onclick={() => pick('surface', i + 1)}>{i + 1} · {name}</button>
		{/each}
	</div>
	<div class="row" role="group" aria-label="Retro">
		<span class="lab">Retro</span>
		<button type="button" class="pl" aria-pressed={pixel} onclick={() => pick('pixel', pixel ? 0 : 1)}>Pixel headings</button>
		<button type="button" class="pl" onclick={() => { const u = new URL(page.url.href); u.searchParams.set('bg', '11'); u.searchParams.set('surface', '11'); u.searchParams.set('pixel', '1'); u.searchParams.set('cards', 'b'); void goto(u.pathname + u.search); }}>Retro set 1</button>
		<button type="button" class="pl" onclick={() => { const u = new URL(page.url.href); u.searchParams.set('bg', '13'); u.searchParams.set('surface', '13'); u.searchParams.set('pixel', '1'); u.searchParams.set('cards', 'e'); void goto(u.pathname + u.search); }}>Retro set 2</button>
		<button type="button" class="pl" onclick={() => { const u = new URL(page.url.href); u.searchParams.set('bg', '12'); u.searchParams.set('surface', '12'); u.searchParams.set('pixel', '1'); u.searchParams.set('cards', 'j'); void goto(u.pathname + u.search); }}>Retro set 3</button>
	</div>
</nav>

<style>
	.pills {
		position: fixed; left: 50%; z-index: 40; transform: translateX(-50%);
		bottom: calc(14px + env(safe-area-inset-bottom, 0px));
		display: flex; flex-direction: column; gap: 4px; padding: 6px; max-width: calc(100vw - 24px); box-sizing: border-box;
		border: 1px solid var(--line); border-radius: 18px;
		background: color-mix(in srgb, var(--surface-1) 78%, transparent);
		-webkit-backdrop-filter: blur(18px) saturate(1.4); backdrop-filter: blur(18px) saturate(1.4);
		box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 10px 30px -10px rgb(0 0 0 / 0.45);
	}
	.row { display: flex; align-items: center; gap: 2px; overflow-x: auto; scrollbar-width: none; }
	.lab { flex: none; width: 82px; padding-left: 6px; font: 600 11px/1 var(--font-ui); letter-spacing: 0.08em; text-transform: uppercase; color: var(--text-muted); }
	.pl {
		flex: none; height: 32px; padding: 0 11px; border: 0; border-radius: var(--r-pill);
		background: none; color: var(--text-muted); font: 500 12.5px/1 var(--font-ui); white-space: nowrap; cursor: pointer;
	}
	.pl:hover { color: var(--text); }
	.pl[aria-pressed='true'] { background: var(--seg-thumb); color: var(--text); box-shadow: 0 0 0 1px var(--line-strong); }
	.pl:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (max-width: 767px) {
		.pills { bottom: calc(var(--tabbar) + 8px + env(safe-area-inset-bottom, 0px)); }
		.lab { width: 64px; }
	}
</style>
