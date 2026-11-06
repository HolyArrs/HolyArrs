<script lang="ts">
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import AdminLinkButton from '#lib/ui/AdminLinkButton.svelte';
	import Button from '#lib/ui/Button.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { dateTime } from '#lib/format.ts';
	import { emptyFilter, levelTone, levelWord, logQuery, pageWords, type LogFilter, type LogPage } from '#lib/logs.ts';

	let filter = $state<LogFilter>(emptyFilter());
	let result = $state<LogPage | null>(null);
	let error = $state('');
	let busy = $state(false);
	// The filter the shown lines answer; a quick second change (Level right
	// after Show) must not let an older answer land on top of a newer one.
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

<svelte:head><title>Logs · HolyArrs</title></svelte:head>

<PageHeader title="Logs">
	{#snippet actions()}
		<AdminLinkButton href={to(`/api/v1/logs/download`)} download>Download</AdminLinkButton>
	{/snippet}
</PageHeader>

<form class="card filters" aria-label="Filter the log" onsubmit={apply}>
	<div class="row-fields">
		<Field id="lg-level" label="Level">
			<select id="lg-level" class="input" bind:value={filter.level} onchange={() => apply()}>
				<option value="">Every line</option>
				<option value="warning">Warnings and errors</option>
				<option value="error">Errors only</option>
			</select>
		</Field>
		<div class="field grow">
			<label for="lg-q">Text</label>
			<input id="lg-q" class="input" type="search" bind:value={filter.q} placeholder="e.g. qBittorrent" autocomplete="off" />
		</div>
		<Field id="lg-from" label="From">
			<input id="lg-from" class="input" type="date" bind:value={filter.from} onchange={() => apply()} />
		</Field>
		<Field id="lg-to" label="To">
			<input id="lg-to" class="input" type="date" bind:value={filter.to} onchange={() => apply()} />
		</Field>
	</div>
	<div class="row" style="margin-top: var(--s-3)">
		<Button variant="primary" type="submit" disabled={busy} aria-busy={busy}>Apply</Button>
		<Button variant="plain" type="button" onclick={clear}>Clear</Button>
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
		<SettingsCard title="Lines">
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
					<Button variant="plain" type="button" disabled={filter.offset === 0 || busy} onclick={() => move(-filter.limit)}>Newer</Button>
					<Button variant="plain" type="button" disabled={filter.offset + result.lines.length >= result.total || busy} onclick={() => move(filter.limit)}
						>Older</Button>
				</div>
			{/if}
		</SettingsCard>
	</div>
{/if}

<style>
	.row-fields {
		display: flex;
		flex-wrap: wrap;
		gap: var(--s-3);
	}
	.row-fields :global(.field) {
		margin-top: 0;
		min-width: 140px;
	}
	.row-fields .grow {
		flex: 1 1 200px;
	}
	.lines {
		list-style: none;
		padding: 0;
		margin: var(--s-3) 0 0;
	}
	.lines li {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: var(--s-2);
		padding: var(--s-2) 0;
		border-top: 1px solid var(--line);
	}
	.text {
		flex: 1 1 100%;
		font-family: var(--font-mono);
		font-size: 0.8125rem;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
		min-width: 0;
		color: var(--text);
	}
</style>
