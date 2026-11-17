<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { setLang, type LocaleChoice } from '#lib/core/i18n/i18n.svelte.ts';
	import IconButton from '#lib/ui/IconButton.svelte';
	import LanguagePicker from './LanguagePicker.svelte';
	import { setTheme, theme } from './theme.svelte.ts';
	import { pickBeforeSignIn } from './profile.svelte.ts';

	let { panelEl }: { panelEl: HTMLElement | undefined } = $props();

	// The pick changes this device's words now and is kept for when they sign in.
	function choose(c: LocaleChoice) {
		pickBeforeSignIn();
		void setLang(c);
	}
</script>

<!-- Language and light or dark, quietly in the top corner; they change this device's words and look. -->
<span class="mode" data-theme={theme.value}>
	<IconButton
		icon={theme.value === 'light' ? 'moon' : 'sun'}
		variant="glass"
		tipSide="below-end"
		label={theme.value === 'light' ? t('accounts.signin.dark') : t('accounts.signin.light')}
		onclick={() => {
			// Switch in one frame: no 150 ms fades on rows and fields while the glass changes at once.
			panelEl?.classList.add('swap');
			setTheme(theme.value === 'light' ? 'dark' : 'light');
			requestAnimationFrame(() => requestAnimationFrame(() => panelEl?.classList.remove('swap')));
		}}
	/>
	<LanguagePicker id="language-signin" variant="signin" align="end" onchoose={choose} />
</span>

<style>
	/* The appearance switch: inside the frame's top-right corner, above the art. */
	.mode { position: absolute; z-index: 3; top: calc(var(--fr) + 14px); inset-inline-end: calc(var(--fr) + 14px); display: flex; align-items: center; gap: 8px; }
	:global(.wrap.leaving) .mode { display: none; }
	/* Desktop language button: globe and the current language, quiet like the corner links. */
	.mode :global(.trigger.button) { min-height: 36px; padding: 0 10px; gap: 6px; border-radius: var(--r-sm); background: transparent; color: rgb(255 255 255 / 0.72); font: 500 14px/1 var(--font-ui); filter: drop-shadow(0 1px 2px rgb(0 0 0 / 0.35)); transition: color var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out); }
	@media (hover: hover) { .mode :global(.trigger.button:hover), .mode :global(.trigger.button[aria-expanded='true']) { color: #fff; background: rgb(0 0 0 / 0.3); } }
	.mode :global(.trigger.button:focus-visible) { outline: none; color: #fff; box-shadow: 0 0 0 2px rgb(255 255 255 / 0.85); }
	/* Barely there, like a watermark: a faint icon with no fill until hover or focus. */
	.mode :global(.ui-ibtn.glass), .mode :global(.trigger.icon) {
		color: rgb(255 255 255 / 0.5);
		background: transparent;
		border-color: transparent;
		box-shadow: none;
		-webkit-backdrop-filter: none;
		backdrop-filter: none;
		filter: drop-shadow(0 1px 2px rgb(0 0 0 / 0.35));
		transition: color var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out);
	}
	.mode :global(.ui-ibtn.glass:focus-visible), .mode :global(.trigger.icon:focus-visible) { color: rgb(255 255 255 / 0.9); background: rgb(255 255 255 / 0.1); }
	/* Hover: a soft dark disc and a full-white icon, readable over bright art too. */
	@media (hover: hover) { .mode :global(.ui-ibtn.glass:hover), .mode :global(.trigger.icon:hover), .mode :global(.trigger.icon[aria-expanded='true']) { color: #fff; background: rgb(0 0 0 / 0.3); }
}
	.mode :global(.ui-ibtn.glass:focus-visible), .mode :global(.trigger.icon:focus-visible) { outline: none; box-shadow: 0 0 0 2px rgb(255 255 255 / 0.85); }
	/* Touch screens: a 44 px tap for the language button too (the 36 px above beats LanguagePicker's own touch rule). */
	@media (pointer: coarse) { .mode :global(.trigger.button) { min-height: 44px; } }
	/* Phones: the desktop card, floating low over full-bleed art with 24 px of art on every side; it rides above the keyboard (--kb) with a smooth lift. */
	@media (max-width: 600px) {
		.mode { top: max(14px, env(safe-area-inset-top)); inset-inline-end: max(14px, env(safe-area-inset-right)); }
}
</style>
