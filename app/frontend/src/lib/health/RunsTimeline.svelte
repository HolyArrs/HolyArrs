<script lang="ts">
	// Last runs above a Now line and coming runs below it, beside the live board.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import LiveViews from './LiveViews.svelte';
	import { pastResult, type StatusRow } from './rows.ts';
	import { nextRun } from './schedules.ts';
	import { ago, dateTime } from '../core/format.ts';

	let { upcoming, recent, now }: { upcoming: StatusRow[]; recent: StatusRow[]; now: number } = $props();

	// Five runs each way keeps the rail short.
	const MAX = 5;
	const up = $derived(upcoming.slice(0, MAX));
	// Oldest at the top so the newest sits right above Now.
	const past = $derived(recent.slice(0, MAX).reverse());
	// A past run shows how it ended, not what the job does now.
	const result = pastResult;
</script>

<section class="st-plate" aria-labelledby="s-tl" data-testid="status-timeline">
	<h2 id="s-tl">{t('health.timeline.title')}<span class="cnt">{t('health.timeline.counts', { ran: recent.length, coming: upcoming.length })}</span></h2>
	<div class="halves">
		<ol class="tl">
			{#each past as r (r.key)}
				{@const res = result(r)}
				<li class="past">
					<time class="tm" datetime={r.lastRun} title={dateTime(r.lastRun)}>{ago(r.lastRun, now)}</time>
					<i class="st-dot {res.tone}" aria-hidden="true"></i>
					<span class="nm">{r.name}</span>
					<span class="chip {res.tone}">{res.word}</span>
				</li>
			{/each}
			<li class="nowline" aria-label={t('health.timeline.now')}><span class="tm">{t('health.timeline.now')}</span><i class="nowd pulse" aria-hidden="true"></i><span class="hr"></span></li>
			{#each up as r (r.key)}
				<li>
					<span class="tm">{r.word === t('health.job.running') ? t('health.until.now') : nextRun(r.nextRun, now)}</span><i class="st-dot {r.tone}" aria-hidden="true"></i><span class="nm">{r.name}</span>
					{#if r.tone === 'fail' || r.tone === 'wait'}<span class="chip {r.tone}">{r.word}</span>{/if}
				</li>
			{:else}<li class="none">{t('health.timeline.none')}</li>{/each}
		</ol>

		<div class="day"><LiveViews {upcoming} {recent} /></div>
	</div>
</section>

<style>
	ol { list-style: none; margin: 0; padding: 0; }
	.halves { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1.15fr); gap: 0 28px; }
	/* Cuts a gap in the rail around the dot. */
	.tl .st-dot { justify-self: center; }
	.tl .st-dot:not(.fail) { box-shadow: 0 0 0 4px color-mix(in srgb, var(--surface-1) 90%, transparent); }
	/* The dot shows the state, so the chip keeps only its word. */
	.tl .chip::before { display: none; }
	.nm { min-width: 0; font-size: 14px; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.tm { font-size: 12.5px; color: var(--text-muted); font-variant-numeric: tabular-nums; white-space: nowrap; text-align: right; }
	.none { font-size: 14px; color: var(--text-muted); }

	.tl { position: relative; }
	.tl::before { content: ''; position: absolute; left: calc(84px + 14px + 4px - 1px); top: 10px; bottom: 10px; width: 2px; border-radius: 1px; background: var(--line); }
	.tl li { position: relative; display: grid; grid-template-columns: 84px 8px minmax(0, 1fr) auto; align-items: center; gap: 14px; min-height: 40px; }
	.tl .past .nm { color: var(--text-muted); }
	.nowline { min-height: 32px; }
	.nowline .tm { color: var(--accent-text, var(--text)); font-weight: 600; }
	.nowd { flex: none; border-radius: 50%; width: 10px; height: 10px; margin-left: -1px; background: var(--accent, var(--text)); box-shadow: 0 0 0 4px color-mix(in srgb, var(--accent, var(--text)) 24%, transparent); }
	.hr { grid-column: 3 / 5; height: 1px; background: linear-gradient(90deg, color-mix(in srgb, var(--accent, var(--text)) 50%, transparent), transparent); }
	.tl .none { grid-column: 3 / 5; }
	@media (prefers-reduced-motion: no-preference) {
		.pulse { animation: pulse 2.4s ease-out infinite; }
		@keyframes pulse { 0% { box-shadow: 0 0 0 4px color-mix(in srgb, var(--accent, var(--text)) 24%, transparent); } 70%, 100% { box-shadow: 0 0 0 10px transparent; } }
	}

	.day { display: flex; flex-direction: column; min-width: 0; padding-left: 28px; border-left: 1px solid var(--line); }

	@media (max-width: 1000px) {
		.halves { grid-template-columns: minmax(0, 1fr); gap: 18px; }
		.day { padding: 16px 0 0; border-left: 0; border-top: 1px solid var(--line); }
	}
	@media (max-width: 767px) {
		.tl::before { left: calc(64px + 12px + 4px - 1px); }
		.tl li { grid-template-columns: 64px 8px minmax(0, 1fr) auto; gap: 12px; }
	}
</style>
