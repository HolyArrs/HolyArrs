<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Field from '#lib/ui/Field.svelte';
	import Button from '#lib/ui/Button.svelte';
	import TestTag from '#lib/ui/TestTag.svelte';
	import { ms } from './api.ts';
	import { checkCount, checkSummary, checkTone, checkWord, defaultQuery, type CheckKind, type SourceCheck } from './check.ts';
	import { api } from '../core/api.ts';
	import { plainMessage as message } from '../core/settings.ts';
	import { to } from '../core/paths.ts';
	import { session } from '../core/session.svelte.ts';

	let kind = $state<CheckKind>('movie');
	let query = $state(defaultQuery.movie);
	let busy = $state(false);
	let rows = $state<SourceCheck[] | null>(null);
	let error = $state('');

	function pickKind(k: CheckKind) {
		if (query === defaultQuery[kind]) query = defaultQuery[k];
		kind = k;
	}

	async function run(e: Event) {
		e.preventDefault();
		busy = true;
		error = '';
		try {
			rows = (await api.post<{ sources: SourceCheck[] }>('/sources/check', { query: query.trim(), kind })).sources;
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}
</script>

{#if session.can('settings')}
	<SettingsCard id="source-check" title={t('sources.test.title')} icon="search" description={t('sources.test.description')}>
		<form class="ask" onsubmit={run}>
			<Field id="sc-kind" label={t('sources.test.kind')}>
				<select id="sc-kind" class="input" value={kind} onchange={(e) => pickKind((e.currentTarget as HTMLSelectElement).value as CheckKind)}>
					<option value="movie">{t('sources.test.movie')}</option>
					<option value="show">{t('sources.test.show')}</option>
					<option value="music">{t('sources.test.music')}</option>
				</select>
			</Field>
			<Field id="sc-query" label={t('sources.test.titleField')}>
				<input id="sc-query" class="input" bind:value={query} maxlength="200" />
			</Field>
			<div class="go">
				<Button variant="primary" type="submit" disabled={busy} aria-busy={busy}>{busy ? t('sources.test.asking') : t('sources.test.run')}</Button>
			</div>
		</form>
		{#if error}<p class="note fail" role="alert">{error}</p>{/if}
		{#if rows}
			<p class="note" data-testid="source-check-summary" aria-live="polite">{checkSummary(rows)}</p>
			<ul class="rows" data-testid="source-check">
				{#each rows as c (c.id)}
					<li class="row">
						<span class="name">
							{#if !c.answered}<a href={to(`/settings/sources/${c.id}`)}>{c.name}</a>{:else}{c.name}{/if}
						</span>
						{#if c.paused}<span class="chip {checkTone(c)}">{checkWord(c)}</span>{:else}<TestTag ok={c.answered} text={c.answered ? '' : (c.error ?? '')} hover={c.answered ? t('sources.test.countIn', { count: checkCount(c), time: ms(c.ms) }) : ''} extra={checkCount(c)} />{/if}
						<span class="meta" class:bad-text={!c.answered && !c.paused}>
							{#if c.answered}{ms(c.ms)}{:else if c.paused}{c.health.text}{:else}{c.error}{#if c.ms} · {ms(c.ms)}{/if}{/if}
						</span>
					</li>
				{/each}
			</ul>
		{/if}
	</SettingsCard>
{/if}

<style>
	.ask {
		display: grid;
		grid-template-columns: 9rem 1fr auto;
		gap: var(--s-3);
		align-items: end;
	}
	/* The Settings row grid would split label and control, so stack them here. */
	.ask > :global(.field) { display: flex !important; flex-direction: column; align-items: stretch !important; gap: 6px; padding-block: 0 !important; border-top: 0 !important; box-shadow: none !important; }
	.ask > :global(.field > label) { display: block; text-align: left; justify-self: start; }
	.ask { padding-block: 14px; }
	.go {
		padding-bottom: 2px;
	}
	.note {
		margin: var(--s-3) 0 0;
		color: var(--text-muted);
	}
	.note.fail {
		color: var(--fail-text);
	}
	.rows {
		list-style: none;
		margin: var(--s-3) 0 0;
		padding: 0;
	}
	.row {
		display: grid;
		grid-template-columns: minmax(8rem, 14rem) auto 1fr;
		gap: var(--s-3);
		align-items: center;
		padding: var(--s-2) 0;
		border-top: 1px solid var(--line);
	}
	.name {
		font-weight: 600;
		overflow-wrap: anywhere;
	}
	.meta {
		color: var(--text-muted);
		overflow-wrap: anywhere;
		min-width: 0;
	}
	.meta.bad-text {
		color: var(--fail-text);
	}
	@media (max-width: 560px) {
		.ask {
			grid-template-columns: 1fr;
		}
		.row {
			grid-template-columns: 1fr auto;
		}
		.meta {
			grid-column: 1 / -1;
		}
	}
</style>
