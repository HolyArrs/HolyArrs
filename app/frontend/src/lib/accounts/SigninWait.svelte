<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	let { cancelPlex }: { cancelPlex: () => void } = $props();
</script>

	<div class="wait" role="status" aria-live="polite" data-testid="plex-wait">
		<div class="ring" aria-hidden="true">
			<span class="orbit"></span>
			<svg viewBox="0 0 24 24"><rect width="24" height="24" rx="6" fill="#1f1f1f" /><path d="M8 5h4.6l5 7-5 7H8l5-7z" fill="#E5A00D" /></svg>
		</div>
		<h1 class="title">{t('accounts.signin.waitTitle')}</h1>
		<p class="lead">{t('accounts.signin.waitLead')}</p>
		<button class="link" type="button" onclick={cancelPlex}>{t('accounts.signin.cancel')}</button>
	</div>

<style>
	/* The entrance: each part fades and lifts 6 px, 70 ms after the one before; the logo settles from a hair smaller instead. Opacity and transform only, so it stays on the compositor. */
	@media (prefers-reduced-motion: no-preference) {
		:global(.intro) .title, :global(.intro) .lead, :global(.intro .step) .link {
			animation: part-in var(--dur-slow) var(--ease-out) calc(90ms + var(--i, 1) * 70ms) both;
		}
}
	:global(.gl-wait) .title, :global(.gl-wait) .lead, :global(.gl-wait .step) .link { animation-play-state: paused; }
	:global(.light) .title { color: #111; }
	:global(.light) .lead { color: rgb(0 0 0 / 0.64); }
	:global(.light) .link {
		color: #1d1d1f;
		font-weight: 600;
		text-decoration: underline;
		text-underline-offset: 3px;
		text-decoration-color: rgb(0 0 0 / 0.25);
	}
	:global(.light) .orbit { border-color: rgb(0 0 0 / 0.14); border-top-color: #e5a00d; }
	.title { font: var(--t-title1); letter-spacing: var(--t-title1-track); text-align: center; color: #fff; text-wrap: balance; }
	.lead { margin: 8px 0 20px; color: rgb(255 255 255 / 0.68); font: var(--t-callout); text-align: center; text-wrap: pretty; }
	.title + :not(.lead) { margin-top: 26px; }

	.link {
		display: block;
		margin: 12px auto 0;
		padding: 0 10px;
		min-height: 36px;
		border: 0;
		border-radius: var(--r-sm);
		background: none;
		color: var(--accent-text);
		font: 500 14px/1 var(--font-ui);
		cursor: pointer;
	}
	.link:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	/* Text links, the help summary and Copy: a faint wash on hover, a firmer one on press. */
	@media (hover: hover) {
		.link:hover { background: rgb(255 255 255 / 0.08); }
		:global(.light) .link:hover { background: rgb(0 0 0 / 0.05); }
}
	.link:active { background: rgb(255 255 255 / 0.14); }
	:global(.light) .link:active { background: rgb(0 0 0 / 0.09); }

	/* The waiting step: the provider's mark inside a turning ring. */
	.wait { text-align: center; }
	.ring { position: relative; width: 64px; height: 64px; margin: 4px auto 16px; display: grid; place-items: center; }
	.ring svg { width: 34px; height: 34px; }
	.orbit {
		position: absolute;
		inset: 0;
		border-radius: 50%;
		border: 2px solid rgb(255 255 255 / 0.14);
		border-top-color: #e5a00d;
		animation: orbit 0.9s linear infinite;
	}

	@media (prefers-reduced-motion: reduce) {
		.orbit { animation-duration: 2.4s; }
}
	/* Phones: the desktop card, floating low over full-bleed art with 24 px of art on every side; it rides above the keyboard (--kb) with a smooth lift. */
	@media (max-width: 600px) {
		.title { font-size: 20px; line-height: 1.15; }
		.lead { margin: 4px 0 10px; }
		.title + :not(.lead) { margin-top: 16px; }
		.link { min-height: 40px; margin-bottom: 0; margin-top: 0; }
}
	/* Touch screens: 44 px tap targets for the small controls (after the phone block, so it wins). */
	@media (pointer: coarse) {
		.link { min-height: 44px; }
}
</style>
