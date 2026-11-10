<script lang="ts">
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import TagInput from '#lib/ui/TagInput.svelte';
	import { forgetTags } from '#lib/ui/tags.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import SaveBar from '#lib/ui/SaveBar.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import TestTag from '#lib/ui/TestTag.svelte';
	import { emptyForm, formBody, formFrom, healthTone, kindWords, pendingAttempts, type DownloaderAttempt, type DownloaderForm, type DownloaderHealth, type DownloaderSettings, type DownloaderView } from '#lib/downloads/index.ts';
	import { onMount } from 'svelte';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { ago } from '#lib/core/format.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { live } from '#lib/core/live.svelte.ts';

	let list = $state<DownloaderView[]>([]);
	let loaded = $state(false);
	let listOk = $state(false);
	let error = $state('');

	let editing = $state<DownloaderView | null>(null);
	let isNew = $state(false);
	let form = $state<DownloaderForm>(emptyForm());
	let busy = $state(false);
	let formError = $state('');
	let testing = $state(0);
	let results = $state<Record<number, { ok: boolean; text: string }>>({});
	let attempts = $state<Record<number, DownloaderAttempt[]>>({});
	let attemptErrors = $state<Record<number, string>>({});
	let abandoning = $state(0);

	let settings = $state<DownloaderSettings>({
		stalled_after_hours: 6,
		stalled_act_hours: 24,
		remove_failed: true,
		fail_dangerous: true,
		replace_vanished: true,
		grab_spacing_seconds: 2,
		try_next_downloader: true,
		remove_delete_files: true,
		remove_turn_down: false,
		remove_search_again: true,
		downloads_auto_add: false
	});
	let setBusy = $state(false);
	let saved = $state('');
	const dirty = $derived(!!saved && JSON.stringify(settings) !== saved);
	let setError = $state('');
	let setLoadError = $state('');
	let setLoading = $state(false);

	async function loadSettings() {
		setLoading = true;
		setLoadError = '';
		try {
			const s = await api.get<DownloaderSettings>('/settings/downloaders');
			settings = s;
			saved = JSON.stringify(s);
		} catch (e) {
			setLoadError = message(e);
		} finally {
			setLoading = false;
		}
	}

	async function load() {
		try {
			const next = (await api.get<{ downloaders: DownloaderView[] }>('/downloaders')).downloaders;
			list = next;
			listOk = true;
			error = '';
			// Show the list now; uncertain sends fill in when they arrive.
			loaded = true;
			await Promise.all(next.map(loadAttempts));
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}

	async function loadAttempts(d: DownloaderView) {
		try {
			attempts[d.id] = (await api.get<{ attempts: DownloaderAttempt[] }>(`/downloaders/${d.id}/attempts`)).attempts;
			attemptErrors[d.id] = '';
		} catch (err) {
			attemptErrors[d.id] = message(err);
		}
	}
	onMount(() => {
		void load();
		void loadSettings();
		const off = live.on('downloader', () => load());
		return off;
	});

	function startAdd() {
		isNew = true;
		editing = { id: 0, name: '' } as DownloaderView;
		form = emptyForm();
		formError = '';
	}

	// HOLYARRS_READONLY_DOWNLOADERS keeps a Downloader read only; its switch shows locked.
	const writableLocked = $derived(!isNew && !!editing?.writable_locked);

	function startChange(d: DownloaderView) {
		isNew = false;
		editing = d;
		form = formFrom(d);
		qbitWay = d.username ? 'login' : 'key';
		formError = '';
	}

	// qBittorrent signs in with an API key (5.2 or newer) or a user name and password.
	let qbitWay = $state<'key' | 'login'>('key');
	function pickWay(w: 'key' | 'login') {
		qbitWay = w;
		form.secret = '';
		if (w === 'key') form.username = '';
	}

	function pickKind(k: DownloaderForm['kind']) {
		const fresh = emptyForm(k);
		form = { ...fresh, url: form.url !== 'http://' ? form.url : fresh.url, allow_local_network: form.allow_local_network, writable: form.writable };
		qbitWay = 'key';
	}

	function hasUsername(kind: DownloaderForm['kind']): boolean {
		return kind === 'transmission' || kind === 'nzbget' || (kind === 'qbittorrent' && qbitWay === 'login');
	}

	function addressPlaceholder(kind: DownloaderForm['kind']): string {
		switch (kind) {
			case 'sabnzbd':
				return 'http://192.168.1.10:8080';
			case 'transmission':
				return 'http://192.168.1.10:9091';
			case 'nzbget':
				return 'http://192.168.1.10:6789';
			case 'deluge':
				return 'http://192.168.1.10:8112';
			default:
				return 'http://192.168.1.10:8090';
		}
	}

	function categoryHint(kind: DownloaderForm['kind']): string {
		if (kind === 'sabnzbd') return t('settings.downloaders.catSab');
		if (kind === 'transmission') return t('settings.downloaders.catTransmission');
		if (kind === 'deluge') return t('settings.downloaders.catDeluge');
		if (kind === 'nzbget') return t('settings.downloaders.catNzbget');
		return t('settings.downloaders.catQbit');
	}

	async function save(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		formError = '';
		try {
			const body = formBody(form);
			const saved = isNew ? await api.post<DownloaderView>('/downloaders', body) : await api.put<DownloaderView>(`/downloaders/${editing!.id}`, body);
			toasts.push(t('settings.downloaders.saved', { name: saved.name }), 'ok');
			forgetTags();
			editing = null;
			await load();
			await test(saved);
		} catch (err) {
			formError = message(err);
		} finally {
			busy = false;
		}
	}

	async function test(d: DownloaderView) {
		testing = d.id;
		try {
			const r = await api.post<{ ok: boolean; text: string; health: DownloaderHealth }>(`/downloaders/${d.id}/test`);
			results[d.id] = { ok: r.ok, text: r.text };
		} catch (err) {
			results[d.id] = { ok: false, text: message(err) };
		} finally {
			testing = 0;
			await load();
		}
	}

	async function setPaused(d: DownloaderView, paused: boolean) {
		try {
			await api.post(`/downloaders/${d.id}/${paused ? 'pause' : 'resume'}`);
			await load();
		} catch (err) {
			toasts.push(paused ? t('settings.downloaders.couldntPause') : t('settings.downloaders.couldntResume'), 'fail', { body: message(err) });
		}
	}

	async function remove(d: DownloaderView) {
		if (!confirm(t('settings.downloaders.confirmRemove', { name: d.name }))) return;
		try {
			await api.del(`/downloaders/${d.id}`);
			await load();
		} catch (err) {
			toasts.push(t('settings.downloaders.removeFailed'), 'fail', { body: message(err) });
		}
	}

	async function abandonAttempt(d: DownloaderView, attempt: DownloaderAttempt) {
		if (
			!confirm(
				t('settings.downloaders.confirmAbandon', { release: attempt.release_id, slot: attempt.slot_id, name: d.name, app: kindWords[d.kind] })
			)
		)
			return;
		abandoning = attempt.id;
		try {
			await api.post(`/downloaders/${d.id}/attempts/${attempt.id}/abandon`);
			toasts.push(t('settings.downloaders.abandoned'), 'ok', { body: t('settings.downloaders.checkFirst') });
			await loadAttempts(d);
		} catch (err) {
			toasts.push(t('settings.downloaders.abandonFailed'), 'fail', { body: message(err) });
		} finally {
			abandoning = 0;
		}
	}

	async function saveSettings(e: SubmitEvent) {
		e.preventDefault();
		setError = '';
		const body = {
			...settings,
			stalled_after_hours: Number(settings.stalled_after_hours),
			stalled_act_hours: Number(settings.stalled_act_hours),
			grab_spacing_seconds: Number(settings.grab_spacing_seconds)
		};
		if (body.stalled_act_hours < body.stalled_after_hours) {
			setError = t('settings.downloaders.actSooner');
			return;
		}
		setBusy = true;
		try {
			settings = await api.put<DownloaderSettings>('/settings/downloaders', body);
			saved = JSON.stringify(settings);
			toasts.push(t('settings.downloaders.settingsSaved'), 'ok');
		} catch (err) {
			setError = message(err);
		} finally {
			setBusy = false;
		}
	}
	/** Keeps the address short: host and port, no scheme. */
	function hostOf(url: string): string {
		try {
			return new URL(url).host || url;
		} catch {
			return url;
		}
	}
</script>

<svelte:head><title>{t('settings.downloaders.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.downloaders.title')}>
	{#snippet actions()}
		{#if !editing && list.length}<Button icon="plus" variant="primary" type="button" onclick={startAdd}>{t('settings.downloaders.add')}</Button>{/if}
	{/snippet}
</PageHeader>

<p class="small muted lead" id="downloaders-card" tabindex="-1">
	{t('settings.downloaders.lead')}
</p>

{#snippet moreFields()}
<div class="two">
	<Field id="dl-cat" label={t('settings.downloaders.category')}>
		<input id="dl-cat" class="input" bind:value={form.category} maxlength="64" required />
		<span class="hint">{categoryHint(form.kind)} {t('settings.downloaders.onlyFollows')}</span>
	</Field>
	<Field id="dl-prio" label={t('settings.downloaders.priority')} hint={t('settings.downloaders.priorityHint')}>
		<input id="dl-prio" class="input" type="number" min="1" max="50" bind:value={form.priority} />
	</Field>
</div>
<Field id="dl-tags" label={t('settings.downloaders.tags')} hint={t('settings.downloaders.tagsHint')}>
	<TagInput id="dl-tags" bind:value={form.tags} describedby="dl-tags-hint" testid="dl-tags" />
</Field>
<details class="more" open={form.path_map.length > 0}>
	<summary>{t('settings.downloaders.folderNames')}</summary>
	<p class="hint">{t('settings.downloaders.folderNamesHint')}</p>
	{#each form.path_map as p, i (i)}
		<div class="two">
			<Field id="pm-theirs-{i}" label={t('settings.downloaders.theirPath')}>
				<input id="pm-theirs-{i}" class="input mono" bind:value={p.theirs} placeholder="/data/downloads" />
			</Field>
			<Field id="pm-ours-{i}" label={t('settings.downloaders.ourPath')}>
				<input id="pm-ours-{i}" class="input mono" bind:value={p.ours} placeholder="/downloads" />
			</Field>
		</div>
	{/each}
	<Button variant="plain" type="button" style="margin-top: var(--s-3)" onclick={() => form.path_map.push({ theirs: '', ours: '' })}>{t('settings.downloaders.addPair')}</Button>
</details>
{/snippet}

{#if editing}
	<form class="sc-form" onsubmit={save} aria-label={isNew ? t('settings.downloaders.add') : t('settings.downloaders.change', { name: editing.name })}>
		<SettingsCard title={isNew ? t('settings.downloaders.add') : t('settings.downloaders.change', { name: editing.name })}>
			{#if isNew}
				<fieldset class="group">
					<legend class="label">{t('settings.downloaders.whichApp')}</legend>
					<div class="row kinds">
						<label class="checkline"><input type="radio" name="kind" checked={form.kind === 'qbittorrent'} onchange={() => pickKind('qbittorrent')} /><span><b>qBittorrent</b> {t('settings.downloaders.torrents')}</span></label>
						<label class="checkline"><input type="radio" name="kind" checked={form.kind === 'transmission'} onchange={() => pickKind('transmission')} /><span><b>Transmission</b> {t('settings.downloaders.torrents')}</span></label>
						<label class="checkline"><input type="radio" name="kind" checked={form.kind === 'deluge'} onchange={() => pickKind('deluge')} /><span><b>Deluge</b> {t('settings.downloaders.torrents')}</span></label>
						<label class="checkline"><input type="radio" name="kind" checked={form.kind === 'sabnzbd'} onchange={() => pickKind('sabnzbd')} /><span><b>SABnzbd</b> {t('settings.downloaders.usenet')}</span></label>
						<label class="checkline"><input type="radio" name="kind" checked={form.kind === 'nzbget'} onchange={() => pickKind('nzbget')} /><span><b>NZBGet</b> {t('settings.downloaders.usenet')}</span></label>
					</div>
				</fieldset>
			{/if}
			<Field id="dl-name" label={t('settings.downloaders.name')} style="margin-top: var(--s-4)">
				<input id="dl-name" class="input" bind:value={form.name} maxlength="80" required />
			</Field>
			<Field id="dl-url" label={t('settings.downloaders.address')} hint={t('settings.downloaders.addressHint')}>
				<input id="dl-url" class="input mono" bind:value={form.url} autocomplete="off" spellcheck="false" required placeholder={addressPlaceholder(form.kind)} />
			</Field>
			{#if form.kind === 'qbittorrent'}
				<fieldset class="group" data-testid="dl-qbit-way">
					<legend class="label">{t('settings.downloaders.signInWith')}</legend>
					<div class="row kinds">
						<label class="checkline"><input type="radio" name="qbit-way" checked={qbitWay === 'key'} onchange={() => pickWay('key')} /><span>{t('settings.downloaders.apiKeyLabel')}</span></label>
						<label class="checkline"><input type="radio" name="qbit-way" checked={qbitWay === 'login'} onchange={() => pickWay('login')} /><span>{t('settings.downloaders.userPass')}</span></label>
					</div>
				</fieldset>
			{/if}
			{#if hasUsername(form.kind)}
				{@const qbit = form.kind === 'qbittorrent'}
				<div class="two">
					<Field id="dl-user" label={t('settings.downloaders.username')}>
						<input id="dl-user" class="input" bind:value={form.username} autocomplete="off" required={qbit} data-testid="dl-user" />
						<span class="hint">{qbit ? t('settings.downloaders.qbitUser') : t('settings.downloaders.leaveEmpty', { app: kindWords[form.kind] })}</span>
					</Field>
					<Field id="dl-secret" label={t('settings.downloaders.passwordLabel')}>
						<input id="dl-secret" class="input" type="password" bind:value={form.secret} autocomplete="new-password" required={qbit && isNew} data-testid="dl-secret" />
					</Field>
				</div>
			{:else}
				{@const key = form.kind === 'sabnzbd' || form.kind === 'qbittorrent'}
				<Field id="dl-secret" label={key ? t('settings.downloaders.apiKeyLabel') : t('settings.downloaders.passwordLabel')}>
					<input id="dl-secret" class="input" class:mono={key} type="password" bind:value={form.secret} autocomplete="new-password" spellcheck="false" required={isNew} data-testid="dl-secret" />
					<span class="hint">{form.kind === 'sabnzbd' ? t('settings.downloaders.secretSab') : form.kind === 'qbittorrent' ? t('settings.downloaders.secretQbit') : t('settings.downloaders.secretDeluge')} {t('settings.downloaders.secretKept')}</span>
				</Field>
			{/if}
			{#if form.kind === 'deluge'}
				<p class="notice">{t('settings.downloaders.delugeNote')}</p>
			{/if}
			{#if form.kind === 'qbittorrent'}
				<details class="more"><summary>{t('settings.downloaders.moreOptions')}</summary>{@render moreFields()}</details>
			{:else}
				{@render moreFields()}
			{/if}
			<label class="checkline">
				<input type="checkbox" bind:checked={form.allow_local_network} />
				<span><b>{t('settings.downloaders.local')}</b><br /><span class="hint">{t('settings.downloaders.localHint')}</span></span>
			</label>
			<label class="checkline" class:off={writableLocked}>
				<input type="checkbox" bind:checked={form.writable} disabled={writableLocked} data-testid="dl-form-writable" />
				<span><b>{t('settings.downloaders.writable')}</b><br /><span class="hint">{writableLocked ? t('settings.downloaders.writableLocked') : t('settings.downloaders.writableOff')}</span></span>
			</label>
			{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
			<div class="row" style="margin-top: var(--s-5)">
				<Button variant="primary" type="submit" disabled={busy || !form.name.trim() || !form.url.trim()} aria-busy={busy}>{t('settings.downloaders.save')}</Button>
				<Button variant="plain" type="button" onclick={() => (editing = null)}>{t('settings.downloaders.cancel')}</Button>
				{#if !isNew && editing}{@const cur = editing}<Button variant="plain" type="button" disabled={testing === cur.id} aria-busy={testing === cur.id} onclick={() => test(cur)} data-testid="dl-form-test">{t('settings.downloaders.test')}</Button>{/if}
				{#if !isNew && editing && results[editing.id]}{@const r = results[editing.id]}<TestTag ok={r.ok} text={r.text} app={kindWords[editing.kind]} />{#if !r.ok}<span class="small bad-text">{r.text}</span>{/if}{/if}
			</div>
		</SettingsCard>
	</form>
{/if}

{#if !loaded}
	<SkSettings rows={1} />
{:else if error && !listOk}
	<LoadError hasData={false} message={error} onretry={load} />
{:else if list.length === 0 && !editing}
	<EmptyState icon="downloader" title={t('settings.downloaders.none')} text={t('settings.downloaders.noneText')} action={{ label: t('settings.downloaders.add'), onclick: startAdd, icon: 'plus' }} />
{:else}
	{#if error}<LoadError hasData={true} message={error} onretry={load} />{/if}
	<ul class="fl">
		{#each list as d (d.id)}
			<li class="dl" data-testid="downloader">
				<div class="main" title={t('settings.downloaders.rowTitle', { host: hostOf(d.url), priority: d.priority, category: d.category })}>
					<span class="ic"><Icon name={d.protocol === 'usenet' ? 'usenet' : 'torrent'} size={20} /></span>
					<div class="grow">
						<div class="top">
							<span class="name">{d.name}</span>
							{#if results[d.id]}<TestTag ok={results[d.id].ok} text={results[d.id].text} app={kindWords[d.kind]} testid="downloader-version" />{/if}
						</div>
						<div class="sub">{kindWords[d.kind]} · {d.protocol === 'usenet' ? t('settings.downloaders.usenetWord') : t('settings.downloaders.torrentsWord')}</div>
						<div class="sr-only">{hostOf(d.url)} · 
							{t('settings.downloaders.priorityCategory', { priority: d.priority, category: d.category })}
							{#if d.tags?.length} · <span data-testid="downloader-tags">{t('settings.downloaders.tagsList', { tags: d.tags.join(', ') })}</span>{/if}
							{#if d.allow_local_network} · {t('settings.downloaders.mayReach')}{/if}
							· {d.writable && !d.writable_locked ? t('settings.downloaders.canChange') : d.writable_locked ? t('settings.downloaders.readOnly') : t('settings.downloaders.handsOff')}
						</div>
					</div>
					<div class="num" title={d.checked_at ? t('settings.downloaders.checked', { ago: ago(d.checked_at) }) : undefined}><b>{d.active_downloads}</b><small>{t('settings.downloaders.active')}</small></div>
					<span class="st {healthTone(d.health.state)}"><i></i><span data-testid="downloader-state">{d.health.label}</span></span>
					<div class="acts">
						<Button variant="plain" size="sm" type="button" disabled={testing === d.id} aria-busy={testing === d.id} onclick={() => test(d)}>{t('settings.downloaders.test')}</Button>
						{#if d.health.state === 'paused'}
							<Button variant="plain" size="sm" type="button" onclick={() => setPaused(d, false)}>{t('settings.downloaders.resume')}</Button>
						{:else}
							<Button variant="plain" size="sm" type="button" onclick={() => setPaused(d, true)}>{t('settings.downloaders.pause')}</Button>
						{/if}
						<Button variant="plain" size="sm" type="button" onclick={() => startChange(d)}>{t('settings.downloaders.changeButton')}</Button>
						<Button variant="plain" size="sm" type="button" onclick={() => remove(d)} aria-label={t('settings.downloaders.removeName', { name: d.name })}>{t('settings.downloaders.remove')}</Button>
					</div>
				</div>
				{#if d.health.text}<p class="note" class:bad-text={d.health.state !== 'working' && d.health.state !== 'paused'} data-testid="downloader-text">{serverWords(d.health.text)}</p>{/if}
				{#if results[d.id] && !results[d.id].ok}
					<p class="note bad-text" role="status" data-testid="downloader-test">{results[d.id].text}</p>
				{/if}
				{#if pendingAttempts(attempts[d.id]).length > 0}
					<section class="notice bad attempt-review" aria-label={t('settings.downloaders.needsDecision')} data-testid="downloader-attempts">
						<h3 class="h3">{t('settings.downloaders.checkArrived')}</h3>
						<p class="small">{t('settings.downloaders.couldNotConfirm', { app: kindWords[d.kind] })}</p>
						{#each pendingAttempts(attempts[d.id]) as attempt (attempt.id)}
							<div class="attempt-row">
								<p class="small">{t('settings.downloaders.attemptLine', { release: attempt.release_id, slot: attempt.slot_id, ago: ago(attempt.created_at) })}</p>
								<Button variant="plain" type="button" disabled={abandoning === attempt.id} aria-busy={abandoning === attempt.id} onclick={() => abandonAttempt(d, attempt)}>{t('settings.downloaders.giveUp')}</Button>
							</div>
						{/each}
						<p class="xs muted">{t('settings.downloaders.wontRetry', { app: kindWords[d.kind] })}</p>
					</section>
				{:else if attemptErrors[d.id]}
					<p class="note bad-text" role="status">{t('settings.downloaders.couldNotCheck', { error: attemptErrors[d.id] })}</p>
				{/if}
			</li>
		{/each}
	</ul>
{/if}

{#if !saved}
	{#if setLoadError}
		<div class="settings"><LoadError hasData={false} message={setLoadError} onretry={loadSettings} busy={setLoading} /></div>
	{:else}
		<div class="settings"><SkSettings rows={2} /></div>
	{/if}
{:else}
<form class="sc-form settings" id="wrong-form" onsubmit={saveSettings} aria-labelledby="h-wrong">
	<SettingsCard id="wrong-card" title={t('settings.downloaders.wrong')}>
		<span id="h-wrong" class="sr-only">{t('settings.downloaders.wrong')}</span>
		<label class="checkline">
			<input id="remove-failed" type="checkbox" bind:checked={settings.remove_failed} />
			<span><b>{t('settings.downloaders.removeFailedDl')}</b><br /><span class="hint">{t('settings.downloaders.removeFailedHint')}</span></span>
		</label>
		<label class="checkline">
			<input id="auto-add" type="checkbox" bind:checked={settings.downloads_auto_add} />
			<span><b>{t('settings.downloaders.autoAdd')}</b><br /><span class="hint">{t('settings.downloaders.autoAddHint')}</span></span>
		</label>
		<fieldset class="defaults" id="remove-defaults" tabindex="-1">
			<legend>{t('settings.downloaders.whenRemove')}</legend>
		<label class="checkline">
			<input type="checkbox" bind:checked={settings.remove_delete_files} />
			<span><b>{t('settings.downloaders.deleteFiles')}</b></span>
		</label>
		<label class="checkline">
			<input type="checkbox" bind:checked={settings.remove_turn_down} />
			<span><b>{t('settings.downloaders.turnDown')}</b><br /><span class="hint">{t('settings.downloaders.turnDownHint')}</span></span>
		</label>
		<label class="checkline">
			<input type="checkbox" bind:checked={settings.remove_search_again} />
			<span><b>{t('settings.downloaders.searchAgain')}</b><br /><span class="hint">{t('settings.downloaders.searchAgainHint')}</span></span>
		</label>
		</fieldset>
		{#snippet advanced()}
			<div class="two">
				<Field id="st-stall" label={t('settings.downloaders.stalledAfter')} hint={t('settings.downloaders.stalledAfterHint')}>
					<input id="st-stall" class="input" type="number" min="1" max="168" step="0.5" bind:value={settings.stalled_after_hours} />
				</Field>
				<Field id="st-act" label={t('settings.downloaders.tryNext')} hint={t('settings.downloaders.tryNextHint')}>
					<input id="st-act" class="input" type="number" min="1" max="720" step="0.5" bind:value={settings.stalled_act_hours} />
				</Field>
			</div>
			<label class="checkline">
				<input id="fail-programs" type="checkbox" bind:checked={settings.fail_dangerous} />
				<span
					><b>{t('settings.downloaders.failPrograms')}</b><br /><span class="hint"
						>{t('settings.downloaders.failProgramsHint')}</span
					></span
				>
			</label>
			<label class="checkline">
				<input id="replace-removed" type="checkbox" bind:checked={settings.replace_vanished} />
				<span><b>{t('settings.downloaders.replace')}</b><br /><span class="hint">{t('settings.downloaders.replaceHint')}</span></span>
			</label>
			<label class="checkline">
				<input id="try-next-dl" type="checkbox" bind:checked={settings.try_next_downloader} />
				<span><b>{t('settings.downloaders.tryNextDl')}</b><br /><span class="hint">{t('settings.downloaders.tryNextDlHint')}</span></span>
			</label>
			<Field id="st-gap" label={t('settings.downloaders.spacing')}>
				<input id="st-gap" class="input narrow" type="number" min="0" max="60" step="0.5" bind:value={settings.grab_spacing_seconds} />
			</Field>
		{/snippet}
	</SettingsCard>
	{#if setError}<p class="form-error" role="alert">{setError}</p>{/if}
</form>
<SaveBar {dirty} form="wrong-form" saving={setBusy} ondiscard={() => (settings = JSON.parse(saved))} />
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
	.settings {
		margin-top: var(--s-6, 32px);
	}
	.defaults {
		border: 0;
		padding: 0;
		margin: 0;
	}
	/* A small sub-heading inside the sheet, never sitting on the divider line. */
	.defaults > legend {
		float: left;
		width: 100%;
		padding: 18px 0 6px;
		color: var(--text-muted);
		font: 600 11px/1.2 var(--font-ui);
		letter-spacing: 0.08em;
		text-transform: uppercase;
	}
	.defaults > legend + * { clear: both; }
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
	.attempt-review {
		display: block;
		margin-top: var(--s-3);
	}
	.attempt-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--s-3);
		margin-top: var(--s-2);
	}
	.kinds {
		flex-wrap: wrap;
		gap: var(--s-4);
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
		margin: var(--s-4) 0 0;
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
		.attempt-row {
			align-items: stretch;
			flex-direction: column;
		}
	}

	/* The list: one glass sheet, one row per Downloader (the Library folders pattern). */
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
</style>
