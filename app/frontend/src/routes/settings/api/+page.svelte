<script lang="ts">
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import AdminLinkButton from '#lib/ui/AdminLinkButton.svelte';
	import Button from '#lib/ui/Button.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import MoreOptions from '#lib/ui/MoreOptions.svelte';
	import { ApiKeysCard, SeerrCard } from '#lib/arrapi/index.ts';
	import { curl, filterOps, groups, operations, type ApiOp, type Spec } from '#lib/arrapi/index.ts';
	import { onMount } from 'svelte';
	import { to } from '#lib/core/paths.ts';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';

	let spec = $state<Spec | null>(null);
	let ops = $state<ApiOp[]>([]);
	let q = $state('');
	let error = $state('');
	let loading = $state(false);
	let open = $state<string | null>(null);
	const shown = $derived(groups(filterOps(ops, q)));
	const base = $derived(typeof location === 'undefined' ? to('/api/v1') : `${location.origin}${to('/api/v1')}`);
	const examples = $derived([
		{ what: 'Is HolyArrs up, and which version', cmd: curl(base, 'GET', '/health') },
		{ what: 'Every movie, show and artist in the Library', cmd: curl(base, 'GET', '/titles') },
		{ what: 'What is downloading now', cmd: curl(base, 'GET', '/downloads/view') },
		{ what: 'Run the feed check now', cmd: curl(base, 'POST', '/schedules/feed_check/run') }
	]);

	async function copy(text: string) {
		try {
			await navigator.clipboard.writeText(text);
			toasts.push('Command copied', 'ok');
		} catch {
			toasts.push("Couldn't copy", 'fail', { body: 'Select it instead' });
		}
	}

	async function load() {
		loading = true;
		error = '';
		try {
			spec = await api.get<Spec>('/openapi.json');
			ops = operations(spec);
		} catch (e) {
			error = message(e);
		} finally {
			loading = false;
		}
	}

	onMount(load);
</script>

<svelte:head><title>API · HolyArrs</title></svelte:head>

<PageHeader title="API">
	{#snippet actions()}
		<AdminLinkButton href={to(`/api/v1/openapi.json`)} target="_blank" rel="noopener">The OpenAPI file</AdminLinkButton>
	{/snippet}
</PageHeader>

{#if session.can('settings')}<ApiKeysCard />{/if}

<div class="sc-form">
<SettingsCard id="api-start" title="Start here">
	<ol class="steps small">
		<li>Make a key under <a href="#keys-card">API keys</a>. It is shown once; copy it then.</li>
		<li>Send it in the <code>X-Api-Key</code> header with every call, never in the address.</li>
		<li>
			The base address is
			<div class="cmd address">
				<code data-testid="api-base">{base}</code>
				<Button variant="plain" type="button" onclick={() => copy(base)} aria-label="Copy the base address"><Icon name="copy" size={16} /></Button>
			</div>
		</li>
	</ol>
</SettingsCard>

<SeerrCard />

</div>

<MoreOptions label="Reference">
{#if spec?.['x-holyarrs-scopes']}
		<SettingsCard title="Scopes">
			<dl class="facts" data-testid="api-scopes">
				{#each Object.entries(spec['x-holyarrs-scopes']) as [name, note] (name)}
					<dt><code>{name}</code></dt>
					<dd class="small">{note}</dd>
				{/each}
			</dl>
		</SettingsCard>
{/if}
<div>
<SettingsCard title="Example calls">
	<p class="small muted">Set <code>HOLYARRS_KEY</code> to your key in your shell first.</p>
	<ul class="examples">
		{#each examples as e (e.cmd)}
			<li>
				<span class="small">{e.what}</span>
				<div class="cmd">
					<code class="mono xs">{e.cmd}</code>
					<Button variant="plain" type="button" onclick={() => copy(e.cmd)} aria-label="Copy: {e.what}"><Icon name="copy" size={16} /></Button>
				</div>
			</li>
		{/each}
	</ul>
</SettingsCard>
</div>

<div data-testid="api-routes">
	<SettingsCard id="api-routes" title="Every route">
		{#if error}
			<LoadError hasData={false} message={error} onretry={load} busy={loading} />
		{:else if !spec}
			<SkSettings rows={2} />
		{:else}
			<Field id="api-q" label="Find a route">
				<input id="api-q" class="input" type="search" bind:value={q} placeholder="e.g. downloads" autocomplete="off" />
			</Field>
			<p class="xs muted" role="status">{filterOps(ops, q).length} of {ops.length} routes</p>
			{#each shown as g (g.tag)}
				<h3 class="tag">{g.tag}</h3>
				<ul class="ops">
					{#each g.ops as o (o.method + o.path)}
						{@const id = o.method + ' ' + o.path}
						<li data-testid="api-op">
							<button class="op" type="button" aria-expanded={open === id} onclick={() => (open = open === id ? null : id)}>
								<span class="m m-{o.method.toLowerCase()}">{o.method}</span>
								<code class="p">{o.path}</code>
								<span class="s small">{o.summary}</span>
							</button>
							{#if open === id}
								<div class="more small">
									<p>{o.description}</p>
									<p class="xs muted">Scope <code>{o.scope}</code> · needs <code>{o.permission}</code></p>
								</div>
							{/if}
						</li>
					{/each}
				</ul>
			{/each}
		{/if}
	</SettingsCard>
</div>
</MoreOptions>

<style>
	.steps {
		margin: var(--s-3) 0 0;
		padding-left: 1.2em;
		display: grid;
		gap: var(--s-2);
		overflow-wrap: anywhere;
	}
	.examples {
		list-style: none;
		padding: 0;
		margin: var(--s-3) 0 0;
		display: grid;
		gap: var(--s-3);
	}
	.cmd {
		display: flex;
		align-items: flex-start;
		gap: var(--s-2);
		background: var(--surface-2);
		border-radius: var(--r-btn);
		padding: var(--s-2) var(--s-3);
	}
	.cmd code {
		flex: 1;
		min-width: 0;
		overflow-wrap: anywhere;
		white-space: pre-wrap;
	}
	.address { align-items: center; margin-top: var(--s-2); }
	.cmd :global(button) { flex: none; min-width: var(--tap); min-height: var(--tap); }
	.tag {
		font-size: 0.9375rem;
		margin: var(--s-4) 0 var(--s-2);
	}
	.ops {
		list-style: none;
		padding: 0;
		margin: 0;
	}
	.ops li {
		border-top: 1px solid var(--line);
	}
	.op {
		display: flex;
		flex-wrap: wrap;
		gap: var(--s-2);
		align-items: baseline;
		width: 100%;
		text-align: left;
		background: none;
		border: 0;
		color: var(--text);
		padding: var(--s-2) 0;
		cursor: pointer;
		font: inherit;
	}
	.m {
		font-family: var(--font-mono);
		font-size: 0.75rem;
		font-weight: 600;
		min-width: 4.5em;
	}
	.p {
		font-family: var(--font-mono);
		font-size: 0.8125rem;
		overflow-wrap: anywhere;
		min-width: 0;
	}
	.s {
		color: var(--text-muted, var(--text));
	}
	.more {
		padding: 0 0 var(--s-3);
		overflow-wrap: anywhere;
	}
	code {
		font-family: var(--font-mono);
	}
	@media (max-width: 767px) and (pointer: coarse) {
		.op { min-height: 44px; }
	}
</style>
