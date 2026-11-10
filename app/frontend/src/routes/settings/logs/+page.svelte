<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import AdminLinkButton from '#lib/ui/AdminLinkButton.svelte';
	import Button from '#lib/ui/Button.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { emptyFilter, levelTone, levelWord, logQuery, pageWords, type LogFilter, type LogPage } from '#lib/health/index.ts';
	import { onMount } from 'svelte';
	import { to } from '#lib/core/paths.ts';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { dateTime } from '#lib/core/format.ts';

	let filter = $state<LogFilter>(emptyFilter());
	let result = $state<LogPage | null>(null);
	let error = $state('');
	let busy = $state(false);
	// The filter the shown lines answer; a quick second change (Level right after Show) must not let an older answer land on top of a newer one.
	let shown = $state('');
	let seq = 0;

	async function load() {
		const mine = ++seq;
		const q = logQuery(filter);
		busy = true;
		try {
			const page = await api.get<LogPage>('/logs?' + q);
			if (mine !== seq) return;
			result = page;
			shown = q;
			error = '';
		} catch (e) {
			if (mine === seq) error = message(e);
		} finally {
			if (mine === seq) busy = false;
		}
	}

	function apply(e?: SubmitEvent) {
		e?.preventDefault();
		filter.offset = 0;
		void load();
	}

	function clear() {
		filter = emptyFilter();
		void load();
	}

	function move(by: number) {
		filter.offset = Math.max(0, filter.offset + by);
		void load();
	}

	onMount(load);
</script>

<svelte:head><title>{t('settings.logs.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.logs.title')}>
	{#snippet actions()}
		<AdminLinkButton href={to(`/api/v1/logs/download`)} download>{t('settings.logs.download')}</AdminLinkButton>
	{/snippet}
</PageHeader>

<form class="card filters" aria-label={t('settings.logs.filter')} onsubmit={apply}>
	<div class="row-fields">
		<Field id="lg-level" label={t('settings.logs.level')}>
			<select id="lg-level" class="input" bind:value={filter.level} onchange={() => apply()}>
				<option value="">{t('settings.logs.every')}</option>
				<option value="warning">{t('settings.logs.warnings')}</option>
				<option value="error">{t('settings.logs.errors')}</option>
			</select>
		</Field>
		<div class="field grow">
			<label for="lg-q">{t('settings.logs.text')}</label>
			<input id="lg-q" class="input" type="search" bind:value={filter.q} placeholder={t('settings.logs.example')} autocomplete="off" />
		</div>
		<Field id="lg-from" label={t('settings.logs.from')}>
			<input id="lg-from" class="input" type="date" bind:value={filter.from} onchange={() => apply()} />
		</Field>
		<Field id="lg-to" label={t('settings.logs.to')}>
			<input id="lg-to" class="input" type="date" bind:value={filter.to} onchange={() => apply()} />
		</Field>
		<div class="go">
			<Button variant="primary" size="sm" type="submit" disabled={busy} aria-busy={busy}>{t('settings.logs.apply')}</Button>
			<Button variant="plain" size="sm" type="button" onclick={clear}>{t('settings.logs.clear')}</Button>
		</div>
	</div>
</form>

{#if !result}
	{#if error}
		<LoadError hasData={false} message={error} onretry={load} {busy} />
	{:else}
		<SkSettings rows={3} />
	{/if}
{:else}
	{#if error}<LoadError hasData={true} message={error} onretry={load} {busy} />{/if}
	<div data-testid="log-lines" aria-busy={busy} data-query={shown}>
		<SettingsCard title={t('settings.logs.lines')}>
			<p class="small muted" role="status">{pageWords(result, result.lines.length)}</p>
			{#if result.lines.length}
				<ol class="lines">
					{#each result.lines as l, i (filter.offset + i)}
						<li data-testid="log-line" data-level={l.level}>
							<span class="chip {levelTone(l.level)}">{levelWord[l.level]}</span>
							{#if l.at}<time class="mono xs muted" datetime={l.at}>{dateTime(l.at)}</time>{/if}
							<code class="text">{l.text}</code>
						</li>
					{/each}
				</ol>
				<div class="row" style="margin-top: var(--s-3)">
					<Button variant="plain" type="button" disabled={filter.offset === 0 || busy} onclick={() => move(-filter.limit)}>{t('settings.logs.newer')}</Button>
					<Button variant="plain" type="button" disabled={filter.offset + result.lines.length >= result.total || busy} onclick={() => move(filter.limit)}
						>{t('settings.logs.older')}</Button>
				</div>
			{/if}
		</SettingsCard>
	</div>
{/if}

<style>
	/* Filters: one row, small labels above each control, buttons at the end. */
	.filters { padding: 14px 18px !important; }
	.row-fields { display: grid; grid-template-columns: 190px minmax(0, 1fr) 160px 160px auto; gap: 12px; align-items: end; }
	.row-fields > :global(.field) { display: flex !important; flex-direction: column; align-items: stretch !important; gap: 6px; margin: 0 !important; padding: 0 !important; border: 0 !important; box-shadow: none !important; min-width: 0; }
	.row-fields > :global(.field) > :global(label) { color: var(--text-muted) !important; font: 500 12px/1.2 var(--font-ui) !important; }
	.go { display: flex; align-items: center; gap: 4px; padding-bottom: 4px; }
	@media (max-width: 900px) { .row-fields { grid-template-columns: 1fr 1fr; } .row-fields > :global(.grow) { grid-column: 1 / -1; } .go { grid-column: 1 / -1; } }
	/* Lines: level, time, then the message on one row. */
	.lines { list-style: none; padding: 0 !important; margin: 0; box-shadow: none !important; }
	.lines li { display: grid; grid-template-columns: 84px 128px minmax(0, 1fr); align-items: baseline; column-gap: 12px; padding: 9px 0; }
	.lines li + li { box-shadow: inset 0 1px 0 var(--line); }
	.lines li > time { font-family: var(--font-ui); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.text { font-family: var(--font-ui); font-size: 13.5px; font-weight: 400; font-variant-numeric: tabular-nums; line-height: 1.45; white-space: pre-wrap; overflow-wrap: anywhere; min-width: 0; color: var(--text); background: none; padding: 0; }
	@media (max-width: 600px) { .lines li { grid-template-columns: auto 1fr; row-gap: 4px; } .text { grid-column: 1 / -1; } }
</style>
