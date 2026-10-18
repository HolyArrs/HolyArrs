<script lang="ts">
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import AdminLinkButton from '#lib/ui/AdminLinkButton.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { dateTime } from '#lib/format.ts';
	import { emptyFilter, levelTone, levelWord, logQuery, pageWords, type LogFilter, type LogPage } from '#lib/logs.ts';

	let filter = $state<LogFilter>(emptyFilter());
	let result = $state<LogPage | null>(null);
	let error = $state('');
	let busy = $state(false);

	async function load() {
		busy = true;
		try {
			result = await api.get<LogPage>('/logs?' + logQuery(filter));
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			busy = false;
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

<PageHeader title="Logs" subtitle="What HolyArrs wrote to its log, newest first. Keys, tokens, passwords and links with sign-in details are always hidden.">
	{#snippet actions()}
		<AdminLinkButton href={to(`/api/v1/logs/download`)} download>Download the log</AdminLinkButton>
	{/snippet}
</PageHeader>

<form class="card filters" aria-label="Filter the log" onsubmit={apply}>
	<div class="row-fields">
		<div class="field">
			<label for="lg-level">Level</label>
			<select id="lg-level" class="input" bind:value={filter.level} onchange={() => apply()}>
				<option value="">Every line</option>
				<option value="warning">Warnings and errors</option>
				<option value="error">Errors only</option>
			</select>
		</div>
		<div class="field grow">
			<label for="lg-q">Text</label>
			<input id="lg-q" class="input" type="search" bind:value={filter.q} placeholder="For example qbittorrent" autocomplete="off" />
		</div>
		<div class="field">
			<label for="lg-from">From</label>
			<input id="lg-from" class="input" type="date" bind:value={filter.from} onchange={() => apply()} />
		</div>
		<div class="field">
			<label for="lg-to">To</label>
			<input id="lg-to" class="input" type="date" bind:value={filter.to} onchange={() => apply()} />
		</div>
	</div>
	<div class="row" style="margin-top: var(--s-3)">
		<Button variant="primary" type="submit" disabled={busy} aria-busy={busy}>Show</Button>
		<Button variant="plain" type="button" onclick={clear}>Clear</Button>
	</div>
</form>

{#if error}
	<p class="notice bad" role="alert">{error}</p>
{:else if !result}
	<div class="sk" style="height: 240px"></div>
{:else}
	<div data-testid="log-lines">
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
	.row-fields .field {
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
