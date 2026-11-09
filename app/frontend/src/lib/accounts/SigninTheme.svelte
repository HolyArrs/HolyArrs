<script lang="ts">
	import IconButton from '#lib/ui/IconButton.svelte';
	import { setTheme, theme } from './theme.svelte.ts';

	let { panelEl }: { panelEl: HTMLElement | undefined } = $props();
</script>

<!-- Light or dark, quietly in the top-right corner; it changes this device's appearance. -->
<span class="mode" data-theme={theme.value}>
	<IconButton
		icon={theme.value === 'light' ? 'moon' : 'sun'}
		variant="glass"
		tipSide="below-end"
		label={theme.value === 'light' ? 'Dark appearance' : 'Light appearance'}
		onclick={() => {
			// Switch in one frame: no 150 ms fades on rows and fields while the glass changes at once.
			panelEl?.classList.add('swap');
			setTheme(theme.value === 'light' ? 'dark' : 'light');
			requestAnimationFrame(() => requestAnimationFrame(() => panelEl?.classList.remove('swap')));
		}}
	/>
</span>

<style>
	/* The appearance switch: inside the frame's top-right corner, above the art. */
	.mode { position: absolute; z-index: 3; top: calc(var(--fr) + 14px); right: calc(var(--fr) + 14px); }
	:global(.wrap.leaving) .mode { display: none; }
	/* Barely there, like a watermark: a faint icon with no fill until hover or focus. */
	.mode :global(.ui-ibtn.glass) {
		color: rgb(255 255 255 / 0.5);
		background: transparent;
		border-color: transparent;
		box-shadow: none;
		-webkit-backdrop-filter: none;
		backdrop-filter: none;
		filter: drop-shadow(0 1px 2px rgb(0 0 0 / 0.35));
		transition: color var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out);
	}
	.mode :global(.ui-ibtn.glass:focus-visible) { color: rgb(255 255 255 / 0.9); background: rgb(255 255 255 / 0.1); }
	/* Hover: a soft dark disc and a full-white icon, readable over bright art too. */
	@media (hover: hover) { .mode :global(.ui-ibtn.glass:hover) { color: #fff; background: rgb(0 0 0 / 0.3); }
}
	.mode :global(.ui-ibtn.glass:focus-visible) { outline: none; box-shadow: 0 0 0 2px rgb(255 255 255 / 0.85); }
	/* Phones: the desktop card, floating low over full-bleed art with 24 px of art on every side; it rides above the keyboard (--kb) with a smooth lift. */
	@media (max-width: 600px) {
		.mode { top: max(14px, env(safe-area-inset-top)); right: max(14px, env(safe-area-inset-right)); }
}
</style>
