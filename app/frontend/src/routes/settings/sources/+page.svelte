<script lang="ts">
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import TagInput from '#lib/ui/TagInput.svelte';
	import { forgetTags } from '#lib/ui/tags.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import TestTag from '#lib/ui/TestTag.svelte';
	import { ConnectBridge } from '#lib/sources/index.ts';
	import { healthTone, healthWord, pausedUntil, SourceCheck, emptyForm, formBody, formFrom, ms, uses, type SourceForm } from '#lib/sources/index.ts';
	import type { SourceHealth, SourceView } from '#lib/sources/index.ts';
	import { onMount } from 'svelte';
	import { to } from '#lib/core/paths.ts';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { ago, words } from '#lib/core/format.ts';
	import { testCount } from '#lib/core/test-tag.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';

	let sources = $state<SourceView[]>([]);
	let loaded = $state(false);
	let error = $state('');

	let editing = $state<SourceView | null>(null);
	let isNew = $state(false);
	let form = $state<SourceForm>(emptyForm());
	let busy = $state(false);
	let formError = $state('');
	let testing = $state(0);
	let results = $state<Record<number, { ok: boolean; text: string; extra?: string; hover?: string }>>({});

	async function load() {
		try {
			sources = (await api.get<{ sources: SourceView[] }>('/sources')).sources;
			error = '';
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
			toasts.push(`${saved.name} saved`, 'ok');
			forgetTags();
			editing = null;
			await load();
			if (isNew) await test(saved);
		} catch (err) {
			formError = message(err);
		} finally {
			busy = false;
		}
	}

	async function test(s: SourceView, how: 'test' | 'try' = 'test') {
		testing = s.id;
		try {
			const r = await api.post<{ ok: boolean; text: string; ms: number; health: SourceHealth }>(`/sources/${s.id}/${how}`);
			results[s.id] = { ok: r.ok, text: r.text, hover: r.ok ? `${r.text} (${ms(r.ms)})` : '', extra: r.ok ? testCount(r.text) : '' };
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
			toasts.push(`Couldn't ${paused ? 'pause' : 'resume'} Source`, 'fail', { body: message(err) });
		}
	}

	async function remove(s: SourceView) {
		if (!confirm(`Remove ${s.name}? Its Releases and history go too.`)) return;
		try {
			await api.del(`/sources/${s.id}`);
			await load();
		} catch (err) {
			toasts.push("Couldn't remove Source", 'fail', { body: message(err) });
		}
	}

	function until(h: SourceHealth): string {
		const t = pausedUntil(h);
		return t ? ` ${t}.` : '';
	}
</script>

<svelte:head><title>Sources · HolyArrs</title></svelte:head>

<PageHeader title="Sources">
	{#snippet actions()}
		{#if !editing && sources.length}<Button icon="plus" variant="primary" type="button" onclick={startAdd}>Add a Source</Button>{/if}
	{/snippet}
</PageHeader>

<p class="small muted lead">
	Priority 1 goes first. A Source that fails is skipped for up to six hours. For sites without a feed, add Jackett or Prowlarr as a Torznab feed.
</p>

{#if editing}
	<form class="sc-form" onsubmit={save} aria-label={isNew ? 'Add a Source' : `Change ${editing.name}`}>
		<SettingsCard title={isNew ? 'Add a Source' : `Change ${editing.name}`}>
			<Field id="so-name" label="Name" style="margin-top: var(--s-4)">
				<input id="so-name" class="input" bind:value={form.name} maxlength="80" required />
			</Field>
			<div class="two">
				<Field id="so-format" label="Kind of feed">
					<select id="so-format" class="input" bind:value={form.format}>
						<option value="torznab">Torznab (torrents, Jackett, Prowlarr)</option>
						<option value="newznab">Newznab (Usenet)</option>
						<option value="rss">RSS (newest only)</option>
					</select>
				</Field>
				<Field id="so-proto" label="Downloads">
					<select id="so-proto" class="input" bind:value={form.protocol}>
						<option value="torrent">Torrents</option>
						<option value="usenet">Usenet</option>
					</select>
				</Field>
			</div>
			<Field id="so-url" label="Address">
				<input id="so-url" class="input mono" bind:value={form.url} autocomplete="off" spellcheck="false" required placeholder={form.format === 'rss' ? 'https://example.org/rss?passkey=…' : 'https://example.org/api'} />
				<span class="hint">
					{#if form.format === 'rss'}Anything after the ? (such as a passkey) is kept encrypted and shown masked.{:else}The feed address, with or without /api.{/if}
				</span>
			</Field>
			{#if form.format !== 'rss'}
				<Field id="so-key" label="API key" hint="Retype it if the address moves host">
					<input id="so-key" class="input mono" type="password" bind:value={form.api_key} autocomplete="new-password" spellcheck="false" />
				</Field>
			{/if}
			<fieldset class="group">
				<legend class="label">Use it for</legend>
				<label class="checkline"><input type="checkbox" bind:checked={form.feed} /><span><b>Newest list</b></span></label>
				{#if form.format !== 'rss'}
					<label class="checkline"><input type="checkbox" bind:checked={form.auto_search} /><span><b>Automatic searches</b></span></label>
					<label class="checkline"><input type="checkbox" bind:checked={form.hand_search} /><span><b>Choose a release</b></span></label>
				{/if}
			</fieldset>
			<fieldset class="group">
				<legend class="label">Media</legend>
				{#each [['movie', 'Movies'], ['tv', 'TV shows'], ['music', 'Music']] as [k, label] (k)}
					<label class="checkline"><input type="checkbox" checked={form.kinds.includes(k)} onchange={(e) => toggleKind(k, e.currentTarget.checked)} /><span>{label}</span></label>
				{/each}
			</fieldset>
			<div class="two">
				<Field id="so-prio" label="Priority" hint="1–50, lowest first and wins ties">
					<input id="so-prio" class="input" type="number" min="1" max="50" bind:value={form.priority} />
				</Field>
				{#if form.protocol === 'torrent'}
					<Field id="so-seed" label="Fewest seeders">
						<input id="so-seed" class="input" type="number" min="0" bind:value={form.min_seeders} />
					</Field>
				{/if}
			</div>
			<Field id="so-tags" label="Tags" hint="Only titles with a matching tag use it">
				<TagInput id="so-tags" bind:value={form.tags} describedby="so-tags-hint" testid="so-tags" />
			</Field>
			<details class="more">
				<summary>Limits</summary>
				<div class="two">
					<Field id="so-cap" label="Most searches per window" hint="0 for no limit">
						<input id="so-cap" class="input" type="number" min="0" bind:value={form.searches_per_window} />
					</Field>
					<Field id="so-win" label="Window (hours)">
						<input id="so-win" class="input" type="number" min="0" step="0.5" bind:value={form.window_hours} />
					</Field>
					<Field id="so-gap" label="Least time between calls (ms)">
						<input id="so-gap" class="input" type="number" min="0" bind:value={form.gap_ms} />
					</Field>
					<Field id="so-timeout" label="Give up after (seconds)">
						<input id="so-timeout" class="input" type="number" min="1" max="300" bind:value={form.timeout_s} />
					</Field>
				</div>
			</details>
			<details class="more" open={!!form.proxy}>
				<summary>Proxy</summary>
				<Field id="so-proxy" label="Proxy address">
					<input id="so-proxy" class="input mono" bind:value={form.proxy} autocomplete="off" spellcheck="false" placeholder="socks5://name@host:1080 or http://host:3128" />
				</Field>
				<Field id="so-proxy-pw" label="Proxy password">
					<input id="so-proxy-pw" class="input" type="password" bind:value={form.proxy_password} autocomplete="new-password" />
				</Field>
				<label class="checkline">
					<input type="checkbox" bind:checked={form.proxy_allow_local_network} />
					<span><b>Proxy may be on the local network</b></span>
				</label>
			</details>
			<label class="checkline">
				<input type="checkbox" bind:checked={form.allow_local_network} />
				<span><b>Reach the local network</b><br /><span class="hint">For Jackett or Prowlarr at home</span></span>
			</label>
			{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
			<div class="row" style="margin-top: var(--s-5)">
				<Button variant="primary" type="submit" disabled={busy || !form.name.trim() || !form.url.trim()} aria-busy={busy}>Save</Button>
				<Button variant="plain" type="button" onclick={() => (editing = null)}>Cancel</Button>
			</div>
		</SettingsCard>
	</form>
{/if}

{#if !loaded}
	<SkSettings rows={1} />
{:else if error && !sources.length}
	<LoadError hasData={false} message={error} onretry={load} />
{:else if sources.length === 0 && !editing}
	<EmptyState
		icon="source"
		title="No Sources"
		text="Add a Torznab or Newznab feed, or Jackett or Prowlarr."
		action={{ label: 'Add a Source', onclick: startAdd, icon: 'plus' }}
	/>
	<ConnectBridge onchange={load} />
{:else}
	{#if error}<LoadError hasData message={error} onretry={load} />{/if}
	<ul class="fl">
		{#each sources as s (s.id)}
			<li class="dl" data-testid="source" title="{s.url} · priority {s.priority} · {uses(s)}">
				<div class="main">
					<span class="ic"><Icon name="source" size={20} /></span>
					<div class="grow">
						<div class="top"><a class="name" href={to(`/settings/sources/${s.id}`)}>{s.name}</a></div>
						<div class="sub">{s.format === 'rss' ? 'RSS' : words(s.format)} · {words(s.protocol)}</div>
						<div class="sr-only">{s.url}{#if s.format === 'rss' && s.api_key}?{s.api_key}{/if} · 
							Priority {s.priority} · {uses(s)}
							{#if s.api_key && s.format !== 'rss'} · key {s.api_key}{/if}
							{#if s.proxy} · through {s.proxy}{/if}
							{#if s.allow_local_network} · may reach the local network{/if}
							{#if s.tags?.length} · <span data-testid="source-tags">tags {s.tags.join(', ')}</span>{/if}
						</div>
					</div>
					{#if s.summary.calls_24h > 0}<div class="num" title="Last answer {s.summary.last_success ? ago(s.summary.last_success) : 'never'} · {s.summary.failures_24h} of {s.summary.calls_24h} calls failed today"><b>{ms(s.summary.avg_ms_24h)}</b><small>avg</small></div>{/if}
					<span class="st {healthTone(s.health)}"><i></i><span data-testid="source-state">{healthWord(s.health)}</span></span>
					<div class="acts">
						<Button variant="plain" size="sm" type="button" disabled={testing === s.id} aria-busy={testing === s.id} onclick={() => test(s)}>Test</Button>
						{#if s.state === 'disabled'}
							<Button variant="plain" size="sm" type="button" onclick={() => setPaused(s, false)}>Resume</Button>
						{:else}
							<Button variant="plain" size="sm" type="button" onclick={() => setPaused(s, true)}>Pause</Button>
							{#if s.state === 'paused'}<Button variant="plain" size="sm" type="button" disabled={testing === s.id} aria-busy={testing === s.id} onclick={() => test(s, 'try')} data-testid="source-try">Try now</Button>{/if}
						{/if}
						<Button variant="plain" size="sm" href={to(`/settings/sources/${s.id}`)}>Health</Button>
						<Button variant="plain" size="sm" type="button" onclick={() => startChange(s)}>Change</Button>
						<Button variant="plain" size="sm" type="button" onclick={() => remove(s)} aria-label="Remove {s.name}">Remove</Button>
					</div>
				</div>
				<p class="note" class:bad-text={s.health.state !== 'working' && s.health.state !== 'disabled'} class:sr-only={s.health.state === 'working' || s.health.state === 'disabled' || s.state === 'paused'} data-testid="source-text">{s.health.text}{until(s.health)}</p>
				{#if s.health.block === 'bot_check' || results[s.id]?.text.includes("site's bot check")}<p class="note"><a href="#connect-bridge" data-testid="source-bridge-link">Link Jackett or Prowlarr</a></p>{/if}
				{#if results[s.id]}
					<div class="testline note" data-testid="source-test"><TestTag ok={results[s.id].ok} text={results[s.id].text} hover={results[s.id].hover} extra={results[s.id].extra} />{#if !results[s.id].ok}<span class="small bad-text">{results[s.id].text}</span>{/if}</div>
				{/if}
			</li>
		{/each}
	</ul>
	<ConnectBridge onchange={load} />
	{#if sources.length}<SourceCheck />{/if}
{/if}

<style>
	.testline { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 10px; }
	.lead {
		margin-bottom: var(--s-5);
	}
	.list {
		display: flex;
		flex-direction: column;
		gap: var(--s-3);
		margin-top: var(--s-4);
	}
	.head {
		display: flex;
		align-items: center;
		gap: var(--s-3);
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
	.two :global(.field + .field) {
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
	.bad-text {
		color: var(--fail-text);
	}
	.ok-text {
		color: var(--ok-text, inherit);
	}
	@media (max-width: 600px) {
		.two {
			grid-template-columns: 1fr;
		}
	}
	@media (max-width: 767px) and (pointer: coarse) {
		.name { display: inline-flex; align-items: center; min-height: 44px; }
	}

	/* The list: one glass sheet, one row per Source (the Library folders pattern). */
	.fl { list-style: none; margin: var(--s-4) 0 0; padding: 0; border: 1px solid color-mix(in srgb, var(--text) 10%, transparent); border-radius: 20px; background: color-mix(in srgb, var(--surface-1) 60%, transparent); -webkit-backdrop-filter: blur(28px) saturate(1.6); backdrop-filter: blur(28px) saturate(1.6); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.2), 0 24px 50px -32px rgb(0 0 0 / 0.45); overflow: hidden; }
	.dl { padding: 14px 16px 14px 18px; }
	.dl + .dl { box-shadow: inset 0 1px 0 var(--line); }
	.dl:hover { background: color-mix(in srgb, var(--text) 3%, transparent); }
	.main { display: grid; grid-template-columns: 24px minmax(0, 1fr) auto auto auto; align-items: center; column-gap: 16px; }
	.dl .ic { width: 24px; height: 24px; background: none; border-radius: 0; color: color-mix(in srgb, var(--text) 55%, transparent); }
	.top { display: flex; align-items: center; gap: 8px; min-width: 0; }
	.name { font: 600 15px/1.3 var(--font-ui); letter-spacing: -0.01em; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.sub { margin-top: 3px; color: var(--text-muted); font: 400 13px/1.3 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.facts { margin-top: 2px; color: var(--text-muted); font: 400 12px/1.35 var(--font-ui); }
	.num { display: flex; align-items: baseline; gap: 4px; white-space: nowrap; font-variant-numeric: tabular-nums; }
	.num b { font: 600 15px/1 var(--font-ui); } .num small { color: var(--text-muted); font: 400 12px/1 var(--font-ui); }
	.st { display: inline-flex; align-items: center; gap: 6px; color: var(--text-muted); font: 500 12px/1 var(--font-ui); white-space: nowrap; }
	.st i { width: 7px; height: 7px; border-radius: 50%; background: var(--text-faint); }
	.st.ok i { background: var(--ok); box-shadow: 0 0 0 3px color-mix(in srgb, var(--ok) 18%, transparent); }
	.st.wait i { background: var(--wait); box-shadow: 0 0 0 3px color-mix(in srgb, var(--wait) 22%, transparent); }
	.st.fail i, .st.bad i { background: var(--fail); box-shadow: 0 0 0 3px color-mix(in srgb, var(--fail) 22%, transparent); }
	.acts { display: flex; align-items: center; gap: 2px; }
	@media (hover: hover) and (min-width: 761px) {
		.dl { position: relative; }
		.main { grid-template-columns: 24px minmax(0, 1fr) auto auto; }
		/* Buttons sit over the right edge only while you point at the row; the status keeps the edge otherwise. */
		.acts { position: absolute; right: 12px; top: 50%; translate: 0 -50%; padding: 4px 4px 4px 28px; border-radius: 999px; background: linear-gradient(90deg, transparent, var(--surface-1) 24px); opacity: 0; pointer-events: none; transition: opacity var(--dur-fast) var(--ease-out); }
		.dl:hover .acts, .dl:focus-within .acts { opacity: 1; pointer-events: auto; }
	}
	.note { margin: 8px 0 0 40px; font: 400 13px/1.35 var(--font-ui); color: var(--text-muted); }
	.dl .attempt-review { margin: 10px 0 0 40px; }
	@media (max-width: 760px) {
		.main { grid-template-columns: 24px minmax(0, 1fr) auto; grid-template-areas: 'ic grow st' 'ic num num' 'ic acts acts'; row-gap: 6px; }
		.main > .ic { grid-area: ic; align-self: start; } .main > .grow { grid-area: grow; } .main > .st { grid-area: st; justify-self: end; align-self: start; }
		.main > .num { grid-area: num; } .main > .acts { grid-area: acts; justify-self: start; flex-wrap: wrap; margin-left: -10px; }
		.note, .dl .attempt-review { margin-left: 0; }
	}
	.name { color: inherit; text-decoration: none; } .name:hover { text-decoration: underline; }
	.note { margin-top: 6px; }
</style>
