<!-- First-run setup: the numbered stops across the top, only the ones this person gets. Done stops show a tick, the current one is lit. A done stop that is a setup step can be clicked to go back to it. -->
<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import { PHASES, STEPS, type Phase } from './welcome-steps.ts';

	// phases: the stops this person gets (see phasesFor); the rest are left out, not shown as skipped.
	// onpick: go to a done stop (only setup steps; Language, Welcome and Account come before sign-in).
	// reach: the furthest stop this person got to; stops up to it can be clicked (back, or forward again).
	let { current, phases = PHASES, onpick, reach }: { current: Phase; phases?: Phase[]; onpick?: (p: Phase) => void; reach?: Phase } = $props();
	const far = $derived(Math.max(phases.indexOf(current), reach ? phases.indexOf(reach) : -1));
	const pickable = (p: Phase) => !!onpick && (STEPS as string[]).includes(p);
	const labels = $derived<Record<Phase, string>>({
		language: t('welcome.bar.language'),
		welcome: t('welcome.bar.welcome'),
		signin: t('welcome.bar.account'),
		parts: t('welcome.bar.parts'),
		server: t('welcome.bar.server'),
		folders: t('welcome.bar.folders'),
		sources: t('welcome.bar.sources'),
		downloads: t('welcome.bar.downloads'),
		subtitles: t('welcome.bar.subtitles'),
		access: t('welcome.bar.access'),
		done: t('welcome.bar.done')
	});
	const at = $derived(phases.indexOf(current));
</script>

<nav class="stepbar" aria-label={t('welcome.bar.steps')} data-testid="setup-bar" data-phase={current}>
	<ol>
		{#each phases as p, i (p)}
			<li class:done={i < at} class:now={i === at} class:been={i > at && i <= far} aria-current={i === at ? 'step' : undefined}>
				{#if i !== at && i <= far && pickable(p)}<button type="button" class="hit" onclick={() => onpick?.(p)} aria-label={t('welcome.bar.goTo', { step: labels[p] })} data-testid="setup-bar-{p}"></button>{/if}
				<span class="mark" aria-hidden="true">{#if i < at}<Icon name="check" size={12} stroke={3} />{:else}{i + 1}{/if}</span>
				<span class="label">{labels[p]}</span>{#if i < at}<span class="sr-only">{t('welcome.bar.doneStep')}</span>{/if}
			</li>
		{/each}
	</ol>
	<p class="compact" aria-hidden="true">{t('welcome.stepOf', { n: at + 1, total: phases.length, title: labels[current] })}</p>
</nav>

<style>
	/* "stepbar", not "bar": the app-wide .bar is the thin progress line, and sharing its name squashed this to 4px. */
	.stepbar {
		margin: var(--s-4) 0 var(--s-5);
		padding: 14px 6px 12px;
		/* The setup glass tokens (welcome page) when present, so every step's controls share this surface. */
		border-radius: var(--g-r, var(--r-card));
		border: 1px solid var(--g-line, color-mix(in srgb, var(--text) 10%, transparent));
		background: var(--g-bg, color-mix(in srgb, var(--text) 6%, transparent));
	}
	ol { display: flex; list-style: none; margin: 0; padding: 0; }
	li.been .mark { border-color: color-mix(in srgb, var(--accent, #e53935) 60%, transparent); }
	.hit { position: absolute; inset: -6px 0; z-index: 2; border: 0; background: none; border-radius: 10px; cursor: pointer; }
	.hit:hover { background: color-mix(in srgb, var(--text) 6%, transparent); }
	.hit:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	li {
		position: relative;
		flex: 1 1 0;
		min-width: 0;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 6px;
		color: var(--ink-2, var(--text-muted));
		font: 500 12px/1.3 var(--font-ui);
		padding: 0 3px;
		text-align: center;
	}
	/* The line to the previous stop, filled once that stop is done. */
	li + li::before {
		content: '';
		position: absolute;
		top: 12px;
		inset-inline-end: calc(50% + 17px);
		inset-inline-start: calc(-50% + 17px);
		height: 2px;
		border-radius: 1px;
		background: color-mix(in srgb, var(--text) 18%, transparent);
	}
	li.done + li::before, li.done + li.now::before { background: var(--accent-fill, var(--accent)); }
	.mark {
		display: grid;
		place-items: center;
		width: 26px;
		height: 26px;
		border-radius: 50%;
		border: 1.5px solid color-mix(in srgb, var(--text) 28%, transparent);
		background: var(--surface, transparent);
		color: var(--ink-2, var(--text-muted));
		font: 600 12px/1 var(--font-ui);
		font-variant-numeric: tabular-nums;
	}
	.done { color: var(--text); }
	.done .mark, .now .mark { background: var(--accent-fill, var(--accent)); border-color: var(--accent-fill, var(--accent)); color: var(--text-on-red, #fff); }
	.now { color: var(--text); font-weight: 650; }
	.now .mark { box-shadow: 0 0 0 4px color-mix(in srgb, var(--accent-fill, var(--accent)) 30%, transparent); }
	/* Whole words only: two-word names wrap between the words, never inside one. */
	.label { width: 100%; overflow-wrap: normal; word-break: normal; hyphens: manual; }
	.compact { display: none; }
	/* Phones: a "Step n of N" line over a segmented line, so nothing crowds or scrolls sideways. */
	@media (max-width: 600px) {
		.stepbar { display: flex; flex-direction: column-reverse; gap: 8px; margin: var(--s-3) 0 var(--s-4); padding: 10px 12px 12px; }
		li { height: 4px; border-radius: 2px; background: color-mix(in srgb, var(--text) 18%, transparent); gap: 0; }
		li + li { margin-inline-start: 4px; }
		li + li::before { display: none; }
		li.done, li.now { background: var(--accent); }
		.mark, .label { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
		.compact { display: block; margin: 0; color: var(--text); font: 600 13px/1.3 var(--font-ui); text-align: center; }
	}
</style>
