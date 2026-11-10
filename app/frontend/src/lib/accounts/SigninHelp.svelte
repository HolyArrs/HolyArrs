<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	let { calm }: { calm: boolean } = $props();

	const RESET_CMD = 'docker exec -i holyarrs /holyarrs reset-owner';
	let copied = $state(false);
	let copyNote = $state('');
	let cmdEl = $state<HTMLElement>();
	async function copyCmd() {
		try {
			await navigator.clipboard.writeText(RESET_CMD);
			copied = true;
			setTimeout(() => (copied = false), 1500);
		} catch {
			// Plain http has no clipboard: select the command so Ctrl+C copies it.
			const sel = window.getSelection();
			if (!cmdEl || !sel) return;
			const r = document.createRange();
			r.selectNodeContents(cmdEl);
			sel.removeAllRanges();
			sel.addRange(r);
			copyNote = t('accounts.signin.selected');
			setTimeout(() => (copyNote = ''), 3000);
		}
	}
</script>

<!-- Opening brings the answers into view (the frame would otherwise hide the last lines). -->
<details class="help" ontoggle={(e) => { const d = e.currentTarget; if (d.open) d.scrollIntoView({ block: 'nearest', behavior: calm ? 'auto' : 'smooth' }); }}>
	<summary>{t('accounts.signin.cant')}</summary>
	<div class="ans">
		<b>{t('accounts.signin.useServer')}</b>
		<p>{t('accounts.signin.askOwner')}</p>
	</div>
	<div class="ans" data-testid="reset-owner-help">
		<b>{t('accounts.signin.runServer')}</b>
		<p>{t('accounts.signin.runThis')}</p>
		<div class="cmd">
			<code bind:this={cmdEl}>{RESET_CMD}</code>
			<span class="sr-only" role="status">{copied ? t('accounts.signin.copied') : copyNote}</span>
			<button type="button" class="cp" onclick={copyCmd}>{copied ? t('accounts.signin.copied') : t('accounts.signin.copy')}</button>
		</div>
		<p>{t('accounts.signin.typeName')}</p>
		<p class="note">{t('accounts.signin.everyone')}</p>
	</div>
</details>

<style>
	/* The entrance: each part fades and lifts 6 px, 70 ms after the one before; the logo settles from a hair smaller instead. Opacity and transform only, so it stays on the compositor. */
	@media (prefers-reduced-motion: no-preference) {
		:global(.intro) .help {
			animation: part-in var(--dur-slow) var(--ease-out) calc(90ms + var(--i, 1) * 70ms) both;
		}
}
	:global(.gl-wait) .help { animation-play-state: paused; }
	:global(.light) .note, :global(.light) .help { color: rgb(0 0 0 / 0.64); }
	:global(.light) .help .note { color: rgb(0 0 0 / 0.56); }
	:global(.light) .help summary {
		color: #1d1d1f;
		font-weight: 600;
		text-decoration: underline;
		text-underline-offset: 3px;
		text-decoration-color: rgb(0 0 0 / 0.25);
	}
	:global(.light) .cmd { background: rgb(255 255 255 / 0.5); border-color: rgb(0 0 0 / 0.08); }
	:global(.light) .cp { border-color: rgb(0 0 0 / 0.15); color: #111; }
	:global(.light) .help .ans + .ans { border-top-color: rgb(0 0 0 / 0.1); }
	code { font-family: var(--font-mono); color: var(--text); overflow-wrap: anywhere; }
	.note { margin-top: 10px; text-align: center; color: rgb(255 255 255 / 0.55); font: var(--t-footnote); }
	.help { margin-top: 12px; color: var(--text-muted); font: var(--t-footnote); scroll-margin-bottom: calc(var(--fr, 0px) + 16px); }
	/* Sized to its words, like the other text links (not the whole panel width). */
	.help summary { cursor: pointer; width: fit-content; margin: 0 auto; padding: 0 10px; min-height: 36px; display: flex; align-items: center; justify-content: center; border-radius: var(--r-sm); color: var(--accent-text); font: 500 14px/1 var(--font-ui); list-style: none; }
	.help summary::-webkit-details-marker { display: none; }
	.help summary:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (max-width: 767px) and (pointer: coarse) { .help summary { min-height: 44px; }
}
	.help p { margin-top: 6px; }
	.help .ans { padding: 12px 0; text-align: left; }
	.help .ans + .ans { border-top: 1px solid rgb(255 255 255 / 0.08); }
	.help .ans b { display: block; color: var(--text); font-weight: 600; }
	.help .note { margin-top: 10px; text-align: left; font-size: 12px; }
	.cmd { display: flex; align-items: center; gap: 8px; margin-top: 8px; padding: 6px 6px 6px 12px; border-radius: 10px; background: rgb(255 255 255 / 0.06); border: 1px solid rgb(255 255 255 / 0.08); }
	.cmd code { flex: 1; min-width: 0; font-size: 12px; line-height: 1.45; }
	.cp { flex: none; min-height: 32px; min-width: 64px; padding: 0 12px; border-radius: 8px; border: 1px solid rgb(255 255 255 / 0.14); background: transparent; color: var(--text); font: 500 12px/1 var(--font-ui); cursor: pointer; }
	.cp:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	/* Text links, the help summary and Copy: a faint wash on hover, a firmer one on press. */
	@media (hover: hover) {
		.help summary:hover, .cp:hover { background: rgb(255 255 255 / 0.08); }
		:global(.light) .help summary:hover, :global(.light) .cp:hover { background: rgb(0 0 0 / 0.05); }
}
	.help summary:active, .cp:active { background: rgb(255 255 255 / 0.14); }
	:global(.light) .help summary:active, :global(.light) .cp:active { background: rgb(0 0 0 / 0.09); }
	/* Phones: the desktop card, floating low over full-bleed art with 24 px of art on every side; it rides above the keyboard (--kb) with a smooth lift. */
	@media (max-width: 600px) {
		/* Views that do not end on a 44 px link still get room at the bottom edge. */
		.help { margin-top: 2px; }
}
	/* Touch screens: 44 px tap targets for the small controls (after the phone block, so it wins). */
	@media (pointer: coarse) {
		.help summary, .cp { min-height: 44px; }
}
</style>
