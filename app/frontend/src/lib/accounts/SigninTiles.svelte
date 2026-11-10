<script lang="ts">
	import { pixelTrail } from './pixel-trail.ts';
	import type { Way } from './signin-art.ts';

	type Props = {
		setup?: boolean;
		setupTiles?: ('password' | 'plex' | 'jellyfin' | 'emby')[];
		tiles?: { way: Way; label: string }[];
		setupName?: (w: 'password' | 'plex' | 'jellyfin' | 'emby') => string;
		lastWay: Way | '';
		failN: number;
		busy: boolean;
		onsetup?: (w: 'password' | 'plex' | 'jellyfin' | 'emby') => void;
		onpress?: (w: Way) => void;
	};
	let { setup = false, setupTiles = [], tiles = [], setupName = (w) => w, lastWay, failN, busy, onsetup, onpress }: Props = $props();
</script>

{#if setup}
<div class="provs" data-n={setupTiles.length} data-testid="setup-ways">
	{#each setupTiles as w (w)}
		<button class="prov" type="button" use:pixelTrail={{ busy: false, fail: lastWay === w ? failN : 0 }} data-way={w} disabled={busy} onclick={() => onsetup?.(w)}>
			{#if w === 'password'}
				<svg viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="#f5f5f7" stroke-width="1.8" stroke-linejoin="round"><rect x="5" y="10.5" width="14" height="9.5" rx="2.5" /><path d="M8.5 10.5V7.8a3.5 3.5 0 0 1 7 0v2.7" stroke-linecap="round" /></svg>
			{:else if w === 'plex'}
				<svg viewBox="0 0 24 24" aria-hidden="true"><rect width="24" height="24" rx="6" fill="#1f1f1f" /><path d="M8 5h4.6l5 7-5 7H8l5-7z" fill="#E5A00D" /></svg>
			{:else if w === 'jellyfin'}
				<svg viewBox="0 0 24 24" aria-hidden="true"><defs><linearGradient id="jg-setup" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#AA5CC3" /><stop offset="1" stop-color="#00A4DC" /></linearGradient></defs><path d="M12 3c-2 0-9 12.4-8 14.4S20 19.4 21 17.4 14 3 12 3zm0 6c1 0 4.4 6 4 7s-7.6 1-8 0S11 9 12 9z" fill="url(#jg-setup)" /></svg>
			{:else}
				<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 2l9 5v10l-9 5-9-5V7z" fill="#52B54B" /><path d="M10 8.5v7l6-3.5z" fill="#fff" /></svg>
			{/if}
			<span>{setupName(w)}</span>
		</button>
	{/each}
</div>
{:else}
<div class="provs" data-n={tiles.length} data-testid="signin-ways">
	{#each tiles as tl (tl.way)}
		<button class="prov" type="button" use:pixelTrail={{ busy: busy && lastWay === tl.way, fail: lastWay === tl.way ? failN : 0 }} data-way={tl.way} disabled={busy} onclick={() => onpress?.(tl.way)}>
			{#if tl.way === 'plex'}
				<svg viewBox="0 0 24 24" aria-hidden="true"><rect width="24" height="24" rx="6" fill="#1f1f1f" /><path d="M8 5h4.6l5 7-5 7H8l5-7z" fill="#E5A00D" /></svg>
			{:else if tl.way === 'jellyfin'}
				<svg viewBox="0 0 24 24" aria-hidden="true"><defs><linearGradient id="jg" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#AA5CC3" /><stop offset="1" stop-color="#00A4DC" /></linearGradient></defs><path d="M12 3c-2 0-9 12.4-8 14.4S20 19.4 21 17.4 14 3 12 3zm0 6c1 0 4.4 6 4 7s-7.6 1-8 0S11 9 12 9z" fill="url(#jg)" /></svg>
			{:else if tl.way === 'emby'}
				<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 2l9 5v10l-9 5-9-5V7z" fill="#52B54B" /><path d="M10 8.5v7l6-3.5z" fill="#fff" /></svg>
			{:else if tl.way === 'oidc'}
				<svg viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="#f5f5f7" stroke-width="1.8" stroke-linecap="round"><circle cx="8" cy="12" r="4" /><path d="M12 12h9M18 12v3M21 12v2" /></svg>
			{:else}
				<svg viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="#f5f5f7" stroke-width="1.8" stroke-linejoin="round"><path d="M12 3l8 3v6c0 4.5-3.4 8-8 9-4.6-1-8-4.5-8-9V6z" /></svg>
			{/if}
			<span>{tl.label}</span>
		</button>
	{/each}
</div>
{/if}

<style>
	/* The entrance: each part fades and lifts 6 px, 70 ms after the one before; the logo settles from a hair smaller instead. Opacity and transform only, so it stays on the compositor. */
	@media (prefers-reduced-motion: no-preference) {
		:global(.intro) .provs {
			animation: part-in var(--dur-slow) var(--ease-out) calc(90ms + var(--i, 1) * 70ms) both;
		}
}
	:global(.gl-wait) .provs { animation-play-state: paused; }
	:global(.light) .prov { background: rgb(255 255 255 / 0.45); border-color: rgb(255 255 255 / 0.6); color: #1d1d1f; }
	:global(.light) .provs[data-n='1'] .prov { color: #111; }
	:global(.light) .prov svg[stroke='#f5f5f7'] { stroke: #1d1d1f; }
	@media (hover: hover) { :global(.light) .prov:not(:disabled):hover { background: rgb(255 255 255 / 0.65); color: #111; }
}
	@media (hover: hover) and (prefers-reduced-motion: no-preference) {
		/* A soft lift on light glass; the dark mode's deep shadow looks like a smudge here. */
		:global(.light) .prov:not(:disabled):hover { box-shadow: 0 4px 14px rgb(0 0 0 / 0.1), 0 1px 3px rgb(0 0 0 / 0.06); }
}

	/* Provider tiles: one row, mark over name; a lone tile is a wide button. */
	.provs { display: grid; gap: 10px; grid-template-columns: repeat(auto-fit, minmax(0, 1fr)); }
	/* Marks sit on one line across the row: a two-line label ("Single sign-on") grows downward instead of pushing its mark up. */
	.prov {
		position: relative;
		overflow: hidden;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: flex-start;
		gap: 6px;
		min-height: 76px;
		padding: 15px 6px 10px;
		text-align: center;
		border: 1px solid rgb(255 255 255 / 0.08);
		border-radius: 16px;
		background: rgb(118 118 128 / 0.22);
		color: rgb(255 255 255 / 0.78);
		font: 500 12px/1.2 var(--font-ui);
		cursor: pointer;
		transition: transform var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out);
	}
	.prov svg { width: 26px; height: 26px; flex: none; }
	/* "Single / sign-on", never "Single sign- / on". */
	.prov span { text-wrap: balance; }
	.provs[data-n='1'] .prov { flex-direction: row; justify-content: center; padding: 0 6px; min-height: 50px; gap: 10px; font-size: 16px; color: #fff; }
	.prov:active:not(:disabled) { transform: scale(0.97); }
	.prov:disabled { opacity: 0.5; cursor: default; }
	.prov:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.prov { isolation: isolate; }
	.prov > :global(:not(canvas)) { position: relative; z-index: 1; }
	/* Colour on hover for everyone; the lift only when motion is welcome. */
	@media (hover: hover) { .prov:not(:disabled):hover { background: rgb(118 118 128 / 0.34); color: #fff; }
}
	@media (hover: hover) and (prefers-reduced-motion: no-preference) {
		.prov:not(:disabled):hover {
			transform: translateY(-1px);
			box-shadow: 0 6px 22px rgb(0 0 0 / 0.35), 0 0 18px rgb(255 255 255 / 0.08);
		}
		/* Pressed: settle back down, so a click reads as a press. */
		.prov:not(:disabled):active { transform: translateY(0) scale(0.98); box-shadow: none; }
}

	@media (prefers-reduced-motion: reduce) {
		/* A static press: no scale, just a dimmer face; busy keeps the Button spinner. */
		.prov:active:not(:disabled) { transform: none; filter: brightness(0.86); }
}
	/* Phones: the desktop card, floating low over full-bleed art with 24 px of art on every side; it rides above the keyboard (--kb) with a smooth lift. */
	@media (max-width: 600px) {
		.prov { min-height: 48px; }
		.prov { min-height: 64px; padding-top: 11px; }
		.provs[data-n='5'] { gap: 6px; }
		.provs[data-n='5'] .prov { padding-inline: 2px; font-size: 11px; }
		/* Views that do not end on a 44 px link still get room at the bottom edge. */
		:global(.step) > .provs:last-child { margin-bottom: 12px; }
}
</style>
