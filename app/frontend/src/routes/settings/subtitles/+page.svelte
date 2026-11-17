<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import MoreOptions from '#lib/ui/MoreOptions.svelte';
	import SaveBar from '#lib/ui/SaveBar.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import TestTag from '#lib/ui/TestTag.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import { LanguageWants as WishEditor, accountMissing, accountWords, kindCovers, siteBody, siteForm, siteState, translatorBody, wishProblem, type Language, type Translator, type TranslatorForm, type SiteForm, type SiteKind, type SiteView, type SubtitleSettings } from '#lib/subtitles/index.ts';
	import { onMount } from 'svelte';
	import { testCount } from '#lib/core/test-tag.ts';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { dateTime } from '#lib/core/format.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { live } from '#lib/core/live.svelte.ts';

	let settings = $state<SubtitleSettings | null>(null);
	let langs = $state<Language[]>([]);
	let kinds = $state<SiteKind[]>([]);
	let sites = $state<SiteView[]>([]);
	let loaded = $state(false);
	let saved = $state('');
	const dirty = $derived(!!settings && !!saved && JSON.stringify(settings) !== saved);
	let error = $state('');
	let setBusy = $state(false);
	let setError = $state('');

	// The translator saves on its own: its key never joins the page's draft.
	let tr = $state<TranslatorForm | null>(null);
	let trSaved = $state('');
	const trDirty = $derived(!!tr && !!trSaved && JSON.stringify(tr) !== trSaved);
	let trBusy = $state(false);
	let trTesting = $state(false);
	let trError = $state('');
	let trResult = $state<{ ok: boolean; message: string; models: string[] } | null>(null);
	const trReady = $derived(!!tr && JSON.parse(trSaved || '{}').kind !== '' && !!JSON.parse(trSaved || '{}').url);

	function setTranslator(x: Translator) {
		tr = { ...x, key: '', clear_key: false };
		trSaved = JSON.stringify(tr);
	}

	async function saveTranslator(e: SubmitEvent) {
		e.preventDefault();
		if (!tr) return;
		trBusy = true;
		trError = '';
		try {
			setTranslator(await api.put<Translator>('/settings/subtitles/translator', translatorBody(tr)));
			toasts.push(t('settings.subtitles.trSaved'), 'ok');
			if (tr.kind) await testTranslator();
		} catch (err) {
			trError = message(err);
		} finally {
			trBusy = false;
		}
	}

	async function testTranslator() {
		trTesting = true;
		try {
			trResult = await api.post<{ ok: boolean; message: string; models: string[] }>('/settings/subtitles/translator/test');
		} catch (err) {
			trResult = { ok: false, message: message(err), models: [] };
		} finally {
			trTesting = false;
		}
	}

	let editing = $state<SiteView | null>(null);
	let isNew = $state(false);
	let form = $state<SiteForm | null>(null);
	let formError = $state('');
	let busy = $state(false);
	let testing = $state(0);
	let results = $state<Record<number, { ok: boolean; text: string }>>({});

	const kindOf = (k: string) => kinds.find((x) => x.kind === k);
	const formKind = $derived(form ? kindOf(form.kind) : undefined);
	const needs = $derived(formKind && form ? accountMissing(formKind, form, !isNew && !!editing?.has_account) : []);

	async function loadSites() {
		try {
			sites = (await api.get<{ sites: SiteView[] }>('/subtitle-sites')).sites;
		} catch (e) {
			error = message(e);
		}
	}

	function load() {
		error = '';
		return Promise.all([
			api.get<SubtitleSettings>('/settings/subtitles').then((s) => {
				settings = s;
				saved = JSON.stringify(s);
			}),
			api.get<{ languages: Language[] }>('/subtitles/languages').then((r) => (langs = r.languages)),
			api.get<Translator>('/settings/subtitles/translator').then(setTranslator),
			api.get<{ kinds: SiteKind[] }>('/subtitle-sites/kinds').then((r) => (kinds = r.kinds)),
			loadSites()
		])
			.catch((e) => (error = message(e)))
			.finally(() => (loaded = true));
	}

	onMount(() => {
		void load();
		const offs = [
			live.on('event', (d) => {
				if (String((d as { Kind?: string })?.Kind) === 'subtitle_site_paused') void loadSites();
			}),
			live.on('resync', () => {
				void loadSites();
			})
		];
		return () => offs.forEach((off) => off());
	});

	async function saveSettings(e: SubmitEvent) {
		e.preventDefault();
		if (!settings) return;
		setError = wishProblem(settings.languages);
		if (setError) return;
		setBusy = true;
		try {
			const body = { ...settings };
			for (const k of ['pass_mark_movie', 'pass_mark_episode', 'search_hours', 'better_hours', 'better_days', 'batch', 'sync_from_fps', 'sync_to_fps', 'sync_step_ms'] as const) body[k] = Number(body[k]);
			settings = await api.put<SubtitleSettings>('/settings/subtitles', body);
			saved = JSON.stringify(settings);
			toasts.push(t('settings.subtitles.saved'), 'ok');
		} catch (err) {
			setError = message(err);
		} finally {
			setBusy = false;
		}
	}

	function startAdd() {
		const k = kinds.find((x) => !x.needs_account && !x.later && !x.default_off) ?? kinds.find((x) => !x.later);
		if (!k) return;
		isNew = true;
		editing = { ID: 0 } as SiteView;
		form = siteForm(k);
		formError = '';
	}

	function pickKind(k: SiteKind) {
		form = siteForm(k);
	}

	function startChange(s: SiteView) {
		const k = kindOf(s.kind);
		if (!k) return;
		isNew = false;
		editing = s;
		form = siteForm(k, s);
		formError = '';
	}

	async function save(e: SubmitEvent) {
		e.preventDefault();
		if (!form) return;
		if (needs.length) {
			formError = t('settings.subtitles.needsAccount', { fields: needs.map((n) => accountWords[n].toLowerCase()).join(t('settings.subtitles.and')) });
			return;
		}
		busy = true;
		formError = '';
		try {
			const body = siteBody(form);
			const saved = isNew ? await api.post<SiteView>('/subtitle-sites', body) : await api.put<SiteView>(`/subtitle-sites/${editing!.ID}`, body);
			toasts.push(t('settings.subtitles.siteSaved', { name: saved.name }), 'ok');
			editing = null;
			form = null;
			await loadSites();
			await test(saved);
		} catch (err) {
			formError = message(err);
		} finally {
			busy = false;
		}
	}

	async function test(s: SiteView) {
		testing = s.ID;
		try {
			const r = await api.post<{ ok: boolean; message: string }>(`/subtitle-sites/${s.ID}/test`);
			results[s.ID] = { ok: r.ok, text: r.message };
		} catch (err) {
			results[s.ID] = { ok: false, text: message(err) };
		} finally {
			testing = 0;
		}
	}

	async function setPaused(s: SiteView, paused: boolean) {
		const k = kindOf(s.kind);
		if (!k) return;
		try {
			// A change without account details keeps the stored ones.
			await api.put(`/subtitle-sites/${s.ID}`, { ...siteBody(siteForm(k, s)), enabled: !paused });
			toasts.push(paused ? t('settings.subtitles.paused', { name: s.name }) : t('settings.subtitles.resumed', { name: s.name }), 'ok', paused ? { body: t('settings.subtitles.skipIt') } : undefined);
			await loadSites();
		} catch (err) {
			toasts.push(paused ? t('settings.subtitles.couldntPause') : t('settings.subtitles.couldntResume'), 'fail', { body: message(err) });
		}
	}

	async function remove(s: SiteView) {
		if (!confirm(t('settings.subtitles.confirmRemove', { name: s.name }))) return;
		try {
			await api.del(`/subtitle-sites/${s.ID}`);
			await loadSites();
		} catch (err) {
			toasts.push(t('settings.subtitles.removeFailed'), 'fail', { body: message(err) });
		}
	}
</script>

<svelte:head><title>{t('settings.subtitles.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.subtitles.title')} />

{#if !loaded}
	<SkSettings rows={2} />
{:else if error && !settings}
	<LoadError hasData={false} message={error} onretry={load} />
{:else if settings}
	{#if error}<LoadError hasData message={error} onretry={load} />{/if}
	<form class="sc-form" id="subtitle-form" onsubmit={saveSettings} aria-label={t('settings.subtitles.defaultLangs')}>
		<SettingsCard id="lang-card" title={t('settings.subtitles.defaultLangs')} description={t('settings.subtitles.defaultLangsDesc')}>
			<WishEditor bind:wants={settings.languages} {langs} id="def" translate={trReady} />
			{#if settings.languages.length}<p class="xs muted hint-block">
				<b>{t('settings.subtitles.hiLabel')}</b> {t('settings.subtitles.hiText')}
				<b>{t('settings.subtitles.forcedLabel')}</b> {t('settings.subtitles.forcedText')}
			</p>{/if}
	
			{#snippet advanced()}
				<div class="two">
					<Field id="pm-ep" label={t('settings.subtitles.passEpisodes')} hint={t('settings.subtitles.passHint')}>
						<input id="pm-ep" class="input narrow" type="number" min="0" max="100" bind:value={settings!.pass_mark_episode} />
					</Field>
					<Field id="pm-mov" label={t('settings.subtitles.passMovies')} hint={t('settings.subtitles.passHint')}>
						<input id="pm-mov" class="input narrow" type="number" min="0" max="100" bind:value={settings!.pass_mark_movie} />
					</Field>
				</div>
				<details class="more" data-testid="pass-mark-help">
					<summary>{t('settings.subtitles.passMeans')}</summary>
					<p class="small">
						{t('settings.subtitles.pass1')}
					</p>
					<p class="small">
						{t('settings.subtitles.pass2')}
					</p>
				</details>
	
			{/snippet}
		</SettingsCard>
		{#if setError}<p class="form-error" role="alert">{setError}</p>{/if}
	</form>
	{#if tr}
		<form class="sc-form" onsubmit={saveTranslator} aria-label={t('settings.subtitles.translate')}>
			<SettingsCard id="translator-card" title={t('settings.subtitles.translate')} description={t('settings.subtitles.translateDesc')}>
				<Field id="tr-kind" label={t('settings.subtitles.translator')} hint={tr.kind === 'openai' ? t('settings.subtitles.trOpenAIHint') : tr.kind === 'lingarr' ? t('settings.subtitles.trLingarrHint') : ''}>
					<select id="tr-kind" class="input" bind:value={tr.kind} data-testid="translator-kind">
						<option value="">{t('settings.subtitles.trOff')}</option>
						<option value="openai">{t('settings.subtitles.trOpenAI')}</option>
						<option value="lingarr">Lingarr</option>
					</select>
				</Field>
				{#if tr.kind}
					<div class="two">
						<Field id="tr-url" label={t('settings.subtitles.address')}>
							<input id="tr-url" class="input mono" bind:value={tr.url} autocomplete="off" spellcheck="false" placeholder={tr.kind === 'openai' ? 'http://192.168.1.20:11434/v1' : 'http://192.168.1.20:9876'} required />
						</Field>
						{#if tr.kind === 'openai'}
							<Field id="tr-model" label={t('settings.subtitles.trModel')}>
								<input id="tr-model" class="input mono" bind:value={tr.model} list="tr-models" autocomplete="off" spellcheck="false" required />
								<datalist id="tr-models">{#each trResult?.models ?? [] as m (m)}<option value={m}></option>{/each}</datalist>
							</Field>
						{/if}
					</div>
					<Field id="tr-key" label={t('settings.subtitles.trKey')} hint={tr.has_key ? t('settings.subtitles.trKeyKept') : t('settings.subtitles.trKeyHint')} style="margin-top: var(--s-4)">
						<input id="tr-key" class="input mono" type="password" bind:value={tr.key} autocomplete="new-password" spellcheck="false" placeholder={tr.has_key ? '••••••••' : t('settings.subtitles.trKeyEmpty')} />
					</Field>
					{#if tr.has_key && !tr.key}
						<label class="checkline">
							<input type="checkbox" bind:checked={tr.clear_key} />
							<span><b>{t('settings.subtitles.trClearKey')}</b></span>
						</label>
					{/if}
					<label class="checkline">
						<input type="checkbox" bind:checked={tr.built_in_auto} data-testid="translator-built-in" />
						<span><b>{t('settings.subtitles.trBuiltIn')}</b><br /><span class="hint">{t('settings.subtitles.trBuiltInHint')}</span></span>
					</label>
				{/if}
				{#if trResult && !trDirty}
					<div class="testline" data-testid="translator-test"><TestTag ok={trResult.ok} text={trResult.message} />{#if !trResult.ok}<span class="small bad-text">{trResult.message}</span>{/if}</div>
				{/if}
				{#if trError}<p class="form-error" role="alert">{trError}</p>{/if}
				<div class="row actions">
					<Button variant="primary" type="submit" disabled={trBusy || !trDirty} busy={trBusy}>{t('settings.subtitles.save')}</Button>
					{#if tr.kind}<Button type="button" disabled={trTesting || trDirty} busy={trTesting} onclick={testTranslator} data-testid="translator-test-button">{t('settings.subtitles.test')}</Button>{/if}
				</div>
			</SettingsCard>
		</form>
	{/if}
	<section class="sites-sec" id="sites-card" aria-labelledby="h-sites" tabindex="-1">
		<div class="sites-head">
			<div>
				<h2 id="h-sites" class="h2">{t('settings.subtitles.sites')}</h2>
				<p class="small muted">{t('settings.subtitles.sitesNote')}</p>
			</div>
			{#if !editing && sites.length}<Button icon="plus" variant="primary" type="button" onclick={startAdd}>{t('settings.subtitles.add')}</Button>{/if}
		</div>

		{#if editing && form}
			<form class="card" onsubmit={save} aria-labelledby="h-site-form">
				<h3 id="h-site-form" class="h3">{isNew ? t('settings.subtitles.add') : t('settings.subtitles.change', { name: editing.name })}</h3>
				{#if isNew}
					<fieldset class="group">
						<legend class="label">{t('settings.subtitles.whichSite')}</legend>
						{#each kinds.filter((x) => !x.later) as k (k.kind)}
							<label class="checkline">
								<input type="radio" name="site-kind" checked={form.kind === k.kind} disabled={k.later} onchange={() => pickKind(k)} />
								<span>
									<b>{k.name}</b> ({kindCovers(k)}){#if k.needs_account}{' '}<span class="chip fail">{t('settings.subtitles.needsYours')}</span>{/if}
									{#if k.note}<br /><span class="hint">{k.note}</span>{/if}
								</span>
							</label>
						{/each}
					</fieldset>
				{/if}
				<div class="two">
					<Field id="st-name" label={t('settings.subtitles.name')}>
						<input id="st-name" class="input" bind:value={form.name} maxlength="100" required />
					</Field>
					<Field id="st-prio" label={t('settings.subtitles.priority')} hint={t('settings.subtitles.priorityHint')}>
						<input id="st-prio" class="input narrow" type="number" min="1" max="100" bind:value={form.priority} />
					</Field>
				</div>
				<Field id="st-url" label={t('settings.subtitles.address')} style="margin-top: var(--s-4)">
					<input id="st-url" class="input mono" bind:value={form.url} autocomplete="off" spellcheck="false" required />
				</Field>
				{#if formKind?.needs_account}
					<fieldset class="group account" data-testid="site-account">
						<legend class="label">{t('settings.subtitles.yourAccount')}</legend>
						<p class="small muted">
							{t('settings.subtitles.accountNote')}{#if !isNew && editing.has_account}
								{t('settings.subtitles.accountKeep')}{/if}
						</p>
						{#each formKind.account ?? [] as a (a)}
							<Field id="st-acct-{a}" label={accountWords[a]}>
								{#if a === 'username'}
									<input id="st-acct-{a}" class="input" bind:value={form.username} autocomplete="off" />
								{:else if a === 'password'}
									<input id="st-acct-{a}" class="input" type="password" bind:value={form.password} autocomplete="new-password" />
								{:else}
									<input id="st-acct-{a}" class="input mono" type="password" bind:value={form.api_key} autocomplete="new-password" spellcheck="false" />
								{/if}
							</Field>
						{/each}
					</fieldset>
				{/if}
				<details class="more">
					<summary>{t('settings.subtitles.limits')}</summary>
					<div class="two">
						<Field id="st-gap" label={t('settings.subtitles.gap')}>
							<input id="st-gap" class="input narrow" type="number" min="0" max="60000" bind:value={form.limits.gap_ms} />
						</Field>
						<Field id="st-per" label={t('settings.subtitles.perWindow')}>
							<input id="st-per" class="input narrow" type="number" min="1" max="10000" bind:value={form.limits.per_window} />
						</Field>
						<Field id="st-win" label={t('settings.subtitles.window')}>
							<input id="st-win" class="input narrow" type="number" min="60" max="86400" bind:value={form.limits.window_seconds} />
						</Field>
						<Field id="st-day" label={t('settings.subtitles.perDay')}>
							<input id="st-day" class="input narrow" type="number" min="0" max="100000" bind:value={form.limits.downloads_per_day} />
						</Field>
					</div>
				</details>
				<label class="checkline">
					<input type="checkbox" bind:checked={form.allow_local_network} />
					<span><b>{t('settings.subtitles.local')}</b><br /><span class="hint">{t('settings.subtitles.localHint')}</span></span>
				</label>
				<label class="checkline">
					<input type="checkbox" bind:checked={form.machine_made} data-testid="site-machine-made" />
					<span><b>{t('settings.subtitles.machineMade')}</b><br /><span class="hint">{t('settings.subtitles.machineMadeHint')}</span></span>
				</label>
				<label class="checkline">
					<input type="checkbox" bind:checked={form.enabled} />
					<span><b>{t('settings.subtitles.on')}</b></span>
				</label>
				{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
				<div class="row" style="margin-top: var(--s-5)">
					<Button variant="primary" type="submit" disabled={busy || !form.name.trim() || !form.url.trim()} aria-busy={busy}>{t('settings.subtitles.save')}</Button>
					<Button variant="plain" type="button" onclick={() => ((editing = null), (form = null))}>{t('settings.subtitles.cancel')}</Button>
				</div>
			</form>
		{/if}

		{#if sites.length === 0 && !editing}
			<EmptyState compact icon="subtitle" title={t('settings.subtitles.none')} text={t('settings.subtitles.noneText')} action={{ label: t('settings.subtitles.add'), icon: 'plus', onclick: startAdd }} />
		{:else}
			<ul class="list">
				{#each sites as s (s.ID)}
					{@const k = kindOf(s.kind)}
					{@const st = siteState(s, k, dateTime)}
					<li class="card" data-testid="subtitle-site">
						<div class="head">
							<span class="ic"><Icon name="subtitle" /></span>
							<div class="grow">
								<h3 class="h3">{s.name}</h3>
								<div class="mono xs muted" style="overflow-wrap: anywhere">{s.url}</div>
							</div>
							<span class="chip {st.tone}" data-testid="subtitle-site-state">{st.label}</span>
						</div>
						<p class="small" data-testid="subtitle-site-text">{st.text}</p>
						<p class="xs muted">
							{k?.name ?? s.kind} · {k ? kindCovers(k) : ''} · {t('settings.subtitles.priorityN', { n: s.priority })}
							{#if k?.needs_account} · {s.has_account ? t('settings.subtitles.accountSaved') : t('settings.subtitles.accountNotSet')}{/if}
							{#if s.allow_local_network} · {t('settings.subtitles.mayReach')}{/if}
							{#if s.limits.downloads_per_day} · {t('settings.subtitles.downloadsToday', { n: s.downloads_today, max: s.limits.downloads_per_day })}{/if}
						</p>
						{#if results[s.ID]}
							<div class="testline" data-testid="subtitle-site-test"><TestTag ok={results[s.ID].ok} text={results[s.ID].text} app={k?.name} extra={results[s.ID].ok ? testCount(results[s.ID].text) : ''} />{#if !results[s.ID].ok}<span class="small bad-text">{results[s.ID].text}</span>{/if}</div>
						{/if}
						<div class="row actions">
							<Button type="button" disabled={testing === s.ID} aria-busy={testing === s.ID} onclick={() => test(s)} aria-label={t('settings.subtitles.testName', { name: s.name })}>{t('settings.subtitles.test')}</Button>
							{#if s.state === 'disabled'}
								<Button variant="plain" type="button" onclick={() => setPaused(s, false)} aria-label={t('settings.subtitles.turnOnName', { name: s.name })}>{t('settings.subtitles.turnOn')}</Button>
							{:else}
								<Button variant="plain" type="button" onclick={() => setPaused(s, true)} aria-label={t('settings.subtitles.pauseName', { name: s.name })}>{t('settings.subtitles.pause')}</Button>
							{/if}
							<Button variant="plain" type="button" onclick={() => startChange(s)} aria-label={t('settings.subtitles.changeName', { name: s.name })}>{t('settings.subtitles.changeButton')}</Button>
							<Button variant="plain" type="button" onclick={() => remove(s)} aria-label={t('settings.subtitles.removeName', { name: s.name })}>{t('settings.subtitles.remove')}</Button>
						</div>
					</li>
				{/each}
			</ul>
		{/if}
	</section>

	<div class="sc-form sub-more">
		<SettingsCard id="when-to-look" title={t('settings.subtitles.whenToLook')} description={t('settings.subtitles.whenToLookDesc')}>
			<label class="checkline">
				<input id="better-subs" form="subtitle-form" type="checkbox" bind:checked={settings.better} />
				<span><b>{t('settings.subtitles.better')}</b><br /><span class="hint">{t('settings.subtitles.betterHint')}</span></span>
			</label>
			{#snippet advanced()}
			<div class="two">
				<Field id="s-hours" label={t('settings.subtitles.searchHours')}>
					<input id="s-hours" form="subtitle-form" class="input narrow" type="number" min="1" max="168" bind:value={settings!.search_hours} />
				</Field>
				<Field id="s-batch" label={t('settings.subtitles.batch')} hint={t('settings.subtitles.batchHint')}>
					<input id="s-batch" form="subtitle-form" class="input narrow" type="number" min="1" max="1000" bind:value={settings!.batch} />
				</Field>
			</div>
				<label class="checkline">
					<input id="slow-down" form="subtitle-form" type="checkbox" bind:checked={settings!.adaptive} />
					<span><b>{t('settings.subtitles.slowDown')}</b><br /><span class="hint">{t('settings.subtitles.slowDownHint')}</span></span>
				</label>
				{#if settings!.better}
					<div class="two">
						<Field id="b-hours" label={t('settings.subtitles.everyHours')}>
							<input id="b-hours" form="subtitle-form" class="input narrow" type="number" min="6" max="168" bind:value={settings!.better_hours} />
						</Field>
						<Field id="b-days" label={t('settings.subtitles.lastDays')}>
							<input id="b-days" form="subtitle-form" class="input narrow" type="number" min="0" max="30" bind:value={settings!.better_days} />
						</Field>
					</div>
				{/if}
			{/snippet}
		</SettingsCard>
	</div>
	<MoreOptions>
		<div class="sc-form">
			<SettingsCard id="inside-video" title={t('settings.subtitles.inside')}>
				<label class="checkline">
					<input id="embedded" form="subtitle-form" type="checkbox" bind:checked={settings.use_built_in} />
					<span><b>{t('settings.subtitles.countInside')}</b><br /><span class="hint">{t('settings.subtitles.countInsideHint')}</span></span>
				</label>
				{#snippet advanced()}
					<label class="checkline">
						<input id="no-picture" form="subtitle-form" type="checkbox" bind:checked={settings!.ignore_picture_tracks} disabled={!settings!.use_built_in} />
						<span><b>{t('settings.subtitles.noPicture')}</b><br /><span class="hint">{t('settings.subtitles.noPictureHint')}</span></span>
					</label>
					<Field id="hi-suffix" label={t('settings.subtitles.hiSuffix')} hint={t('settings.subtitles.hiSuffixHint')}>
						<select id="hi-suffix" form="subtitle-form" class="input narrow" bind:value={settings!.hi_suffix}>
							<option value="hi">.hi (Movie.en.hi.srt)</option>
							<option value="cc">.cc (Movie.en.cc.srt)</option>
							<option value="sdh">.sdh (Movie.en.sdh.srt)</option>
						</select>
					</Field>
				{/snippet}
			</SettingsCard>
			<SettingsCard id="sync-fix" title={t('settings.subtitles.syncFix')} description={t('settings.subtitles.syncFixDesc')}>
				<p class="small muted">
					{t('settings.subtitles.syncNote')}
				</p>
				{#snippet advanced()}
					<div class="three">
						<Field id="sy-step" label={t('settings.subtitles.shift')}>
							<input id="sy-step" form="subtitle-form" class="input narrow" type="number" min="10" max="60000" bind:value={settings!.sync_step_ms} />
						</Field>
						<Field id="sy-from" label={t('settings.subtitles.fpsFrom')}>
							<input id="sy-from" form="subtitle-form" class="input narrow" type="number" min="1" max="120" step="0.001" bind:value={settings!.sync_from_fps} />
						</Field>
						<Field id="sy-to" label={t('settings.subtitles.fpsTo')}>
							<input id="sy-to" form="subtitle-form" class="input narrow" type="number" min="1" max="120" step="0.001" bind:value={settings!.sync_to_fps} />
						</Field>
					</div>
				{/snippet}
			</SettingsCard>
		</div>
	</MoreOptions>
	<SaveBar {dirty} form="subtitle-form" saving={setBusy} ondiscard={() => (settings = JSON.parse(saved))} />

{/if}

<style>
	.testline { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 10px; }
	.sec {
		margin-top: var(--s-6, 32px);
	}
	.hint-block {
		margin-top: var(--s-3);
	}
	.two,
	.three {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--s-3);
		margin-top: var(--s-4);
	}
	.three {
		grid-template-columns: 1fr 1fr 1fr;
	}
	.two :global(.field + .field),
	.three :global(.field + .field) {
		margin-top: 0;
	}
	.narrow {
		max-width: 220px;
	}
	.more {
		margin-top: var(--s-4);
	}
	.more summary {
		cursor: pointer;
		font-weight: var(--weight-semibold);
	}
	.more p {
		margin-top: var(--s-2);
	}
	.sites-sec {
		margin-top: 0;
	}
	/* The heading reads like every section heading: small and grey, its note beside it. */
	.sites-head > div { display: flex; align-items: baseline; flex-wrap: wrap; gap: 2px 10px; padding: 0 6px; }
	.sites-head .h2 { color: var(--text-muted); font: 600 13px/1.2 var(--font-ui); letter-spacing: 0.02em; }
	.sites-head p { margin: 0; color: var(--text-muted); font: 400 12px/1.2 var(--font-ui); }
	.sites-head {
		display: flex;
		justify-content: space-between;
		align-items: flex-end;
		gap: var(--s-3);
		margin-bottom: 8px;
	}
	/* No sites yet: the empty note sits in the same glass sheet as every other section. */
	.sites-sec > :global(.ui-empty) { width: auto; max-width: none; margin: 0; padding: 22px 18px; border: 1px solid color-mix(in srgb, var(--text) 10%, transparent); border-radius: 20px; background: color-mix(in srgb, var(--surface-1) 60%, transparent); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.2), 0 24px 50px -32px rgb(0 0 0 / 0.45); }
	.group {
		border: 0;
		padding: 0;
		margin: var(--s-4) 0 0;
	}
	.account :global(.field) {
		margin-top: var(--s-3);
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
	.bad-text {
		color: var(--fail-text);
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.ok-text {
		color: var(--ok-text, inherit);
	}
	@container settings (max-width: 600px) {
		.two,
		.three {
			grid-template-columns: 1fr;
		}
		.sites-head {
			flex-direction: column;
		}
	}
	/* No languages yet: the note and Add a language share one row. */
	:global(#lang-card > .body:has(> .wants > li.small)) { flex-direction: row !important; align-items: center; justify-content: space-between; min-height: 64px; }
	:global(#lang-card > .body:has(> .wants > li.small) > *) { margin: 0 !important; box-shadow: none !important; padding-block: 12px !important; }
	:global(#lang-card > .body:has(> .wants > li.small) > .add) { padding-left: 0 !important; }
</style>
