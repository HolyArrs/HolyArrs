<script lang="ts">
	// Test search: asks every enabled Source for one title at once and shows
	// who answered. Owners and admins only; it sits on the Sources page.
	import SettingsCard from './SettingsCard.svelte';
	import Field from './Field.svelte';
	import Button from './Button.svelte';
	import { api, message } from '../api.ts';
	import { to } from '../paths.ts';
	import { session } from '../session.svelte.ts';
	import { ms } from '../sources.ts';
	import { checkCount, checkSummary, checkTone, checkWord, defaultQuery, type CheckKind, type SourceCheck } from '../source-check.ts';

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
	<SettingsCard id="source-check" title="Test search" icon="search" description="Ask every enabled Source for one title at once and see which ones answer.">
		<form class="ask" onsubmit={run}>
			<Field id="sc-kind" label="Kind">
				<select id="sc-kind" class="input" value={kind} onchange={(e) => pickKind((e.currentTarget as HTMLSelectElement).value as CheckKind)}>
					<option value="movie">Movie</option>
					<option value="show">Show</option>
					<option value="music">Music</option>
				</select>
			</Field>
			<Field id="sc-query" label="Title">
				<input id="sc-query" class="input" bind:value={query} maxlength="200" />
			</Field>
			<div class="go">
				<Button variant="primary" type="submit" disabled={busy} aria-busy={busy}>{busy ? 'Asking…' : 'Run'}</Button>
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
						<span class="chip {checkTone(c)}">{checkWord(c)}</span>
						<span class="meta">
							{#if c.answered}{checkCount(c)} · {ms(c.ms)}{:else if c.paused}{c.health.text}{:else}{c.error}{#if c.ms} · {ms(c.ms)}{/if}{/if}
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
