<script lang="ts">
	import { fade } from 'svelte/transition';
	import { toasts } from './toasts.svelte';
	import Icon from './Icon.svelte';

	function fastDuration(node: Element) {
		if (typeof matchMedia !== 'undefined' && matchMedia('(prefers-reduced-motion: reduce)').matches) {
			return 0;
		}

		const durationValue = getComputedStyle(node).getPropertyValue('--dur-fast').trim();
		if (!durationValue.endsWith('ms')) return 0;

		const duration = Number.parseFloat(durationValue);
		return Number.isFinite(duration) && duration >= 0 ? duration : 0;
	}

	function toastFade(node: Element) {
		return fade(node, { duration: fastDuration(node) });
	}
</script>

<div class="toasts" role="status" aria-live="polite">
	{#each toasts.list as t (t.id)}
		<div class="toast {t.tone}" in:toastFade out:toastFade>
			<span class="grow">{t.text}</span>
			<button class="x" type="button" aria-label="Close message" onclick={() => toasts.dismiss(t.id)}>
				<Icon name="x" size={16} />
			</button>
		</div>
	{/each}
</div>

<style>
	.toasts {
		position: fixed;
		z-index: 90;
		left: 50%;
		bottom: var(--s-6);
		transform: translateX(-50%);
		display: flex;
		flex-direction: column;
		gap: var(--s-2);
		width: min(440px, calc(100vw - 32px));
		pointer-events: none;
	}
	.toast {
		pointer-events: none;
		display: flex;
		align-items: center;
		gap: var(--s-3);
		padding: 10px 8px 10px 16px;
		background: var(--surface-2);
		border-radius: var(--r-btn);
		box-shadow: var(--shadow-float);
		font-size: var(--text-sm);
		font-weight: var(--weight-medium);
		border-left: 3px solid var(--line-strong);
	}
	.toast.ok {
		border-left-color: var(--ok);
	}
	.toast.fail {
		border-left-color: var(--fail);
	}
	.x {
		pointer-events: auto;
		width: 44px;
		height: 44px;
		display: grid;
		place-items: center;
		border-radius: var(--r-sm);
		color: var(--text-muted);
	}
	.x:hover {
		color: var(--text);
		background: var(--surface-3);
	}
	@media (max-width: 767px) {
		.toasts {
			bottom: calc(var(--tabbar) + var(--s-3) + env(safe-area-inset-bottom));
		}
	}
</style>
