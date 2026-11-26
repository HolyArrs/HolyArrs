<script lang="ts">
	// The setup folder browser as a pop-up: focus goes in, Tab stays in, Escape closes, and focus returns to the button that opened it.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import FolderBrowse from './FolderBrowse.svelte';

	let { title, hint = '', names = [], current = '', onpick, onclose }: { title: string; hint?: string; names?: string[]; current?: string; onpick: (path: string) => void; onclose: () => void } = $props();

	function trap(node: HTMLElement) {
		const back = document.activeElement as HTMLElement | null;
		const items = () => [...node.querySelectorAll<HTMLElement>('button:not([disabled]), [href], input, [tabindex]:not([tabindex="-1"])')].filter((e) => !e.classList.contains('fback'));
		queueMicrotask(() => (items()[0] ?? node).focus());
		const key = (e: KeyboardEvent) => {
			if (e.key === 'Escape') {
				e.preventDefault();
				onclose();
				return;
			}
			if (e.key !== 'Tab') return;
			const f = items();
			if (!f.length) return;
			const i = f.indexOf(document.activeElement as HTMLElement);
			if (e.shiftKey && i <= 0) {
				e.preventDefault();
				f[f.length - 1].focus();
			} else if (!e.shiftKey && i === f.length - 1) {
				e.preventDefault();
				f[0].focus();
			}
		};
		window.addEventListener('keydown', key);
		return { destroy() { window.removeEventListener('keydown', key); if (back?.isConnected) back.focus(); } };
	}
</script>

<div class="fpop" role="dialog" aria-modal="true" aria-label={title} tabindex="-1" use:trap>
	<button type="button" class="fback" tabindex="-1" aria-label={t('migrate.browse.close')} onclick={onclose}></button>
	<div class="fpanel">
		<FolderBrowse {names} {title} {hint} {current} oncancel={onclose} {onpick} />
	</div>
</div>

<style>
	.fpop { position: fixed; inset: 0; z-index: 50; display: grid; place-items: center; padding: 16px; }
	.fback { position: absolute; inset: 0; border: 0; background: rgb(0 0 0 / 0.6); backdrop-filter: blur(6px); -webkit-backdrop-filter: blur(6px); cursor: default; }
	.fpanel { position: relative; width: min(600px, 100%); padding: 24px; border-radius: 24px; background: color-mix(in srgb, var(--surface, #141416) 82%, transparent); backdrop-filter: blur(28px) saturate(1.2); -webkit-backdrop-filter: blur(28px) saturate(1.2); border: 1px solid var(--g-line, rgb(255 255 255 / 0.12)); box-shadow: 0 24px 80px rgb(0 0 0 / 0.5); }
</style>
