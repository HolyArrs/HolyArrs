<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import StepCapsule from './StepCapsule.svelte';
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
					{#if i === now}
						<StepCapsule tone="go" live={!!busy} fill={busy ? Math.max(fill, 0.15) : fill} word={busy || s.word} status={busy ? 'processing' : undefined} />
					{:else if i < now}
						<StepCapsule tone="ok" fill={1} word={s.word} status="ready" />
					{:else}
						<StepCapsule tone="soft" fill={0} word={s.word} />
					{/if}
					<span class="num" aria-hidden="true">{i + 1}</span>
				</button>
			</li>
		{/each}
	</ol>
	<p class="sr-only" aria-live="polite">{t('migrate.stepOf', { n: now + 1, total: list.length, word: list[now].word })}</p>
</nav>

<style>
	.rail-wrap { min-width: 0; }
	.rail { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px; margin: 0; padding: 0; list-style: none; }
	.rail li { min-width: 0; }
	.cap-btn { position: relative; display: block; width: 100%; min-width: 0; padding: 0; border: 0; background: none; color: inherit; font: inherit; text-align: inherit; border-radius: 12px; cursor: pointer; }
	.cap-btn:disabled { cursor: default; }
	.now .cap-btn { cursor: default; }
	.cap-btn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.cap-btn:disabled :global(.st-cap),
	.cap-btn:disabled .num { opacity: 0.7; }
	.num { display: none; }
	@media (max-width: 600px) {
		.rail { grid-template-columns: var(--phone-cols); gap: 6px; }
		.rail li :global(.st-cap) { --st-h: 44px; }
		li:not(.now) :global(.st-cap) { padding: 0; }
		li:not(.now) :global(.st-tx),
		li:not(.now) :global(.st-i) { display: none; }
		li:not(.now) .num { position: absolute; inset: 0; display: grid; place-items: center; font: 700 14px/1 var(--font-ui); color: var(--text); pointer-events: none; }
		li.done:not(.now) .num { color: light-dark(#073d1b, #a6efc0); }
	}
</style>
