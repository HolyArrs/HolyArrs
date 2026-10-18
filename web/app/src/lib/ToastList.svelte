<script lang="ts">
	import { fly } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { toasts } from './toasts.svelte';
	import Toast from './ui/Toast.svelte';
	import { reducedMotion } from './motion.ts';

	// Slide up 12px and fade (spec §3); no travel with reduced motion.
	function toastIn(node: Element) {
		const reduce = reducedMotion();
		return fly(node, { y: reduce ? 0 : 12, duration: reduce ? 0 : 240, easing: cubicOut });
	}
	function toastOut(node: Element) {
		const reduce = reducedMotion();
		return fly(node, { y: reduce ? 0 : 6, duration: reduce ? 0 : 150 });
	}
</script>

<div class="toasts" role="status" aria-live="polite">
	{#each toasts.list as t (t.id)}
		<div in:toastIn out:toastOut>
			<Toast text={t.text} tone={t.tone} ondismiss={() => toasts.dismiss(t.id)} />
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
	@media (max-width: 767px) {
		.toasts {
			bottom: calc(var(--tabbar) + var(--s-3) + env(safe-area-inset-bottom));
		}
	}
</style>
