<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Popover from '#lib/ui/Popover.svelte';
	import { size as bytes } from './size.ts';
	import { CONN_HINT, CONN_WORD } from './pane-facts.ts';
	import type { Downloaders } from './clients.svelte.ts';

	let { notes, dls, watch }: { notes: string[]; dls: Downloaders; watch: boolean } = $props();
</script>

<span class="mop" class:warn={notes.length > 0 || dls.clients.some((c) => c.error)}><Popover label={t('downloads.more.label')} icon="more" placement="below" align="end" width={320}>{#snippet children()}
	<div class="mo" data-testid="sb-status">
		{#each notes.filter((n) => !dls.clients.some((c) => n.startsWith(c.name))) as n (n)}<p class="mnote" data-testid="dl-note">{n}</p>{/each}
		{#if !dls.clients.length}<p class="mnote">{dls.health}</p>{/if}
		{#each dls.clients as c (c.id)}
			{@const s = c.status}
			{@const usenet = c.protocol === 'usenet'}
			{@const tone = c.error ? 'warn' : s?.connection === 'disconnected' ? 'bad' : s?.connection === 'firewalled' ? 'warn' : s?.paused ? 'held' : 'ok'}
			<!-- The dot carries the state; a word shows only when something is off. -->
			{@const word = c.error ? '' : s?.connection && s.connection !== 'connected' ? CONN_WORD[s.connection] : s?.paused ? t('downloads.status.paused') : ''}
			{@const hint = c.error ? t('downloads.more.notAnswering') : s?.connection ? CONN_HINT[s.connection] : s?.paused ? t('downloads.status.paused') : t('downloads.more.answering')}
			<div class="mc" class:warn={!!c.error} data-testid="sb-client">
				<div class="mch">
					<i class="dot {tone}" role="img" aria-label={hint} title={hint}></i>
					<b class="cname">{c.name}</b>
					<span class="kind">{usenet ? 'Usenet' : 'Torrent'}</span>
					{#if word}<span class="cn" data-testid="sb-connection">{word}</span>{/if}
					{#if c.error}<button type="button" class="xbn quiet retry" onclick={() => void dls.read()} data-testid="sb-retry">{t('health.sections.tryAgain')}</button>{/if}
				</div>
				{#if c.error}<p class="mnote">{c.error}</p>
				{:else if s}
					<div class="mcg">
						<span><small>{t('downloads.pf.down')}</small>{bytes(s.speed)}/s</span>
						{#if usenet}<span><small>{t('downloads.more.free')}</small>{s.free_space ? bytes(s.free_space) : '–'}</span><span><small>{t('downloads.day.today')}</small>{s.day_down ? bytes(s.day_down) : '–'}</span>
						{:else}<span><small>{t('downloads.pf.up')}</small>{bytes(s.up_speed)}/s</span><span data-testid="sb-session"><small>{t('downloads.more.session')}</small>↑ {bytes(s.session_up)}</span>{/if}
					</div>
					{#if s.slow_mode != null}<label class="slow"><span>{t('downloads.more.slow')}</span><input type="checkbox" role="switch" checked={s.slow_mode} disabled={watch || dls.slowBusy === c.id} onchange={(e) => dls.setSlow(c, e.currentTarget.checked)} data-testid="sb-slow" /></label>{/if}
				{/if}
			</div>
		{/each}
		<div class="mt" data-testid="sb-totals">
			<span><small>{t('downloads.day.today')}</small><b>{dls.sumOf('day_down') ? bytes(dls.sumOf('day_down')) : '–'}</b></span>
			<span><small>{t('downloads.more.month')}</small><b>{dls.sumOf('month_down') ? bytes(dls.sumOf('month_down')) : '–'}</b></span>
			<span><small>{t('downloads.more.allTime')}</small><b>{dls.sumOf('all_time_down') ? bytes(dls.sumOf('all_time_down')) : '–'}</b></span>
		</div>
	</div>
{/snippet}</Popover></span>

<style>
	.xbn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.dot { width: 8px; height: 8px; border-radius: 50%; background: var(--wait); }
	.dot.ok { background: var(--ok); box-shadow: 0 0 0 3px color-mix(in srgb, var(--ok) 25%, transparent); }
	.mo { margin: 0; padding: 6px; display: grid; gap: 8px; font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.mnote { margin: 0; padding: 0 4px; color: var(--text-muted); }
	.mc { display: grid; gap: 8px; padding: 10px 12px; border-radius: var(--r-card); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.mc.warn { box-shadow: 0 0 0 1px color-mix(in srgb, var(--wait) 45%, transparent); }
	.mch { display: flex; align-items: center; gap: 8px; min-width: 0; }
	.mch .dot { flex: none; width: 8px; height: 8px; border-radius: 50%; background: var(--ok); }
	.mch .dot.held { background: var(--text-muted); }
	.mch .dot.warn { background: var(--wait); }
	.mch .dot.bad { background: var(--fail); }
	.cname { flex: 1 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: 600; }
	.mch .kind { flex: none; padding: 1px 8px; border-radius: 999px; box-shadow: 0 0 0 1px var(--line-strong); color: var(--text-muted); font: var(--t-caption); }
	.mch .cn { flex: none; color: var(--text-muted); font: var(--t-caption); }
	.mch .retry { flex: none; min-height: 28px; padding: 0 10px; }
	.mcg, .mt { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 8px; }
	.mcg > span, .mt > span { display: flex; flex-direction: column; gap: 1px; min-width: 0; }
	.mcg small, .mt small { color: var(--text-muted); font: var(--t-caption); }
	.mt > span { padding: 8px 10px; border-radius: var(--r-card); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.mt b { font: 600 15px / 1.2 var(--font-ui); }
	.slow { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin: 0; padding-top: 6px; border-top: 1px solid var(--line); cursor: pointer; min-height: 28px; color: var(--text-muted); }
	.slow input { width: 18px; height: 18px; accent-color: var(--accent); }
	.mop { position: relative; display: inline-flex; }
	.mop.warn::after { content: ''; position: absolute; top: 3px; right: 3px; width: 7px; height: 7px; border-radius: 50%; background: var(--wait); box-shadow: 0 0 0 2px var(--bg); pointer-events: none; }
	.more { min-width: 36px; padding: 0 10px; }
	.xbn { min-height: 32px; padding: 0 14px; border: 1px solid var(--surface-3); border-radius: var(--r-btn); background: var(--surface-2); color: var(--text); font: var(--t-footnote); font-weight: 500; cursor: pointer; }
	.xbn:hover:not(:disabled) { background: var(--surface-3); }
	.xbn:disabled { opacity: 0.5; cursor: not-allowed; }
	.xbn.quiet { border-color: transparent; background: none; color: var(--text-muted); }
	.xbn.quiet:hover:not(:disabled) { border-color: color-mix(in srgb, var(--fail) 35%, transparent); background: color-mix(in srgb, var(--fail) 10%, transparent); color: var(--fail-text); }
	@media (max-width: 640px) {
		.xbn, .mch .retry { min-height: 32px; }
	}
</style>
