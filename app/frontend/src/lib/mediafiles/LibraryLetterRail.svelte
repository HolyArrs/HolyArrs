<script lang="ts">
	import { letters } from './library.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	let { jumps, focus = $bindable(), onjump }: { jumps: Map<string, number>; focus: string; onjump: (letter: string) => void } = $props();

	let rail = $state<HTMLElement | null>(null);
	function railKey(e: KeyboardEvent) {
		const on = letters.filter((l) => jumps.has(l));
		const i = on.indexOf(focus);
		let next = '';
		if (e.key === 'ArrowDown' || e.key === 'ArrowRight') next = on[Math.min(on.length - 1, i + 1)];
		else if (e.key === 'ArrowUp' || e.key === 'ArrowLeft') next = on[Math.max(0, i - 1)];
		else if (e.key === 'Home') next = on[0];
		else if (e.key === 'End') next = on[on.length - 1];
		if (!next) return;
		e.preventDefault();
		focus = next;
		rail?.querySelector<HTMLElement>(`[data-letter="${next === '#' ? 'num' : next}"]`)?.focus();
	}
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<nav class="rail" aria-label={t('mediafiles.rail.jump')} data-testid="letter-rail" bind:this={rail} onkeydown={railKey}>
	{#each letters as l (l)}
		{@const on = jumps.has(l)}
		<button type="button" class="rail-l" data-letter={l === '#' ? 'num' : l} disabled={!on} tabindex={on && l === focus ? 0 : -1} aria-label={l === '#' ? t('mediafiles.rail.numbers') : t('mediafiles.rail.letter', { l })} title={l === '#' ? t('mediafiles.rail.numbersTitle') : l} onclick={() => onjump(l)}>{l}</button>
	{/each}
</nav>

<style>
	.rail { position: sticky; top: 84px; display: flex; flex-direction: column; align-items: stretch; padding: 4px 0; border-radius: 999px; background: var(--surface-1); box-shadow: var(--shadow-card); }
	.rail-l { display: grid; place-items: center; min-height: 24px; /* WCAG 2.5.8: each letter is at least a 24px target. */ padding: 0; border: 0; border-radius: 6px; background: none; color: var(--text); font: 600 11px/1 var(--font-ui); cursor: pointer; }
	.rail-l:hover:not(:disabled) { background: var(--surface-2); color: var(--accent); }
	.rail-l:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.rail-l:disabled { color: var(--text-muted); opacity: .45; cursor: default; }
	@media (max-width: 639px) {
		.rail { top: 64px; padding: 2px 0; }
		.rail-l { font-size: 10px; min-height: 24px; }
	}
</style>
