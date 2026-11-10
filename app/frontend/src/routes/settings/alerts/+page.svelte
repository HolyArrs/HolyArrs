<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import TestTag from '#lib/ui/TestTag.svelte';
	import { switchKind } from '#lib/alerts/index.ts';
	import { askMoments, libraryMoments, momentWords } from '#lib/requests/index.ts';
	import { onMount } from 'svelte';
	import { session } from '#lib/core/session.svelte.ts';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { ago } from '#lib/core/format.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import type { AlertView, Moment } from '#lib/core/types.ts';

	type Kind = 'discord' | 'email' | 'webhook' | 'telegram' | 'ntfy' | 'gotify' | 'pushover';
	const allMoments: Moment[] = [...askMoments, ...libraryMoments];
	const momentGroups = $derived<{ id: string; label: string; hint: string; moments: readonly Moment[] }[]>([
		{ id: 'requests', label: t('settings.alerts.groupRequests'), hint: t('settings.alerts.groupRequestsHint'), moments: askMoments },
		{ id: 'library', label: t('settings.alerts.groupLibrary'), hint: t('settings.alerts.groupLibraryHint'), moments: libraryMoments }
	]);
	const kindWord = $derived<Record<Kind, string>>({ discord: 'Discord', email: t('settings.alerts.email'), webhook: 'Webhook', telegram: 'Telegram', ntfy: 'ntfy', gotify: 'Gotify', pushover: 'Pushover' });
	let alerts = $state<AlertView[]>([]);
	let loaded = $state(false);
	let hasData = $state(false);
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
		bot_token: '',
		chat_id: '',
		server_url: '',
		topic: '',
		user_key: '',
		app_token: '',
		clear_app_token: false,
		allow_local_network: false
	});
	let busy = $state(false);
	let formError = $state('');
	let testing = $state(0);
	// The last test of each Alert, shown as a tag on its card.
	let results = $state<Record<number, { ok: boolean; text: string }>>({});

	// How a pack of several episodes is told. Null until the stored value is loaded, so no made-up default shows.
	let pack = $state<'one' | 'each' | null>(null);
	let packBusy = $state(false);
	let packError = $state('');

	async function loadPack() {
		packBusy = true;
		packError = '';
		try {
			pack = (await api.get<{ pack_alerts: 'one' | 'each' }>('/settings/alerts')).pack_alerts;
		} catch (e) {
			packError = message(e);
		} finally {
			packBusy = false;
		}
	}

	async function savePack(v: 'one' | 'each') {
		packBusy = true;
		try {
			pack = (await api.put<{ pack_alerts: 'one' | 'each' }>('/settings/alerts', { pack_alerts: v })).pack_alerts;
			toasts.push(t('settings.alerts.packSaved'), 'ok', { body: v === 'one' ? t('settings.alerts.perPack') : t('settings.alerts.perEpisode') });
		} catch (e) {
			toasts.push(t('settings.alerts.couldntSave'), 'fail', { body: message(e) });
		} finally {
			packBusy = false;
		}
	}

	async function load() {
		try {
			alerts = (await api.get<{ alerts: AlertView[] }>('/alerts')).alerts;
			hasData = true;
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}
	onMount(() => {
		void load();
		void loadPack();
	});

	function startAdd() {
		isNew = true;
		editing = { id: 0, name: '', kind: 'discord', enabled: true, moments: [...allMoments] };
		form = { name: '', kind: 'discord', enabled: true, moments: [...allMoments], url: '', smtp_host: '', smtp_port: 587, smtp_security: 'starttls', smtp_from: '', smtp_user: '', smtp_pass: '', staff_to: '', secret: '', webhook_auth: 'sign', bot_token: '', chat_id: '', server_url: '', topic: '', user_key: '', app_token: '', clear_app_token: false, allow_local_network: false };
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
			smtp_pass: '', // Empty keeps the stored one.
			staff_to: a.staff_to ?? '',
			secret: '', // Empty keeps the stored one.
			webhook_auth: a.webhook_auth ?? 'sign',
			bot_token: '', // Empty keeps the stored one.
			chat_id: a.chat_id ?? '',
			server_url: a.server_url ?? '',
			topic: a.topic ?? '',
			user_key: a.user_key ?? '',
			app_token: '', // Empty keeps the stored one.
			clear_app_token: false,
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
			toasts.push(t('settings.alerts.saved', { name: form.name }), 'ok');
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
			results = { ...results, [a.id]: { ok: true, text: t('settings.alerts.testSent', { name: a.name }) } };
		} catch (err) {
			results = { ...results, [a.id]: { ok: false, text: message(err) } };
		} finally {
			testing = 0;
			await load();
		}
	}

	async function remove(a: AlertView) {
		if (!confirm(t('settings.alerts.confirmRemove', { name: a.name }))) return;
		try {
			await api.del(`/alerts/${a.id}`);
			await load();
		} catch (err) {
			toasts.push(t('settings.alerts.removeFailed'), 'fail', { body: message(err) });
		}
	}
</script>

<svelte:head><title>{t('settings.alerts.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.alerts.title')}>
	{#snippet actions()}
		{#if !editing && alerts.length}<Button icon="plus" variant="primary" type="button" onclick={startAdd}>{t('settings.alerts.add')}</Button>{/if}
	{/snippet}
</PageHeader>

<p class="small muted lead">
	{t('settings.alerts.lead')}
</p>

{#if editing}
	<form class="sc-form" onsubmit={save} aria-label={isNew ? t('settings.alerts.add') : t('settings.alerts.change', { name: editing.name })}>
		<SettingsCard title={isNew ? t('settings.alerts.add') : t('settings.alerts.change', { name: editing.name })}>
			<Field id="al-name" label={t('settings.alerts.name')} style="margin-top: var(--s-4)">
				<input id="al-name" class="input" bind:value={form.name} maxlength="80" required />
			</Field>
			{#if isNew}
				<Field id="al-kind" label={t('settings.alerts.kind')}>
					<select id="al-kind" class="input" bind:value={() => form.kind, (k) => (form = switchKind(form, k))}>
						<option value="discord">Discord</option>
						{#if !session.held('alert_channels_more')}<option value="telegram">Telegram</option>{/if}
						{#if !session.held('alert_channels_more')}<option value="ntfy">ntfy</option>{/if}
						{#if !session.held('alert_channels_more')}<option value="gotify">Gotify</option>{/if}
						{#if !session.held('alert_channels_more')}<option value="pushover">Pushover</option>{/if}
						<option value="email">{t('settings.alerts.email')}</option>
						{#if !session.held('alert_channels_more')}<option value="webhook">{t('settings.alerts.webhookJson')}</option>{/if}
					</select>
				</Field>
			{/if}
			{#if form.kind === 'webhook'}
				<Field id="al-hook" label={t('settings.alerts.hookLink')} hint={t('settings.alerts.hookLinkHint')}>
					<input id="al-hook" class="input mono" bind:value={form.url} autocomplete="off" spellcheck="false" placeholder="https://example.com/holyarrs" />
				</Field>
				<Field id="al-secret" label={t('settings.alerts.secretLabel')} hint={t('settings.alerts.secretHint')}>
					<input id="al-secret" class="input mono" type="password" bind:value={form.secret} autocomplete="new-password" />
				</Field>
				<Field id="al-auth" label={t('settings.alerts.secretDoes')}>
					<select id="al-auth" class="input" bind:value={form.webhook_auth}>
						<option value="sign">{t('settings.alerts.authSign')}</option>
						<option value="bearer">{t('settings.alerts.authBearer')}</option>
						<option value="none">{t('settings.alerts.authNone')}</option>
					</select>
				</Field>
			{:else if form.kind === 'telegram'}
				<Field id="al-tg-token" label={t('settings.alerts.botTokenLabel')} hint={isNew ? t('settings.alerts.botTokenNew') : t('settings.alerts.keptKeep')}>
					<input id="al-tg-token" class="input mono" type="password" bind:value={form.bot_token} autocomplete="new-password" spellcheck="false" placeholder={isNew ? '123456789:AAH…' : t('settings.alerts.stored')} />
				</Field>
				<Field id="al-tg-chat" label={t('settings.alerts.chatId')} hint={t('settings.alerts.chatIdHint')}>
					<input id="al-tg-chat" class="input mono" bind:value={form.chat_id} autocomplete="off" spellcheck="false" placeholder={t('settings.alerts.chatIdExample')} />
				</Field>
			{:else if form.kind === 'ntfy'}
				<Field id="al-ntfy-server" label={t('settings.alerts.serverAddress')} hint={t('settings.alerts.ntfyServerHint')}>
					<input id="al-ntfy-server" class="input mono" bind:value={form.server_url} autocomplete="off" spellcheck="false" placeholder="https://ntfy.sh" />
				</Field>
				<Field id="al-ntfy-topic" label={t('settings.alerts.topic')} hint={t('settings.alerts.topicHint')}>
					<input id="al-ntfy-topic" class="input mono" bind:value={form.topic} maxlength="64" autocomplete="off" spellcheck="false" placeholder="holyarrs-alerts" />
				</Field>
				<Field id="al-ntfy-token" label={t('settings.alerts.accessTokenLabel')} hint={isNew || !editing?.app_token ? t('settings.alerts.accessTokenNew') : t('settings.alerts.keptKeepServer')}>
					<input id="al-ntfy-token" class="input mono" type="password" bind:value={form.app_token} disabled={form.clear_app_token} autocomplete="new-password" spellcheck="false" placeholder={isNew || !editing?.app_token ? 'tk_…' : t('settings.alerts.stored')} />
				</Field>
				{#if !isNew && editing?.app_token}
					<label class="checkline">
						<input id="al-ntfy-clear" type="checkbox" bind:checked={form.clear_app_token} onchange={() => (form.app_token = '')} />
						<span><b>{t('settings.alerts.removeTokenLabel')}</b><br /><span class="hint">{t('settings.alerts.removeTokenHint')}</span></span>
					</label>
				{/if}
			{:else if form.kind === 'gotify'}
				<Field id="al-gotify-server" label={t('settings.alerts.serverAddress')} hint={t('settings.alerts.gotifyServerHint')}>
					<input id="al-gotify-server" class="input mono" bind:value={form.server_url} autocomplete="off" spellcheck="false" placeholder="https://gotify.example.com" />
				</Field>
				<Field id="al-gotify-token" label={t('settings.alerts.appTokenLabel')} hint={isNew ? t('settings.alerts.gotifyTokenNew') : t('settings.alerts.keptKeepServer')}>
					<input id="al-gotify-token" class="input mono" type="password" bind:value={form.app_token} autocomplete="new-password" spellcheck="false" placeholder={isNew ? '' : t('settings.alerts.stored')} />
				</Field>
			{:else if form.kind === 'pushover'}
				<Field id="al-push-user" label={t('settings.alerts.userKey')} hint={t('settings.alerts.userKeyHint')}>
					<input id="al-push-user" class="input mono" bind:value={form.user_key} autocomplete="off" spellcheck="false" />
				</Field>
				<Field id="al-push-token" label={t('settings.alerts.appTokenLabel')} hint={isNew ? t('settings.alerts.pushoverTokenNew') : t('settings.alerts.keptKeep')}>
					<input id="al-push-token" class="input mono" type="password" bind:value={form.app_token} autocomplete="new-password" spellcheck="false" placeholder={isNew ? '' : t('settings.alerts.stored')} />
				</Field>
			{:else if form.kind === 'discord'}
				<Field id="al-url" label={t('settings.alerts.discordLink')} hint={t('settings.alerts.discordLinkHint')}>
					<input id="al-url" class="input mono" bind:value={form.url} autocomplete="off" spellcheck="false" placeholder="https://discord.com/api/webhooks/…" />
				</Field>
			{:else}
				<Field id="al-host" label={t('settings.alerts.mailServer')}>
					<input id="al-host" class="input" bind:value={form.smtp_host} placeholder="smtp.example.com" autocomplete="off" />
				</Field>
				<div class="two">
					<Field id="al-port" label={t('settings.alerts.port')}>
						<input id="al-port" class="input" type="number" min="1" max="65535" bind:value={form.smtp_port} />
					</Field>
					<Field id="al-sec" label={t('settings.alerts.security')}>
						<select id="al-sec" class="input" bind:value={form.smtp_security}>
							<option value="starttls">STARTTLS (port 587)</option>
							<option value="tls">TLS (port 465)</option>
							<option value="none">{t('settings.alerts.secNone')}</option>
						</select>
					</Field>
				</div>
				<Field id="al-from" label={t('settings.alerts.fromAddress')}>
					<input id="al-from" class="input" type="email" bind:value={form.smtp_from} autocomplete="off" />
				</Field>
				<Field id="al-user" label={t('settings.alerts.mailUser')}>
					<input id="al-user" class="input" bind:value={form.smtp_user} autocomplete="off" />
				</Field>
				<Field id="al-pw" label={t('settings.alerts.mailPass')}>
					<input id="al-pw" class="input" type="password" bind:value={form.smtp_pass} autocomplete="new-password" />
				</Field>
				<Field id="al-staff" label={t('settings.alerts.staff')} hint={t('settings.alerts.staffHint')}>
					<input id="al-staff" class="input" bind:value={form.staff_to} placeholder="you@example.com, other@example.com" autocomplete="off" />
				</Field>
			{/if}
			{#each momentGroups as g (g.id)}
				{@const all = g.moments.every((m) => form.moments.includes(m))}
				<fieldset class="moments" data-testid="moments-{g.id}">
					<legend class="label">{t('settings.alerts.whenToSend', { group: g.label })}</legend>
					<p class="hint">{g.hint}</p>
					<Button variant="plain" type="button" onclick={() => toggleGroup(g.moments, !all)}>{all ? t('settings.alerts.none') : t('settings.alerts.all')}</Button>
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
				<span><b>{t('settings.alerts.local')}</b><br /><span class="hint">{t('settings.alerts.localHint')}</span></span>
			</label>
			<label class="checkline">
				<input type="checkbox" bind:checked={form.enabled} />
				<span><b>{t('settings.alerts.send')}</b></span>
			</label>
			{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
			<div class="row" style="margin-top: var(--s-5)">
				<Button variant="primary" type="submit" disabled={busy || !form.name.trim()} aria-busy={busy}>{t('settings.alerts.save')}</Button>
				<Button variant="plain" type="button" onclick={() => (editing = null)}>{t('settings.alerts.cancel')}</Button>
			</div>
		</SettingsCard>
	</form>
{/if}

{#if !loaded}
	<SkSettings rows={2} />
{:else if !hasData}
	<LoadError hasData={false} message={error} onretry={load} />
{:else if alerts.length === 0 && !editing}
	{#if error}<LoadError hasData={true} message={error} onretry={load} />{/if}
	<EmptyState icon="bell" title={t('settings.alerts.none2')} text={t('settings.alerts.noneText')} action={{ label: t('settings.alerts.add'), onclick: startAdd, icon: 'plus' }} />
{:else}
	{#if error}<LoadError hasData={true} message={error} onretry={load} />{/if}
	<ul class="list">
		{#each alerts as a (a.id)}
			<li class="card" data-testid="alert">
				<div class="head">
					<h2 class="h3">{a.name}</h2>
					<span class="chip">{kindWord[a.kind] ?? a.kind}</span>
					{#if !a.enabled}<span class="chip">{t('settings.alerts.off')}</span>{/if}
					{#if results[a.id]}<TestTag ok={results[a.id].ok} text={results[a.id].text} app={kindWord[a.kind] ?? a.kind} testid="alert-test" />{/if}
				</div>
				<p class="small muted">{a.moments.length ? a.moments.map((m) => momentWords[m]?.[0] ?? m).join(' · ') : t('settings.alerts.noMoments')}</p>
				{#if results[a.id] && !results[a.id].ok}
					<p class="small bad-text" data-testid="alert-result">{results[a.id].text}</p>
				{:else if a.last_error}
					<p class="small bad-text" data-testid="alert-result">{t('settings.alerts.lastFailed', { error: a.last_error })}</p>
				{:else if a.last_sent_at}
					<p class="small muted" data-testid="alert-result">{t('settings.alerts.lastSent', { ago: ago(a.last_sent_at) })}</p>
				{/if}
				<div class="row actions">
					<Button type="button" disabled={testing === a.id} aria-busy={testing === a.id} onclick={() => test(a)}>{t('settings.alerts.sendTest')}</Button>
					<Button variant="plain" type="button" onclick={() => startChange(a)}>{t('settings.alerts.changeButton')}</Button>
					<Button variant="plain" type="button" onclick={() => remove(a)} aria-label={t('settings.alerts.removeName', { name: a.name })}>{t('settings.alerts.remove')}</Button>
				</div>
			</li>
		{/each}
	</ul>
{/if}

<section class="card packs" id="pack-alerts" tabindex="-1" aria-labelledby="h-packs" data-testid="pack-alerts">
	<h2 id="h-packs" class="h2">{t('settings.alerts.packs')}</h2>
	<p class="small muted">{t('settings.alerts.packsNote')}</p>
	{#if pack === null}
		{#if packError}
			<LoadError hasData={false} message={packError} onretry={loadPack} busy={packBusy} />
		{:else}
			<SkSettings rows={1} />
		{/if}
	{:else}
	<fieldset class="pack-choice" disabled={packBusy}>
		<legend class="sr-only">{t('settings.alerts.packLegend')}</legend>
		<label class="checkline">
			<input type="radio" name="pack" value="one" checked={pack === 'one'} onchange={() => savePack('one')} />
			<span><b>{t('settings.alerts.onePack')}</b><br /><span class="hint">{t('settings.alerts.onePackHint')}</span></span>
		</label>
		<label class="checkline">
			<input type="radio" name="pack" value="each" checked={pack === 'each'} onchange={() => savePack('each')} />
			<span><b>{t('settings.alerts.perEpisode')}</b></span>
		</label>
	</fieldset>
	{/if}
</section>

<style>
	.packs {
		margin-top: var(--s-6);
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
		color: var(--fail-text);
	}
	@media (max-width: 600px) {
		.two {
			grid-template-columns: 1fr;
		}
	}
</style>
