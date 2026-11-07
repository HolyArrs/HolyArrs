<script lang="ts">
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import { defaultRegion } from '#lib/region.ts';
	import { lookFrom, MOVIE_LOOKS, MUSIC_LOOKS, type CaptionLook, type Channels } from '#lib/calendar-captions.ts';

	// The floating glass bar for comparing the calendar's caption lines:
	// ?movie=1-6, ?music=1-5 and ?channels=all|known. The picks are kept for
	// the session; the bar shows once one has been picked by the address or
	// earlier in the session, so plain visits stay as they are (the defaults).
	let { look = $bindable() }: { look: CaptionLook } = $props();

	const KEY = 'holyarrs.calendar.captions';
	const PARAMS = ['movie', 'music', 'channels'];
	let shown = $state(false);

	function kept(): Partial<CaptionLook> | null {
		try {
			return JSON.parse(sessionStorage.getItem(KEY) ?? 'null');
		} catch {
			return null;
		}
	}
	function keep(l: CaptionLook) {
		try {
			sessionStorage.setItem(KEY, JSON.stringify({ movie: l.movie, music: l.music, channels: l.channels }));
		} catch {
			/* storage blocked: the address alone decides */
		}
	}
	$effect(() => {
		const q = page.url.searchParams;
		const k = kept();
		// The local copy is kept, so this effect never reads `look` back (that would loop).
		const l = lookFrom(q, k, defaultRegion());
		look = l;
		if (PARAMS.some((p) => q.has(p)) || k) {
			shown = true;
			keep(l);
		}
	});
	function pick(change: Partial<CaptionLook>) {
		look = { ...look, ...change };
		keep(look);
		const u = new URL(page.url.href);
		u.searchParams.set('movie', String(look.movie));
		u.searchParams.set('music', String(look.music));
		u.searchParams.set('channels', look.channels);
		replaceState(u, page.state);
	}
	const CHANNELS: { value: Channels; label: string }[] = [
		{ value: 'all', label: 'All' },
		{ value: 'known', label: 'Known' }
	];
</script>

{#if shown}
	<div class="cb-room" aria-hidden="true"></div>
	<div class="cb" role="group" aria-label="Calendar caption choices" data-testid="caption-bar">
		<span class="lbl">Movies</span>
		{#each MOVIE_LOOKS as o (o.value)}
			<button type="button" title={o.label} aria-label="Movies: {o.label}" aria-pressed={look.movie === o.value} class:on={look.movie === o.value} onclick={() => pick({ movie: o.value })}>{o.value}</button>
		{/each}
		<span class="sep" aria-hidden="true"></span>
		<span class="lbl">Music</span>
		{#each MUSIC_LOOKS as o (o.value)}
			<button type="button" title={o.label} aria-label="Music: {o.label}" aria-pressed={look.music === o.value} class:on={look.music === o.value} onclick={() => pick({ music: o.value })}>{o.value}</button>
		{/each}
		<span class="sep" aria-hidden="true"></span>
		<span class="lbl">Channels</span>
		{#each CHANNELS as o (o.value)}
			<button type="button" aria-label="Channels: {o.label}" aria-pressed={look.channels === o.value} class:on={look.channels === o.value} onclick={() => pick({ channels: o.value })}>{o.label}</button>
		{/each}
	</div>
{/if}

<style>
	.cb-room { height: 84px; }
	.cb {
		position: fixed;
		z-index: 34;
		left: 50%;
		bottom: calc(18px + env(safe-area-inset-bottom, 0px));
		transform: translateX(-50%);
		display: flex;
		align-items: center;
		gap: 2px;
		max-width: calc(100vw - 24px);
		padding: 0 6px;
		overflow-x: auto;
		scrollbar-width: none;
		border-radius: var(--r-pill);
		background: color-mix(in srgb, var(--surface-1) 72%, transparent);
		-webkit-backdrop-filter: saturate(180%) blur(20px);
		backdrop-filter: saturate(180%) blur(20px);
		box-shadow: 0 0 0 0.5px color-mix(in srgb, var(--text) 14%, transparent), 0 10px 30px rgb(0 0 0 / 0.22);
	}
	.cb::-webkit-scrollbar { display: none; }
	.lbl { flex: none; padding: 0 6px 0 8px; color: var(--text-muted); font: var(--t-footnote); font-weight: 600; white-space: nowrap; }
	.sep { flex: none; width: 1px; height: 22px; margin: 0 4px; background: var(--line); }
	/* Each choice is a 44 px tap target; the visible pill is drawn inside it. */
	button {
		position: relative;
		isolation: isolate;
		flex: none;
		min-width: 40px;
		min-height: 44px;
		padding: 0 12px;
		border: 0;
		border-radius: var(--r-pill);
		background: none;
		color: var(--text-muted);
		font: var(--t-subhead);
		font-weight: 600;
		white-space: nowrap;
		cursor: pointer;
		transition: color var(--dur-fast) var(--ease-out);
	}
	button::before { content: ''; position: absolute; z-index: -1; inset: 4px 0; border-radius: var(--r-pill); transition: background var(--dur-fast) var(--ease-out); }
	button:hover { color: var(--text); }
	button.on { color: var(--bg); }
	button.on::before { background: var(--text); }
	button:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (max-width: 767px) {
		.cb { bottom: calc(var(--tabbar) + 10px + env(safe-area-inset-bottom, 0px)); }
		.cb-room { height: 64px; }
		button { min-width: 36px; padding: 0 10px; font-size: 12.5px; }
	}
	@media (prefers-reduced-motion: reduce) { button, button::before { transition: none; } }
</style>
