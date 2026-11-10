<script lang="ts">
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	// One group's parts as ruled rows, with the reason and Check / Fix under any row that is not working.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import { atVerb, type StatusRow } from './rows.ts';
	import { ago, dateTime } from '../core/format.ts';
	import { to } from '../core/paths.ts';

	// Long lists show six rows until "Show all".
	const MAX = 6;

	// flat drops the list's own frame so rows can sit inside a ledger.
	let { rows, canAct, busy, oncheck, flat = false, compact = false }: { rows: StatusRow[]; canAct: boolean; busy: (key: string) => boolean; oncheck: (r: StatusRow) => void; flat?: boolean; compact?: boolean } = $props();

	let all = $state(false);

	const ordered = $derived([...rows].sort((a, b) => rank(a) - rank(b)));

	// Problems float to the top.
	const rank = (r: StatusRow) => (r.tone === 'fail' ? 0 : r.tone === 'wait' ? 1 : 2);
	// The verb keeps "3 d ago" from reading as "not checked for 3 days".
	const when = (r: StatusRow) => (atVerb[r.group] ? t('health.rows.since', { verb: atVerb[r.group] ?? '', when: ago(r.at) }) : ago(r.at));
</script>

<ul class="prl" class:flat class:compact>
	{#each all ? ordered : ordered.slice(0, MAX) as r (r.key)}
		{@const bad = r.tone === 'fail' || r.tone === 'wait'}
		<li class="pr" class:bad data-testid="status-row" data-tone={r.tone} data-key={r.key}>
			<div class="line">
				<span class="nm" title={r.name}>{r.name}</span>
				{#if r.at && !bad}<time class="at" datetime={r.at} title={dateTime(r.at)}>{when(r)}</time>{/if}
				<span class="chip {r.tone}">{r.word}</span>
			</div>
			{#if bad}
				{#if r.reason}<p class="why">{serverWords(r.reason)}</p>{/if}
				{#if canAct && (r.check || r.fix)}
					<div class="acts">
						{#if r.check}<Button variant="plain" size="sm" disabled={busy(r.key)} onclick={() => oncheck(r)}>{t('health.rows.check')}</Button>{/if}
						{#if r.fix}<Button variant="plain" size="sm" href={to(r.fix)}>{t('health.rows.fix')}</Button>{/if}
					</div>
				{/if}
			{/if}
		</li>
	{/each}
</ul>
{#if ordered.length > MAX}
	<div class="all"><Button variant="secondary" aria-expanded={all} onclick={() => (all = !all)}>{all ? t('health.rows.fewer') : t('health.rows.all', { count: ordered.length })}</Button></div>
{/if}

<style>
	.prl { list-style: none; margin: 0; padding: 0; border: 1px solid var(--line); border-radius: 12px; overflow: hidden; background: var(--surface-1); }
	.prl.flat { border: 0; border-radius: 0; background: none; }
	.prl.flat .pr { padding: 0 2px; }
	.prl.flat .pr:nth-child(even):not(.bad) { background: none; }
	.prl.flat .pr.bad { margin: 0 -8px; padding: 0 8px 10px; border-radius: 8px; }
	.prl.compact .line { min-height: 38px; }
	.pr { padding: 0 14px; border-top: 1px solid var(--line); }
	.pr:first-child { border-top: 0; }
	.pr:nth-child(even):not(.bad) { background: color-mix(in srgb, var(--surface-2, var(--surface-1)) 45%, transparent); }
	.pr.bad { background: color-mix(in srgb, var(--fail) 6%, var(--surface-1)); padding-bottom: 10px; }
	.pr.bad[data-tone='wait'] { background: color-mix(in srgb, var(--wait) 7%, var(--surface-1)); }
	.line { display: flex; align-items: center; gap: 10px; min-height: 44px; }
	.nm { flex: 1 1 auto; min-width: 0; font-size: 14px; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.at { flex: none; font-size: 12px; color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.chip { flex: none; }
	.why { margin: -4px 0 0 17px; font-size: 13px; line-height: 1.45; color: var(--text-muted); overflow-wrap: anywhere; }
	.acts { display: flex; gap: 4px; margin: 6px 0 0 9px; }
	.all { display: flex; justify-content: center; margin-top: 12px; }
	@media (max-width: 767px) { .at { display: none; } .all > :global(*) { flex: 1; min-height: 44px; } }
</style>
