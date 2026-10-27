<script lang="ts">
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import { tick } from 'svelte';

	// The floating glass pill bar for pages whose layouts Luis is comparing.
	// `?<param>=1…n` picks a layout; `now`/`0` or no parameter is today's page.
	// The pick is kept for the session (sessionStorage), so a reload or a trip
	// to another page and back keeps it. The bar shows once a layout has been
	// picked by the address or earlier in the session; plain visits stay as today.
	let {
		options,
		value = $bindable(0),
		param = 'view',
		storageKey,
		label
	}: {
		/** 0 is today's page; 1…n the layouts. */
		options: { value: number; label: string }[];
		value?: number;
		param?: string;
		storageKey: string;
		label: string;
	} = $props();

	let shown = $state(false);
	let bar: HTMLDivElement | undefined = $state();
	const valid = (v: string | null): number | null => {
		if (v === null) return null;
		if (v === 'now' || v === '0') return 0;
		const n = Number(v);
		return options.some((o) => o.value === n) ? n : null;
	};
	function keep(n: number) {
		try {
			sessionStorage.setItem(storageKey, String(n));
		} catch {
			/* storage blocked: the address alone decides */
		}
	}
	$effect(() => {
		const fromUrl = valid(page.url.searchParams.get(param));
		let kept: number | null = null;
		try {
			kept = valid(sessionStorage.getItem(storageKey));
		} catch {
			/* storage blocked */
		}
		if (fromUrl !== null) {
			value = fromUrl;
			shown = true;
			keep(fromUrl);
		} else if (kept !== null) {
			value = kept;
			shown = true;
		}
	});
	// Keep the picked pill in view when the bar scrolls (narrow phones).
	$effect(() => {
		void value;
		if (!shown || !bar) return;
		// Only the bar scrolls: scrollIntoView would also slide any clipping box of the page around it.
		void tick().then(() => {
			const on = bar?.querySelector<HTMLElement>('[aria-pressed="true"]');
			if (bar && on && bar.scrollWidth > bar.clientWidth) bar.scrollLeft = on.offsetLeft - (bar.clientWidth - on.offsetWidth) / 2;
		});
	});
	function pick(n: number) {
		value = n;
		shown = true;
		keep(n);
		const u = new URL(page.url.href);
		if (n) u.searchParams.set(param, String(n));
		else u.searchParams.set(param, 'now');
		replaceState(u, page.state);
	}
</script>

{#if shown}
	<!-- Room under the page so the bar never covers the last of it. -->
	<div class="vb-room" aria-hidden="true"></div>
	<div class="vb" role="group" aria-label={label} data-testid="variant-bar" bind:this={bar}>
		{#each options as o (o.value)}
			<button type="button" aria-pressed={value === o.value} class:on={value === o.value} onclick={() => pick(o.value)}>{o.label}</button>
		{/each}
	</div>
{/if}

<style>
	.vb-room { height: 84px; }
	.vb {
		position: fixed;
		z-index: 34;
		left: 50%;
		bottom: calc(18px + env(safe-area-inset-bottom, 0px));
		transform: translateX(-50%);
		display: flex;
		gap: 2px;
		max-width: calc(100vw - 24px);
		padding: 0 4px;
		overflow-x: auto;
		scrollbar-width: none;
		border-radius: var(--r-pill);
		background: color-mix(in srgb, var(--surface-1) 72%, transparent);
		-webkit-backdrop-filter: saturate(180%) blur(20px);
		backdrop-filter: saturate(180%) blur(20px);
		box-shadow: 0 0 0 0.5px color-mix(in srgb, var(--text) 14%, transparent), 0 10px 30px rgb(0 0 0 / 0.22);
	}
	.vb::-webkit-scrollbar { display: none; }
	/* Each choice is a 44 px tap target; the visible pill is drawn inside it. */
	button {
		position: relative;
		isolation: isolate;
		flex: none;
		min-height: 44px;
		padding: 0 14px;
		border: 0;
		border-radius: var(--r-pill);
		background: none;
		color: var(--text-muted);
		font: var(--t-subhead);
		font-weight: 600;
		white-space: nowrap;
		cursor: pointer;
		transition: background var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out);
	}
	button::before { content: ''; position: absolute; z-index: -1; inset: 4px 0; border-radius: var(--r-pill); transition: background var(--dur-fast) var(--ease-out); }
	button:hover { color: var(--text); }
	button.on { color: var(--bg); }
	button.on::before { background: var(--text); }
	button:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (max-width: 767px) {
		/* Above the phone tab bar. */
		.vb { bottom: calc(var(--tabbar) + 10px + env(safe-area-inset-bottom, 0px)); }
		.vb-room { height: 64px; }
		button { padding: 0 11px; font-size: 12.5px; }
	}
	@media (prefers-reduced-motion: reduce) { button, button::before { transition: none; } }
</style>
