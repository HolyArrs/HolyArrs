<script lang="ts">
	import { fly } from 'svelte/transition';
	import { toasts } from './toasts.svelte';
	import Icon from './Icon.svelte';

	const reduce = typeof matchMedia !== 'undefined' && matchMedia('(prefers-reduced-motion: reduce)').matches;
</script>

<div class="toasts" role="status" aria-live="polite">
	{#each toasts.list as t (t.id)}
		<div class="toast {t.tone}" transition:fly={{ y: reduce ? 0 : 12, opacity: 1, duration: reduce ? 0 : 200 }}>
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
		pointer-events: auto;
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
