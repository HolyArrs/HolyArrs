<script lang="ts">
	import { to } from '../core/paths.ts';
</script>

<div class="brand">
	<!-- Both marks load up front, so switching the appearance never shows a blank logo. -->
	<img class="mark-dark word" src={to('/brand/wordmark-on-dark.svg')} alt="Eleven+" width="150" height="36" />
	<img class="mark-light word" src={to('/brand/wordmark.svg')} alt="Eleven+" width="150" height="36" />
</div>

<style>
	/* The entrance: each part fades and lifts 6 px, 70 ms after the one before; the logo settles from a hair smaller instead. Opacity and transform only, so it stays on the compositor. */
	@media (prefers-reduced-motion: no-preference) {
		:global(.intro) .brand { animation: brand-in var(--dur-slow) var(--ease-out) 60ms both; }
}
	:global(.gl-wait) .brand { animation-play-state: paused; }
	.mark-light, :global(.light) .mark-dark { display: none; }
	:global(.light) .mark-light { display: block; }
	:global(.light) .brand { color: #111; }
	.brand {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 14px;
		margin-bottom: 28px;
		font: 650 24px/1 var(--font-ui);
		letter-spacing: -0.015em;
		color: #fff;
	}
	/* Phones: the desktop card, floating low over full-bleed art with 24 px of art on every side; it rides above the keyboard (--kb) with a smooth lift. */
	@media (max-width: 600px) {
		.brand { gap: 12px; margin-bottom: 16px; font-size: 18px; }
		.brand img { width: 116px; height: 28px; }
}
</style>
