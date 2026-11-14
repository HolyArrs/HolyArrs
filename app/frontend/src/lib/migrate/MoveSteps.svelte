<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { steps, stepIndex, type Step } from './move.ts';

	let {
		step,
		busy = '',
		fill = 0.5,
		locked = false,
		onpick
	}: {
		step: Step;
		busy?: string;
		fill?: number;
		locked?: boolean;
		onpick: (s: Step) => void;
	} = $props();
	const now = $derived(stepIndex(step));
	const list = $derived(steps());
	const cols = $derived(list.map((_, i) => (i === now ? 'minmax(0, 1fr)' : '44px')).join(' '));
</script>

<nav class="rail-wrap" aria-label={t('migrate.moveSteps')}>
	<ol class="rail" aria-label={t('migrate.steps')} style:--phone-cols={cols} data-testid="move-steps">
		{#each list as s, i (s.id)}
			<li class:now={i === now} class:done={i < now}>
				<button
					type="button"
					class="cap-btn"
					aria-current={i === now ? 'step' : undefined}
					aria-disabled={i === now ? 'true' : undefined}
					aria-label={i < now ? t('migrate.stepOfDone', { n: i + 1, total: list.length, word: s.word }) : t('migrate.stepOf', { n: i + 1, total: list.length, word: s.word })}
					disabled={i > now || (locked && i !== now)}
					onclick={() => i !== now && onpick(s.id)}
				>
					<span class="tx" class:live={i === now && !!busy}>{i === now && busy ? busy : s.word}</span>
					<span class="num" aria-hidden="true">{i + 1}</span>
				</button>
			</li>
		{/each}
	</ol>
	<p class="sr-only" aria-live="polite">{t('migrate.stepOf', { n: now + 1, total: list.length, word: list[now].word })}</p>
</nav>

<style>
	/* The app's segmented look: a 36px track, 10px corners, 14px labels; the step you are on is the raised thumb. */
	.rail-wrap { min-width: 0; }
	.rail { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 2px; margin: 0; padding: 2px; list-style: none; border-radius: 10px; background: var(--surface-2); }
	.rail li { min-width: 0; }
	.cap-btn { display: flex; align-items: center; justify-content: center; width: 100%; min-width: 0; min-height: 32px; padding: 0 12px; border: 0; border-radius: 8px; background: none; color: var(--text-muted); font: var(--t-subhead); font-size: 14px; cursor: pointer; }
	.cap-btn:disabled { cursor: default; opacity: 0.7; }
	.now .cap-btn { cursor: default; background: var(--seg-thumb); color: var(--text); font-weight: 600; box-shadow: 0 1px 2px rgb(0 0 0 / 12%), 0 0 0 .5px rgb(0 0 0 / 4%); }
	.done .cap-btn { color: var(--text); }
	.cap-btn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.tx { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.tx.live { animation: st-pulse 1.6s ease-in-out infinite; }
	@keyframes st-pulse { 50% { opacity: 0.55; } }
	.num { display: none; }
	@media (pointer: coarse) { .cap-btn { min-height: 40px; } }
	@media (max-width: 600px) {
		.rail { grid-template-columns: var(--phone-cols); }
		li:not(.now) .tx { display: none; }
		li:not(.now) .num { display: block; font-weight: 700; }
	}
	@media (prefers-reduced-motion: reduce) { .tx.live { animation: none; } }
</style>
