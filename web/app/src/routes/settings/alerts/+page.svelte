<script lang="ts">
	import Field from '#lib/ui/Field.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import EmptyState from '#lib/EmptyState.svelte';
	import Icon from '#lib/Icon.svelte';
	import { ago } from '#lib/format.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { askMoments, libraryMoments, momentWords } from '#lib/wishlist.ts';
	import type { AlertView, Moment } from '#lib/types.ts';

	type Kind = 'discord' | 'email' | 'webhook';
	const allMoments: Moment[] = [...askMoments, ...libraryMoments];
	const momentGroups: { label: string; hint: string; moments: readonly Moment[] }[] = [
		{ label: 'Wishlist', hint: 'Asks and their answers.', moments: askMoments },
		{ label: 'Library', hint: 'What HolyArrs does with your movies and shows. Email goes to the staff addresses.', moments: libraryMoments }
	];
	const kindWord: Record<Kind, string> = { discord: 'Discord', email: 'Email', webhook: 'Webhook' };
	let alerts = $state<AlertView[]>([]);
	let loaded = $state(false);
	let error = $state('');

	// The add or change form.
	let editing = $state<AlertView | null>(null);
	let isNew = $state(false);
	let form = $state({
		name: '',
		kind: 'discord' as Kind,
		enabled: true,
		moments: [...allMoments] as Moment[],
		url: '',
		smtp_host: '',
		smtp_port: 587,
		smtp_security: 'starttls' as 'starttls' | 'tls' | 'none',
		smtp_from: '',
		smtp_user: '',
		smtp_pass: '',
		staff_to: '',
		secret: '',
		webhook_auth: 'sign' as 'sign' | 'bearer' | 'none',
		allow_local_network: false
	});
	let busy = $state(false);
	let formError = $state('');
	let testing = $state(0);

	// How a pack of several episodes is told (5.2.5).
	let pack = $state<'one' | 'each'>('one');
	let packBusy = $state(false);

	async function savePack(v: 'one' | 'each') {
		packBusy = true;
		try {
			pack = (await api.put<{ pack_alerts: 'one' | 'each' }>('/settings/alerts', { pack_alerts: v })).pack_alerts;
			toasts.push(v === 'one' ? 'A pack now sends one Alert listing its episodes.' : 'A pack now sends one Alert per episode.', 'ok');
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			packBusy = false;
		}
	}

	async function load() {
		try {
			void api
				.get<{ pack_alerts: 'one' | 'each' }>('/settings/alerts')
				.then((s) => (pack = s.pack_alerts))
				.catch(() => {});
			alerts = (await api.get<{ alerts: AlertView[] }>('/alerts')).alerts;
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}
	onMount(load);

	function startAdd() {
		isNew = true;
		editing = { id: 0, name: '', kind: 'discord', enabled: true, moments: [...allMoments] };
		form = { name: '', kind: 'discord', enabled: true, moments: [...allMoments], url: '', smtp_host: '', smtp_port: 587, smtp_security: 'starttls', smtp_from: '', smtp_user: '', smtp_pass: '', staff_to: '', secret: '', webhook_auth: 'sign', allow_local_network: false };
		formError = '';
	}

	function startChange(a: AlertView) {
		isNew = false;
		editing = a;
		form = {
			name: a.name,
			kind: a.kind,
			enabled: a.enabled,
			moments: [...a.moments],
			url: a.url ?? '',
			smtp_host: a.smtp_host ?? '',
			smtp_port: a.smtp_port ?? 587,
			smtp_security: a.smtp_security ?? 'starttls',
			smtp_from: a.smtp_from ?? '',
			smtp_user: a.smtp_user ?? '',
			smtp_pass: '', // empty keeps the stored one
			staff_to: a.staff_to ?? '',
			secret: '', // empty keeps the stored one
			webhook_auth: a.webhook_auth ?? 'sign',
			allow_local_network: a.allow_local_network ?? false
		};
		formError = '';
	}

	function toggle(m: Moment, on: boolean) {
		form.moments = on ? [...new Set([...form.moments, m])] : form.moments.filter((x) => x !== m);
	}

	function toggleGroup(ms: readonly Moment[], on: boolean) {
		form.moments = on ? [...new Set([...form.moments, ...ms])] : form.moments.filter((x) => !ms.includes(x));
	}

	async function save(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		formError = '';
		try {
			const body = { ...form, smtp_port: Number(form.smtp_port) };
			if (isNew) await api.post('/alerts', body);
			else await api.put(`/alerts/${editing!.id}`, body);
			toasts.push(`Alert ${form.name} saved.`, 'ok');
			editing = null;
			await load();
		} catch (err) {
			formError = message(err);
		} finally {
			busy = false;
		}
	}

	async function test(a: AlertView) {
		testing = a.id;
		try {
			await api.post(`/alerts/${a.id}/test`);
			toasts.push(`Test sent through ${a.name}.`, 'ok');
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			testing = 0;
			await load();
		}
	}

	async function remove(a: AlertView) {
		if (!confirm(`Remove the Alert ${a.name}?`)) return;
		try {
			await api.del(`/alerts/${a.id}`);
			await load();
		} catch (err) {
			toasts.push(message(err), 'fail');
		}
	}
</script>

<svelte:head><title>Alerts · HolyArrs</title></svelte:head>

<PageHeader title="Alerts">
	{#snippet actions()}
		{#if !editing}<Button icon="plus" variant="primary" type="button" onclick={startAdd}>Add an Alert</Button>{/if}
	{/snippet}
</PageHeader>

<p class="small muted lead">
	Everyone also gets these messages inside HolyArrs, on their Wishlist. For email and Discord, each person adds their address or Discord user id in My account.
</p>

{#if editing}
	<form class="sc-form" onsubmit={save} aria-label={isNew ? 'Add an Alert' : `Change ${editing.name}`}>
		<SettingsCard title={isNew ? 'Add an Alert' : `Change ${editing.name}`}>
			<Field id="al-name" label="Name" style="margin-top: var(--s-4)">
				<input id="al-name" class="input" bind:value={form.name} maxlength="80" required />
			</Field>
			{#if isNew}
				<Field id="al-kind" label="Kind">
					<select id="al-kind" class="input" bind:value={form.kind}>
						<option value="discord">Discord</option>
						<option value="email">Email</option>
						<option value="webhook">Webhook (JSON to your own server)</option>
					</select>
				</Field>
			{/if}
			{#if form.kind === 'webhook'}
				<Field id="al-hook" label="Webhook link" hint="HolyArrs posts JSON here: the moment, the movie, and the reasons and paths. Kept encrypted; shown masked.">
					<input id="al-hook" class="input mono" bind:value={form.url} autocomplete="off" spellcheck="false" placeholder="https://example.com/holyarrs" />
				</Field>
				<Field id="al-secret" label="Secret" hint="Optional. Kept encrypted and never shown again. Leave as it is to keep the stored one; a link to another server needs it typed again.">
					<input id="al-secret" class="input mono" type="password" bind:value={form.secret} autocomplete="new-password" />
				</Field>
				<Field id="al-auth" label="What the secret does">
					<select id="al-auth" class="input" bind:value={form.webhook_auth}>
						<option value="sign">Signs each message (X-HolyArrs-Signature, HMAC-SHA256)</option>
						<option value="bearer">Is sent as a bearer token (Authorization header)</option>
						<option value="none">Nothing (no secret is sent)</option>
					</select>
				</Field>
			{:else if form.kind === 'discord'}
				<Field id="al-url" label="Discord link" hint="In Discord: channel settings, Integrations, Webhooks, Copy link. Kept encrypted; shown masked.">
					<input id="al-url" class="input mono" bind:value={form.url} autocomplete="off" spellcheck="false" placeholder="https://discord.com/api/webhooks/…" />
				</Field>
			{:else}
				<Field id="al-host" label="Mail server">
					<input id="al-host" class="input" bind:value={form.smtp_host} placeholder="smtp.example.com" autocomplete="off" />
				</Field>
				<div class="two">
					<Field id="al-port" label="Port">
						<input id="al-port" class="input" type="number" min="1" max="65535" bind:value={form.smtp_port} />
					</Field>
					<Field id="al-sec" label="Security">
						<select id="al-sec" class="input" bind:value={form.smtp_security}>
							<option value="starttls">STARTTLS (port 587)</option>
							<option value="tls">TLS (port 465)</option>
							<option value="none">None (local mail server only)</option>
						</select>
					</Field>
				</div>
				<Field id="al-from" label="From address">
					<input id="al-from" class="input" type="email" bind:value={form.smtp_from} autocomplete="off" />
				</Field>
				<Field id="al-user" label="Mail sign-in name">
					<input id="al-user" class="input" bind:value={form.smtp_user} autocomplete="off" />
				</Field>
				<Field id="al-pw" label="Mail password" hint="Kept encrypted. Leave as it is to keep the stored one.">
					<input id="al-pw" class="input" type="password" bind:value={form.smtp_pass} autocomplete="new-password" />
				</Field>
				<Field id="al-staff" label="Staff addresses" hint="Who hears when someone asks for a movie. The people who asked get their own messages at their own address.">
					<input id="al-staff" class="input" bind:value={form.staff_to} placeholder="you@example.com, other@example.com" autocomplete="off" />
				</Field>
			{/if}
			{#each momentGroups as g (g.label)}
				{@const all = g.moments.every((m) => form.moments.includes(m))}
				<fieldset class="moments" data-testid="moments-{g.label.toLowerCase()}">
					<legend class="label">When to send: {g.label}</legend>
					<p class="hint">{g.hint}</p>
					<Button variant="plain" size="sm" type="button" onclick={() => toggleGroup(g.moments, !all)}>{all ? 'None of these' : 'All of these'}</Button>
					{#each g.moments as m (m)}
						<label class="checkline">
							<input type="checkbox" checked={form.moments.includes(m)} onchange={(e) => toggle(m, e.currentTarget.checked)} />
							<span><b>{momentWords[m][0]}</b><br /><span class="hint">{momentWords[m][1]}</span></span>
						</label>
					{/each}
				</fieldset>
			{/each}
			<label class="checkline">
				<input type="checkbox" bind:checked={form.allow_local_network} />
				<span><b>May reach the local network</b><br /><span class="hint">Off: only discord.com links, and mail and webhook servers on the internet. On: also a server on this machine or your home network.</span></span>
			</label>
			<label class="checkline">
				<input type="checkbox" bind:checked={form.enabled} />
				<span><b>On</b><br /><span class="hint">Off keeps the Alert but sends nothing.</span></span>
			</label>
			{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
			<div class="row" style="margin-top: var(--s-5)">
				<Button variant="plain" type="button" onclick={() => (editing = null)}>Cancel</Button>
				<Button variant="primary" type="submit" disabled={busy || !form.name.trim()} aria-busy={busy}>Save Alert</Button>
			</div>
		</SettingsCard>
	</form>
{/if}

{#if error}
	<p class="notice bad" role="alert">{error}</p>
{:else if loaded && alerts.length === 0 && !editing}
	<EmptyState icon="bell" title="No Alerts yet">
		<p>Add a Discord link, an email server or a webhook to tell people (or your own tools) outside HolyArrs.</p>
	</EmptyState>
{:else}
	<ul class="list">
		{#each alerts as a (a.id)}
			<li class="card" data-testid="alert">
				<div class="head">
					<h2 class="h3">{a.name}</h2>
					<span class="chip">{kindWord[a.kind] ?? a.kind}</span>
					{#if !a.enabled}<span class="chip">Off</span>{/if}
				</div>
				<p class="small muted">{a.moments.length ? a.moments.map((m) => momentWords[m]?.[0] ?? m).join(' · ') : 'No moments chosen'}</p>
				{#if a.last_error}
					<p class="small bad-text" data-testid="alert-result">Last try failed: {a.last_error}</p>
				{:else if a.last_sent_at}
					<p class="small muted" data-testid="alert-result">Last sent {ago(a.last_sent_at)}</p>
				{/if}
				<div class="row actions">
					<Button type="button" disabled={testing === a.id} aria-busy={testing === a.id} onclick={() => test(a)}>Send a test</Button>
					<Button variant="plain" type="button" onclick={() => startChange(a)}>Change</Button>
					<Button variant="plain" type="button" onclick={() => remove(a)} aria-label="Remove {a.name}">Remove</Button>
				</div>
			</li>
		{/each}
	</ul>
{/if}

<section class="card packs" id="pack-alerts" tabindex="-1" aria-labelledby="h-packs" data-testid="pack-alerts">
	<h2 id="h-packs" class="h2">Season packs</h2>
	<p class="small muted">When one download brings several episodes of a show into the Library.</p>
	<fieldset class="pack-choice" disabled={packBusy}>
		<legend class="sr-only">Alerts for a pack</legend>
		<label class="checkline">
			<input type="radio" name="pack" value="one" checked={pack === 'one'} onchange={() => savePack('one')} />
			<span><b>One Alert for the pack</b><br /><span class="hint">Lists every episode it brought, one per line.</span></span>
		</label>
		<label class="checkline">
			<input type="radio" name="pack" value="each" checked={pack === 'each'} onchange={() => savePack('each')} />
			<span><b>One Alert per episode</b><br /><span class="hint">A ten-episode pack sends ten messages.</span></span>
		</label>
	</fieldset>
</section>

<style>
	.packs {
		margin-top: var(--s-6);
		max-width: 720px;
	}
	.pack-choice {
		border: 0;
		padding: 0;
		margin: 0;
	}
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
		flex-wrap: wrap;
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
	.moments {
		border: 0;
		padding: 0;
		margin: var(--s-5) 0 0;
	}
	.bad-text {
		color: var(--fail);
	}
	@media (max-width: 600px) {
		.two {
			grid-template-columns: 1fr;
		}
	}
</style>
