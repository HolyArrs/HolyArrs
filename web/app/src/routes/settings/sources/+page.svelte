<script lang="ts">
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { ago, words } from '#lib/format.ts';
	import Icon from '#lib/Icon.svelte';
	import EmptyState from '#lib/EmptyState.svelte';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { emptyForm, formBody, formFrom, healthWords, ms, sourceTone, uses, type SourceForm } from '#lib/sources.ts';
	import type { SourceHealth, SourceView } from '#lib/types.ts';

	let sources = $state<SourceView[]>([]);
	let loaded = $state(false);
	let error = $state('');

	let editing = $state<SourceView | null>(null);
	let isNew = $state(false);
	let form = $state<SourceForm>(emptyForm());
	let busy = $state(false);
	let formError = $state('');
	let testing = $state(0);
	let results = $state<Record<number, { ok: boolean; text: string }>>({});

	async function load() {
		try {
			sources = (await api.get<{ sources: SourceView[] }>('/sources')).sources;
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}
	onMount(load);

	function startAdd() {
		isNew = true;
		editing = { id: 0, name: '' } as SourceView;
		form = emptyForm();
		formError = '';
	}

	function startChange(s: SourceView) {
		isNew = false;
		editing = s;
		form = formFrom(s);
		formError = '';
	}

	function toggleKind(k: string, on: boolean) {
		form.kinds = on ? [...new Set([...form.kinds, k])] : form.kinds.filter((x) => x !== k);
	}

	async function save(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		formError = '';
		try {
			const body = formBody(form);
			const saved = isNew ? await api.post<SourceView>('/sources', body) : await api.put<SourceView>(`/sources/${editing!.id}`, body);
			toasts.push(`Source ${saved.name} saved.`, 'ok');
			editing = null;
			await load();
			if (isNew) await test(saved);
		} catch (err) {
			formError = message(err);
		} finally {
			busy = false;
		}
	}

	async function test(s: SourceView) {
		testing = s.id;
		try {
			const r = await api.post<{ ok: boolean; text: string; ms: number; health: SourceHealth }>(`/sources/${s.id}/test`);
			results[s.id] = { ok: r.ok, text: r.ok ? `${r.text} (${ms(r.ms)})` : r.text };
		} catch (err) {
			results[s.id] = { ok: false, text: message(err) };
		} finally {
			testing = 0;
			await load();
		}
	}

	async function setPaused(s: SourceView, paused: boolean) {
		try {
			await api.post(`/sources/${s.id}/${paused ? 'pause' : 'resume'}`);
			await load();
		} catch (err) {
			toasts.push(message(err), 'fail');
		}
	}

	async function remove(s: SourceView) {
		if (!confirm(`Remove the Source ${s.name}? Its Releases and its record go too.`)) return;
		try {
			await api.del(`/sources/${s.id}`);
			await load();
		} catch (err) {
			toasts.push(message(err), 'fail');
		}
	}

	function until(h: SourceHealth): string {
		if (!h.until) return '';
		const t = new Date(h.until);
		return Number.isNaN(t.getTime()) ? '' : ` Next try ${t.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}.`;
	}
</script>

<svelte:head><title>Sources · HolyArrs</title></svelte:head>

<PageHeader title="Sources" subtitle="Where HolyArrs looks for Releases: Torznab and Newznab feeds, plain rss feeds, and bridges such as Jackett or Prowlarr for sites that need their own rules.">
	{#snippet actions()}
		{#if !editing}<Button icon="plus" variant="primary" type="button" onclick={startAdd}>Add a Source</Button>{/if}
	{/snippet}
</PageHeader>

<p class="small muted lead">
	Sources are asked in priority order, 1 first. A Source that fails rests for a while and is tried again on its own: one minute, then twice as long each time, up to six hours.
	For sites without a feed of their own, see the guide on using Jackett or Prowlarr as a bridge (<code>docs/sources-bridge.md</code>).
</p>

{#if editing}
	<form class="sc-form" onsubmit={save} aria-label={isNew ? 'Add a Source' : `Change ${editing.name}`}>
		<SettingsCard title={isNew ? 'Add a Source' : `Change ${editing.name}`}>
			<div class="field" style="margin-top: var(--s-4)">
				<label for="so-name">Name</label>
				<input id="so-name" class="input" bind:value={form.name} maxlength="80" required />
			</div>
			<div class="two">
				<div class="field">
					<label for="so-format">Kind of feed</label>
					<select id="so-format" class="input" bind:value={form.format}>
						<option value="torznab">Torznab (torrents, also Jackett and Prowlarr)</option>
						<option value="newznab">Newznab (Usenet)</option>
						<option value="rss">Plain rss feed (newest list only)</option>
					</select>
				</div>
				<div class="field">
					<label for="so-proto">Downloads</label>
					<select id="so-proto" class="input" bind:value={form.protocol}>
						<option value="torrent">Torrents</option>
						<option value="usenet">Usenet</option>
					</select>
				</div>
			</div>
			<div class="field">
				<label for="so-url">Address</label>
				<input id="so-url" class="input mono" bind:value={form.url} autocomplete="off" spellcheck="false" required placeholder={form.format === 'rss' ? 'https://example.org/rss?passkey=…' : 'https://example.org/api'} />
				<span class="hint">
					{#if form.format === 'rss'}Anything after the ? (such as a passkey) is kept encrypted and shown masked.{:else}The feed address, with or without /api.{/if}
				</span>
			</div>
			{#if form.format !== 'rss'}
				<div class="field">
					<label for="so-key">Key</label>
					<input id="so-key" class="input mono" type="password" bind:value={form.api_key} autocomplete="new-password" spellcheck="false" />
					<span class="hint">Kept encrypted. Leave as it is to keep the stored one. Moving the address to another host needs the key again.</span>
				</div>
			{/if}
			<fieldset class="group">
				<legend class="label">Use it for</legend>
				<label class="checkline"><input type="checkbox" bind:checked={form.feed} /><span><b>Newest list</b><br /><span class="hint">Read its newest Releases now and then.</span></span></label>
				{#if form.format !== 'rss'}
					<label class="checkline"><input type="checkbox" bind:checked={form.auto_search} /><span><b>Searches on its own</b><br /><span class="hint">When HolyArrs looks for a title by itself.</span></span></label>
					<label class="checkline"><input type="checkbox" bind:checked={form.hand_search} /><span><b>Search by hand</b><br /><span class="hint">When you search from a title's page.</span></span></label>
				{/if}
			</fieldset>
			<fieldset class="group">
				<legend class="label">Media</legend>
				{#each [['movie', 'Movies'], ['tv', 'TV shows'], ['music', 'Music']] as [k, label] (k)}
					<label class="checkline"><input type="checkbox" checked={form.kinds.includes(k)} onchange={(e) => toggleKind(k, e.currentTarget.checked)} /><span>{label}</span></label>
				{/each}
			</fieldset>
			<div class="two">
				<div class="field">
					<label for="so-prio">Priority</label>
					<input id="so-prio" class="input" type="number" min="1" max="50" bind:value={form.priority} />
					<span class="hint">1 to 50; 1 is asked first and wins a tie.</span>
				</div>
				{#if form.protocol === 'torrent'}
					<div class="field">
						<label for="so-seed">Fewest seeders</label>
						<input id="so-seed" class="input" type="number" min="0" bind:value={form.min_seeders} />
					</div>
				{/if}
			</div>
			<details class="more">
				<summary>Limits</summary>
				<div class="two">
					<div class="field">
						<label for="so-cap">Most searches per window</label>
						<input id="so-cap" class="input" type="number" min="0" bind:value={form.searches_per_window} />
						<span class="hint">0 means no cap.</span>
					</div>
					<div class="field">
						<label for="so-win">Window (hours)</label>
						<input id="so-win" class="input" type="number" min="0" step="0.5" bind:value={form.window_hours} />
					</div>
					<div class="field">
						<label for="so-gap">Least time between calls (ms)</label>
						<input id="so-gap" class="input" type="number" min="0" bind:value={form.gap_ms} />
					</div>
					<div class="field">
						<label for="so-timeout">Give up after (seconds)</label>
						<input id="so-timeout" class="input" type="number" min="1" max="300" bind:value={form.timeout_s} />
					</div>
				</div>
			</details>
			<details class="more" open={!!form.proxy}>
				<summary>Proxy</summary>
				<div class="field">
					<label for="so-proxy">Proxy address</label>
					<input id="so-proxy" class="input mono" bind:value={form.proxy} autocomplete="off" spellcheck="false" placeholder="socks5://name@host:1080 or http://host:3128" />
					<span class="hint">Every call to this Source goes through it. Empty means none.</span>
				</div>
				<div class="field">
					<label for="so-proxy-pw">Proxy password</label>
					<input id="so-proxy-pw" class="input" type="password" bind:value={form.proxy_password} autocomplete="new-password" />
					<span class="hint">Kept encrypted. Leave as it is to keep the stored one.</span>
				</div>
				<label class="checkline">
					<input type="checkbox" bind:checked={form.proxy_allow_local_network} />
					<span><b>The proxy may be on the local network</b><br /><span class="hint">Off: only a proxy on the internet.</span></span>
				</label>
			</details>
			<label class="checkline">
				<input type="checkbox" bind:checked={form.allow_local_network} />
				<span><b>May reach the local network</b><br /><span class="hint">Off: only addresses on the internet. On: also this machine or your home network, for a bridge such as Jackett or Prowlarr.</span></span>
			</label>
			{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
			<div class="row" style="margin-top: var(--s-5)">
				<Button variant="plain" type="button" onclick={() => (editing = null)}>Cancel</Button>
				<Button variant="primary" type="submit" disabled={busy || !form.name.trim() || !form.url.trim()} aria-busy={busy}>Save Source</Button>
			</div>
		</SettingsCard>
	</form>
{/if}

{#if !loaded}
	<div class="sk" style="height: 100px"></div>
{:else if error}
	<p class="notice bad" role="alert">{error}</p>
{:else if sources.length === 0 && !editing}
	<EmptyState icon="source" title="No Sources yet">
		<p>Without a Source, HolyArrs cannot find Releases. Add a Torznab or Newznab feed, or a bridge such as Jackett or Prowlarr.</p>
	</EmptyState>
{:else}
	<ul class="list">
		{#each sources as s (s.id)}
			<li class="card" data-testid="source">
				<div class="head">
					<span class="ic"><Icon name="source" /></span>
					<div class="grow">
						<h2 class="h3"><a href={to(`/settings/sources/${s.id}`)}>{s.name}</a></h2>
						<div class="mono xs muted ellipsis">{s.url}{#if s.format === 'rss' && s.api_key}?{s.api_key}{/if}</div>
					</div>
					<span class="chip {sourceTone(s.health.state)}" data-testid="source-state">{healthWords[s.health.state] ?? words(s.health.state)}</span>
				</div>
				<p class="small" class:bad-text={s.health.state !== 'working' && s.health.state !== 'disabled'} data-testid="source-text">{s.health.text}{until(s.health)}</p>
				<p class="xs muted">
					{words(s.format)} · {words(s.protocol)} · priority {s.priority} · {uses(s)}
					{#if s.api_key && s.format !== 'rss'} · key {s.api_key}{/if}
					{#if s.proxy} · through {s.proxy}{/if}
					{#if s.allow_local_network} · may reach the local network{/if}
				</p>
				<p class="xs muted">
					Last answer {s.summary.last_success ? ago(s.summary.last_success) : 'never'} · {ms(s.summary.avg_ms_24h)} on average today ·
					{s.summary.failures_24h} of {s.summary.calls_24h} calls failed today
				</p>
				{#if results[s.id]}
					<p class="small" class:bad-text={!results[s.id].ok} class:ok-text={results[s.id].ok} role="status" data-testid="source-test">{results[s.id].text}</p>
				{/if}
				<div class="row actions">
					<Button type="button" disabled={testing === s.id} aria-busy={testing === s.id} onclick={() => test(s)}>Test</Button>
					{#if s.state === 'disabled'}
						<Button variant="plain" type="button" onclick={() => setPaused(s, false)}>Resume</Button>
					{:else}
						<Button variant="plain" type="button" onclick={() => setPaused(s, true)}>Pause</Button>
						{#if s.state === 'paused'}<Button variant="plain" type="button" onclick={() => setPaused(s, false)}>Try again now</Button>{/if}
					{/if}
					<Button variant="plain" href={to(`/settings/sources/${s.id}`)}>Health</Button>
					<Button variant="plain" type="button" onclick={() => startChange(s)}>Change</Button>
					<Button variant="plain" type="button" onclick={() => remove(s)} aria-label="Remove {s.name}">Remove</Button>
				</div>
			</li>
		{/each}
	</ul>
{/if}

<style>
	.lead {
		margin-bottom: var(--s-5);
	}
	.list {
		display: flex;
		flex-direction: column;
		gap: var(--s-3);
		margin-top: var(--s-4);
	}
	.list .card + .card {
		margin-top: 0;
	}
	.head {
		display: flex;
		align-items: center;
		gap: var(--s-3);
	}
	.head a {
		color: inherit;
	}
	.ic {
		width: 40px;
		height: 40px;
		display: grid;
		place-items: center;
		border-radius: var(--r-btn);
		background: var(--surface-3);
		flex: none;
	}
	.grow {
		min-width: 0;
		flex: 1;
	}
	.card p {
		margin-top: var(--s-2);
	}
	.actions {
		margin-top: var(--s-3);
		flex-wrap: wrap;
	}
	.two {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--s-3);
		margin-top: var(--s-4);
	}
	.two .field + .field {
		margin-top: 0;
	}
	.group {
		border: 0;
		padding: 0;
		margin: var(--s-5) 0 0;
	}
	.more {
		margin-top: var(--s-5);
	}
	.more summary {
		cursor: pointer;
		font-weight: var(--weight-semibold);
	}
	code {
		font-family: var(--font-mono);
	}
	.bad-text {
		color: var(--fail);
	}
	.ok-text {
		color: var(--ok-text, inherit);
	}
	@media (max-width: 600px) {
		.two {
			grid-template-columns: 1fr;
		}
	}
</style>
