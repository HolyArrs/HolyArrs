<script lang="ts">
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import TestTag from '#lib/ui/TestTag.svelte';
	import { barHeight, ms, opWords, uses, healthTone, healthWord, pausedUntil } from '#lib/sources/index.ts';
	import type { SourceCall, SourceDay, SourceView } from '#lib/sources/index.ts';
	import { page } from '$app/state';
	import { to } from '#lib/core/paths.ts';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { ago, words } from '#lib/core/format.ts';
	import { testCount } from '#lib/core/test-tag.ts';

	let src = $state<SourceView | null>(null);
	let days = $state<SourceDay[]>([]);
	let recent = $state<SourceCall[]>([]);
	let span = $state(14);
	let error = $state('');
	let trying = $state(false);
	let tried = $state<{ ok: boolean; text: string; extra?: string; hover?: string } | null>(null);

	async function tryNow() {
		if (!src) return;
		trying = true;
		try {
			const r = await api.post<{ ok: boolean; text: string; ms: number }>(`/sources/${src.id}/try`);
			tried = { ok: r.ok, text: r.text, hover: r.ok ? `${r.text} (${ms(r.ms)})` : '', extra: r.ok ? testCount(r.text) : '' };
		} catch (e) {
			tried = { ok: false, text: message(e) };
		} finally {
			trying = false;
			await load();
		}
	}

	async function load() {
		try {
			const r = await api.get<{ source: SourceView; days: SourceDay[]; recent: SourceCall[] }>(`/sources/${page.params.id}/stats?days=${span}`);
			src = r.source;
			days = r.days;
			recent = r.recent;
			error = '';
		} catch (e) {
			error = message(e);
		}
	}
	$effect(() => {
		void span;
		load();
	});

	const most = $derived(Math.max(0, ...days.map((d) => d.calls)));
	const slowest = $derived(Math.max(0, ...days.map((d) => d.avg_ms)));
	const totals = $derived({
		calls: days.reduce((n, d) => n + d.calls, 0),
		failures: days.reduce((n, d) => n + d.failures, 0),
		grabs: days.reduce((n, d) => n + d.grabs, 0)
	});
	const dayLabel = (d: string) => new Date(d + 'T12:00:00Z').toLocaleDateString([], { month: 'short', day: 'numeric' });
</script>

<svelte:head><title>{src?.name ?? 'Source'} health · HolyArrs</title></svelte:head>

<PageHeader
	back={{ href: to('/settings/sources'), label: 'Sources' }}
	title={src?.name ?? 'Source'}
	subtitle={src ? `${src.format === 'rss' ? 'RSS' : words(src.format)} · ${words(src.protocol)} · priority ${src.priority} · ${uses(src)}` : undefined}
>
	{#snippet actions()}
		{#if src}
			{#if src.state === 'paused'}<Button type="button" disabled={trying} aria-busy={trying} onclick={tryNow} data-testid="source-try">Try now</Button>{/if}
			<span class="chip {healthTone(src.health)}" data-testid="health-state">{healthWord(src.health)}</span>
		{/if}
	{/snippet}
</PageHeader>

{#if error && !src}
	<LoadError hasData={false} message={error} onretry={load} />
{:else if !src}
	<SkSettings rows={2} />
{:else}
	{#if error}<LoadError hasData message={error} onretry={load} />{/if}

	<section class="tiles" aria-label="At a glance">
		<div class="card tile">
			<span class="label">State</span>
			<span class="big">{healthWord(src.health)}</span>
			<span class="xs muted">{src.health.text}{#if pausedUntil(src.health)} {pausedUntil(src.health)}.{/if}</span>
			{#if tried}<span class="testline" data-testid="source-tried"><TestTag ok={tried.ok} text={tried.text} hover={tried.hover} extra={tried.extra} />{#if !tried.ok}<span class="xs bad-text">{tried.text}</span>{/if}</span>{/if}
		</div>
		<div class="card tile">
			<span class="label">Last answer</span>
			<span class="big" data-testid="last-success">{src.summary.last_success ? ago(src.summary.last_success) : 'Never'}</span>
			<span class="xs muted">{src.summary.last_failure ? `Last failure ${ago(src.summary.last_failure)}` : 'No failures kept'}</span>
		</div>
		<div class="card tile">
			<span class="label">Response time today</span>
			<span class="big">{ms(src.summary.avg_ms_24h)}</span>
			<span class="xs muted">on average, answered calls</span>
		</div>
		<div class="card tile">
			<span class="label">Last {span} days</span>
			<span class="big">{totals.calls} calls</span>
			<span class="xs muted">{totals.failures} failed · {totals.grabs} picked</span>
		</div>
	</section>

	<section class="card" aria-labelledby="h-days">
		<div class="row between">
			<h2 id="h-days" class="h2">Over time</h2>
			<label class="xs">
				<span class="sr-only">Days shown</span>
				<select class="input" bind:value={span}>
					<option value={7}>7 days</option>
					<option value={14}>14 days</option>
					<option value={30}>30 days</option>
					<option value={90}>90 days</option>
				</select>
			</label>
		</div>
		{#if days.length === 0}
			<p class="small muted empty-chart">No calls in this period</p>
		{:else}
		<p class="xs muted legend"><span class="key calls"></span> answered <span class="key fails"></span> failed <span class="key grabs"></span> picked · days in UTC</p>
		<div class="bars" role="img" aria-label="Calls, failures and picks per day">
			{#each days as d (d.day)}
				<div class="day" title="{dayLabel(d.day)}: {d.calls} calls, {d.failures} failed, {d.grabs} picked, {ms(d.avg_ms)} on average">
					<div class="stack">
						<span class="bar calls" style="height: {barHeight(d.calls - d.failures, most)}%"></span>
						<span class="bar fails" style="height: {barHeight(d.failures, most)}%"></span>
					</div>
					{#if d.grabs}<span class="dot" aria-hidden="true">{d.grabs}</span>{/if}
				</div>
			{/each}
		</div>
		<h3 class="h3" style="margin-top: var(--s-5)">Response time</h3>
		<div class="bars short" role="img" aria-label="Average response time per day">
			{#each days as d (d.day)}
				<div class="day" title="{dayLabel(d.day)}: {ms(d.avg_ms)}"><div class="stack"><span class="bar time" style="height: {barHeight(d.avg_ms, slowest)}%"></span></div></div>
			{/each}
		</div>
		<div class="ends xs muted"><span>{days.length ? dayLabel(days[0].day) : ''}</span><span>{days.length ? dayLabel(days[days.length - 1].day) : ''}</span></div>
		{/if}
	</section>

	<SettingsCard title="Recent calls">
		{#if recent.length === 0}
			<p class="small muted">No calls. Test it from Sources.</p>
		{:else}
			<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
			<div class="table-scroll" tabindex="0" role="region" aria-label="Recent calls">
				<table class="calls-table">
					<thead><tr><th>When</th><th>What</th><th>Result</th><th>Time</th></tr></thead>
					<tbody>
						{#each recent as c, i (i)}
							<tr data-testid="call">
								<td>{ago(c.at)}</td>
								<td>{opWords[c.op] ?? c.op}</td>
								<td class:bad-text={!c.ok}>{c.ok ? 'Answered' : words(c.error_kind || 'failed')}</td>
								<td>{ms(c.ms)}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</SettingsCard>
{/if}

<style>
	.testline { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 8px; margin-top: 4px; }
	.tiles {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(170px, 1fr));
		gap: var(--s-3);
		margin-bottom: var(--s-4);
	}
	.tiles .card {
		margin-top: 0;
	}
	.tile {
		display: flex;
		flex-direction: column;
		gap: var(--s-1);
	}
	.big {
		font-size: 1.35rem;
		font-weight: var(--weight-semibold);
	}
	.between {
		justify-content: space-between;
		align-items: center;
	}
	.legend {
		margin-top: var(--s-2);
	}
	.empty-chart {
		margin-top: var(--s-4);
	}
	.key {
		display: inline-block;
		width: 10px;
		height: 10px;
		border-radius: 2px;
		margin: 0 4px 0 8px;
		vertical-align: middle;
	}
	.bars {
		display: flex;
		align-items: flex-end;
		gap: 2px;
		height: 140px;
		margin-top: var(--s-3);
	}
	.bars.short {
		height: 70px;
	}
	.day {
		flex: 1;
		height: 100%;
		display: flex;
		flex-direction: column;
		justify-content: flex-end;
		align-items: center;
		position: relative;
		min-width: 0;
	}
	.stack {
		width: 100%;
		height: 100%;
		display: flex;
		flex-direction: column-reverse;
	}
	.bar {
		display: block;
		width: 100%;
	}
	.calls,
	.bar.calls {
		background: var(--ok);
	}
	.fails,
	.bar.fails {
		background: var(--fail);
	}
	.grabs {
		background: var(--accent);
	}
	.bar.time {
		background: var(--text-muted);
		opacity: 0.6;
	}
	.dot {
		position: absolute;
		top: -2px;
		font-size: 10px;
		line-height: 14px;
		min-width: 14px;
		text-align: center;
		border-radius: var(--r-pill);
		background: var(--accent-fill);
		color: var(--text-on-red);
	}
	.ends {
		display: flex;
		justify-content: space-between;
		margin-top: var(--s-1);
	}
	.calls-table {
		width: 100%;
		min-width: 30rem;
		border-collapse: collapse;
		margin-top: var(--s-3);
		font-size: 0.9rem;
	}
	.calls-table th,
	.calls-table td {
		text-align: left;
		padding: var(--s-2) var(--s-2);
		border-bottom: 1px solid var(--line);
	}
	.bad-text {
		color: var(--fail-text);
	}
	.table-scroll {
		max-width: 100%;
		overflow-x: auto;
		overscroll-behavior-inline: contain;
	}
	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip: rect(0 0 0 0);
	}
</style>
