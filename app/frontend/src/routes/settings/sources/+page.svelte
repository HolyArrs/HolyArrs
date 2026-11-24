<script lang="ts">
	import { FromPlugin } from '#lib/plugins/index.ts';
	import { editForm, withoutEditing } from '#lib/ui/edit-in-place.ts';
	import { ask } from '#lib/ui/ask.svelte.ts';
	import { ltr, t } from '#lib/core/i18n/i18n.svelte.ts';
	import { serverWords } from '#lib/core/i18n/server-words.ts';
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
	import ConnectHelp from '#lib/ui/ConnectHelp.svelte';
	import { ConnectBridge } from '#lib/sources/index.ts';
	import { partOff } from '#lib/mediafiles/index.ts';
	import { healthTone, healthWord, pausedUntil, SourceCheck, emptyForm, formBody, formFrom, ms, uses, builtInSites, pickSite, sendToChoices, type SourceForm } from '#lib/sources/index.ts';
	import type { SourceHealth, SourceView } from '#lib/sources/index.ts';
	import { onMount } from 'svelte';
	import { to } from '#lib/core/paths.ts';
	import { api } from '#lib/core/api.ts';
	import { recheckSettingsStatus } from '#lib/alerts/index.ts';
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
	let downloaders = $state<{ id: number; name: string; protocol: string }[] | null>(null);
	let sendGone = $state(false);
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
	// Not every account may list Downloaders; then the choice stays hidden.
	async function loadDownloaders() {
		try {
			downloaders = (await api.get<{ downloaders: { id: number; name: string; protocol: string }[] }>('/downloaders')).downloaders ?? [];
		} catch {
			downloaders = null;
		}
	}
	onMount(() => {
		load();
		loadDownloaders();
	});

	function startAdd() {
		isNew = true;
		editing = { id: 0, name: '' } as SourceView;
		form = emptyForm();
		sendGone = false;
		formError = '';
	}

	function startChange(s: SourceView) {
		isNew = false;
		editing = s;
		form = formFrom(s);
		sendGone = false;
		formError = '';
	}

	const sendProto = $derived(form.format === 'site' ? 'torrent' : form.protocol);
	const sendTo = $derived(sendToChoices(downloaders ?? [], sendProto, form.downloader_id));
	// A chosen Downloader that is not on the list (removed, or of the other
	// kind after a switch) means any Downloader. Only a removed one is worth a
	// warning.
	$effect(() => {
		if (!downloaders || !sendTo.missing) return;
		sendGone = !!editing && sendProto === editing.protocol && form.downloader_id === (editing.downloader_id ?? 0);
		form.downloader_id = 0;
	});
	const siteNeedsKey = $derived(builtInSites.find((x) => x.id === form.site)?.key ?? false);
	const siteNeedsUser = $derived(builtInSites.find((x) => x.id === form.site)?.user ?? false);

	function setFormat(v: string) {
		form.format = v as SourceForm['format'];
		if (form.format === 'site' && !form.site) pickSite(form, builtInSites[0].id);
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
			toasts.push(t('settings.sources.saved', { name: saved.name }), 'ok');
			forgetTags();
			editing = null;
			await load();
			// A changed Source is tested again, so its state never rests on an old test.
			await test(saved);
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
			// A test that works clears its warning and the menu dot at once.
			void recheckSettingsStatus('source');
		}
	}

	async function setPaused(s: SourceView, paused: boolean) {
		try {
			await api.post(`/sources/${s.id}/${paused ? 'pause' : 'resume'}`);
			await load();
		} catch (err) {
			toasts.push(paused ? t('settings.sources.couldntPause') : t('settings.sources.couldntResume'), 'fail', { body: message(err) });
		}
	}

	async function remove(s: SourceView) {
		if (!(await ask(t('settings.sources.confirmRemove', { name: s.name }), { yes: t('ui.ask.remove'), danger: true }))) return;
		try {
			await api.del(`/sources/${s.id}`);
			await load();
			void recheckSettingsStatus('source');
		} catch (err) {
			toasts.push(t('settings.sources.removeFailed'), 'fail', { body: message(err) });
		}
	}

	function until(h: SourceHealth): string {
		const when = pausedUntil(h);
		return when ? ` ${when}.` : '';
	}
</script>

<svelte:head><title>{t('settings.sources.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.sources.title')}>
	{#snippet actions()}
		{#if !editing && sources.length}<Button icon="plus" variant="primary" type="button" onclick={startAdd}>{t('settings.sources.add')}</Button>{/if}
	{/snippet}
</PageHeader>

<p class="small muted lead">
	{t('settings.sources.lead')}
	{t('settings.sources.builtIn', { sites: ltr(builtInSites.map((x) => x.name).join(', ')), site: t('settings.sources.site'), kind: t('settings.sources.feedKind') })}
</p>

{#if !editing}<FromPlugin kind="source" taken={sources.map((x) => x.url)} onadded={load} />{/if}

{#if editing}
	<form class="sc-form" use:editForm={isNew ? null : editing.id} onsubmit={save} aria-label={isNew ? t('settings.sources.add') : t('settings.sources.change', { name: editing.name })}>
		<SettingsCard title={isNew ? t('settings.sources.add') : t('settings.sources.change', { name: editing.name })}>
			<Field id="so-name" label={t('settings.sources.name')} style="margin-top: var(--s-4)">
				<input id="so-name" class="input" bind:value={form.name} maxlength="80" required />
			</Field>
			<div class="two">
				<Field id="so-format" label={t('settings.sources.feedKind')}>
					<select id="so-format" class="input" value={form.format} disabled={!isNew && form.format === 'site'} onchange={(e) => setFormat(e.currentTarget.value)}>
						<option value="torznab">{t('settings.sources.torznab')}</option>
						<option value="newznab">{t('settings.sources.newznab')}</option>
						<option value="rss">{t('settings.sources.rss')}</option>
						{#if isNew || form.format === 'site'}<option value="site">{t('settings.sources.site')}</option>{/if}
					</select>
				</Field>
				{#if form.format === 'site'}
					<Field id="so-site" label={t('settings.sources.siteLabel')}>
						<select id="so-site" class="input" value={form.site} disabled={!isNew} onchange={(e) => pickSite(form, e.currentTarget.value)}>
							{#each builtInSites as s (s.id)}<option value={s.id}>{s.name}</option>{/each}
						</select>
					</Field>
				{:else}
					<Field id="so-proto" label={t('settings.sources.downloads')}>
						<select id="so-proto" class="input" bind:value={form.protocol}>
							<option value="torrent">{t('settings.sources.torrents')}</option>
							<option value="usenet">{t('settings.sources.usenet')}</option>
						</select>
					</Field>
				{/if}
			</div>
			<Field id="so-url" label={t('settings.sources.address')}>
				<input id="so-url" class="input mono" bind:value={form.url} autocomplete="off" spellcheck="false" required placeholder={form.format === 'rss' ? 'https://example.org/rss?passkey=…' : 'https://example.org/api'} />
				<span class="hint">
					{#if form.format === 'rss'}{t('settings.sources.addressRss')}{:else if form.format === 'site'}{t('settings.sources.addressSite')}{:else}{t('settings.sources.addressFeed')}{/if}
				</span>
			</Field>
			{#if form.format === 'site'}
				{#if siteNeedsUser && isNew}
					<Field id="so-login" label={t('settings.sources.siteLoginLabel')}>
						<select id="so-login" class="input" bind:value={form.site_login} data-testid="so-login">
							<option value="passkey">{t('settings.sources.siteLoginPasskey')}</option>
							<option value="cookie">{t('settings.sources.siteLoginCookie')}</option>
						</select>
					</Field>
				{/if}
				{#if siteNeedsUser && form.site_login === 'cookie'}
					<Field id="so-key" label={t('settings.sources.siteCookieLabel')} hint={t('settings.sources.siteCookieHint')}>
						<input id="so-key" class="input mono" type="password" bind:value={form.api_key} autocomplete="new-password" spellcheck="false" />
					</Field>
					<Field id="so-agent" label={t('settings.sources.siteAgentLabel')} hint={t('settings.sources.siteAgentHint')}>
						<input id="so-agent" class="input mono" bind:value={form.site_agent} maxlength="500" autocomplete="off" spellcheck="false" />
					</Field>
				{:else if siteNeedsUser}
					<Field id="so-user" label={t('settings.sources.siteUserLabel')} hint={t('settings.sources.siteUserHint')}>
						<input id="so-user" class="input mono" bind:value={form.site_user} inputmode="numeric" pattern="[0-9]*" autocomplete="off" spellcheck="false" />
					</Field>
				{/if}
				{#if siteNeedsKey && !(siteNeedsUser && form.site_login === 'cookie')}
					<Field id="so-key" label={t('settings.sources.siteKeyLabel')} hint={t('settings.sources.siteKeyHint')}>
						<input id="so-key" class="input mono" type="password" bind:value={form.api_key} autocomplete="new-password" spellcheck="false" />
					</Field>
				{/if}
			{:else if form.format !== 'rss'}
				<Field id="so-key" label={t('settings.sources.apiKeyLabel')} hint={t('settings.sources.apiKeyHint')}>
					<input id="so-key" class="input mono" type="password" bind:value={form.api_key} autocomplete="new-password" spellcheck="false" />
				</Field>
			{/if}
			<fieldset class="group">
				<legend class="label">{t('settings.sources.useFor')}</legend>
				<label class="checkline"><input type="checkbox" bind:checked={form.feed} /><span><b>{t('settings.sources.newest')}</b></span></label>
				{#if form.format !== 'rss'}
					<label class="checkline"><input type="checkbox" bind:checked={form.auto_search} /><span><b>{t('settings.sources.auto')}</b></span></label>
					<label class="checkline"><input type="checkbox" bind:checked={form.hand_search} /><span><b>{t('settings.sources.hand')}</b></span></label>
				{/if}
			</fieldset>
			<fieldset class="group">
				<legend class="label">{t('settings.sources.media')}</legend>
				{#each [['movie', t('settings.sources.movies')], ['tv', t('settings.sources.tvShows')], ['music', t('settings.sources.music')]] as [k, label] (k)}
					<label class="checkline"><input type="checkbox" checked={form.kinds.includes(k)} onchange={(e) => toggleKind(k, e.currentTarget.checked)} /><span>{label}</span></label>
				{/each}
			</fieldset>
			<div class="two">
				<Field id="so-prio" label={t('settings.sources.priority')} hint={t('settings.sources.priorityHint')}>
					<input id="so-prio" class="input" type="number" min="1" max="50" bind:value={form.priority} />
				</Field>
				{#if form.protocol === 'torrent'}
					<Field id="so-seed" label={t('settings.sources.seeders')}>
						<input id="so-seed" class="input" type="number" min="0" bind:value={form.min_seeders} />
					</Field>
				{/if}
			</div>
			<Field id="so-tags" label={t('settings.sources.tags')} hint={t('settings.sources.tagsHint')}>
				<TagInput id="so-tags" bind:value={form.tags} describedby="so-tags-hint" testid="so-tags" />
			</Field>
			{#if downloaders}
				<Field id="so-send" label={t('settings.sources.sendTo')} hint={t('settings.sources.sendToHint')}>
					<select id="so-send" class="input" bind:value={form.downloader_id} aria-describedby="so-send-hint" data-testid="so-send">
						<option value={0}>{t('settings.sources.sendToAny')}</option>
						{#each sendTo.choices as d (d.id)}<option value={d.id}>{d.name}</option>{/each}
					</select>
				</Field>
				{#if sendGone}<p class="notice warn" role="status" data-testid="so-send-gone">{t('settings.sources.sendToGone')}</p>{/if}
			{/if}
			<Field id="so-multi" label={t('settings.sources.multiLanguages')} hint={t('settings.sources.multiLanguagesHint')}>
				<input id="so-multi" class="input mono" bind:value={form.multi_languages} placeholder="fr, en" aria-describedby="so-multi-hint" spellcheck="false" data-testid="so-multi" />
			</Field>
			<Field id="so-nolang" label={t('settings.sources.noLanguage')} hint={t('settings.sources.noLanguageHint')}>
				<input id="so-nolang" class="input mono" bind:value={form.no_language_languages} placeholder="uk" aria-describedby="so-nolang-hint" spellcheck="false" data-testid="so-nolang" />
			</Field>
			<details class="more">
				<summary>{t('settings.sources.limits')}</summary>
				<div class="two">
					<Field id="so-cap" label={t('settings.sources.cap')} hint={t('settings.sources.capHint')}>
						<input id="so-cap" class="input" type="number" min="0" bind:value={form.searches_per_window} />
					</Field>
					<Field id="so-win" label={t('settings.sources.window')}>
						<input id="so-win" class="input" type="number" min="0" step="0.5" bind:value={form.window_hours} />
					</Field>
					<Field id="so-gap" label={t('settings.sources.gap')}>
						<input id="so-gap" class="input" type="number" min="0" bind:value={form.gap_ms} />
					</Field>
					<Field id="so-timeout" label={t('settings.sources.timeout')}>
						<input id="so-timeout" class="input" type="number" min="1" max="300" bind:value={form.timeout_s} />
					</Field>
				</div>
			</details>
			<details class="more" open={!!form.proxy || !!form.flaresolverr}>
				<summary>{t('settings.sources.proxy')}</summary>
				<Field id="so-proxy" label={t('settings.sources.proxyAddress')}>
					<input id="so-proxy" class="input mono" bind:value={form.proxy} autocomplete="off" spellcheck="false" placeholder={t('settings.sources.proxyExample')} />
				</Field>
				<Field id="so-proxy-pw" label={t('settings.sources.proxyPass')}>
					<input id="so-proxy-pw" class="input" type="password" bind:value={form.proxy_password} autocomplete="new-password" />
				</Field>
				<label class="checkline">
					<input type="checkbox" bind:checked={form.proxy_allow_local_network} />
					<span><b>{t('settings.sources.proxyLocal')}</b></span>
				</label>
				<Field id="so-flaresolverr" label={t('settings.sources.flaresolverr')} hint={t('settings.sources.flaresolverrHint')}>
					<input id="so-flaresolverr" class="input mono" bind:value={form.flaresolverr} autocomplete="off" spellcheck="false" placeholder="http://flaresolverr:8191" />
				</Field>
			</details>
			<label class="checkline">
				<input type="checkbox" bind:checked={form.allow_local_network} />
				<span><b>{t('settings.sources.local')}</b><br /><span class="hint">{t('settings.sources.localHint')}</span></span>
			</label>
			{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
			<div class="row" style="margin-top: var(--s-5)">
				<Button variant="primary" type="submit" disabled={busy || !form.name.trim() || !form.url.trim()} aria-busy={busy}>{t('settings.sources.save')}</Button>
				<Button variant="secondary" type="button" onclick={() => (editing = null)}>{t('settings.sources.cancel')}</Button>
			</div>
		</SettingsCard>
	</form>
{/if}

{#if !loaded}
	<SkSettings rows={1} />
{:else if error && !sources.length}
	<LoadError hasData={false} message={error} onretry={load} />
{:else if partOff('sources') && sources.length === 0 && !editing}
	<EmptyState icon="source" title={t('settings.sources.off')} text={t('settings.sources.offText')} action={{ label: t('settings.sources.openParts'), href: to('/settings/parts') }} />
{:else if sources.length === 0 && !editing}
	<EmptyState
		icon="source"
		title={t('settings.sources.none')}
		text={t('settings.sources.noneText')}
		action={{ label: t('settings.sources.add'), onclick: startAdd, icon: 'plus' }}
	/>
	<ConnectBridge onchange={load} />
{:else}
	{#if error}<LoadError hasData message={error} onretry={load} />{/if}
	{#if partOff('sources')}<EmptyState compact icon="source" title={t('settings.sources.off')} text={t('settings.sources.offText')} action={{ label: t('settings.sources.openParts'), href: to('/settings/parts') }} />{/if}
	<ul class="fl">
		{#each withoutEditing(sources, editing?.id) as s (s.id)}
			<li class="dl" data-testid="source" data-edit-row={s.id} title={t('settings.sources.rowTitle', { url: s.url, priority: s.priority, uses: uses(s) })}>
				<div class="main">
					<span class="ic"><Icon name="source" size={20} /></span>
					<div class="grow">
						<div class="top"><a class="name" href={to(`/settings/sources/${s.id}`)}>{s.name}</a>{#if results[s.id]}<span data-testid={results[s.id].ok ? 'source-test' : undefined}><TestTag ok={results[s.id].ok} text={results[s.id].text} hover={results[s.id].hover} extra={results[s.id].extra} /></span>{/if}</div>
						<div class="sub">{s.format === 'rss' ? 'RSS' : s.format === 'site' ? (builtInSites.find((x) => x.id === s.site)?.name ?? t('settings.sources.site')) : words(s.format)} · {words(s.protocol)}</div>
						<div class="sr-only">{s.url}{#if s.format === 'rss' && s.api_key}?{s.api_key}{/if} · 
							{t('settings.sources.priorityUses', { priority: s.priority, uses: uses(s) })}
							{#if s.api_key && s.format !== 'rss'} · {t('settings.sources.keyIs', { key: s.api_key })}{/if}
							{#if s.proxy} · {t('settings.sources.through', { proxy: s.proxy })}{/if}
							{#if s.allow_local_network} · {t('settings.sources.mayReach')}{/if}
							{#if s.tags?.length} · <span data-testid="source-tags">{t('settings.sources.tagsList', { tags: s.tags.join(', ') })}</span>{/if}
						</div>
					</div>
					<div class="num">{#if s.summary.calls_24h > 0 && s.summary.avg_ms_24h}<span class="numin" title={t('settings.sources.lastAnswer', { ago: s.summary.last_success ? ago(s.summary.last_success) : t('settings.sources.never'), failed: s.summary.failures_24h, calls: s.summary.calls_24h })}><b>{ms(s.summary.avg_ms_24h)}</b><small>{t('settings.sources.avg')}</small></span>{/if}</div>
					{#if healthTone(s.health) === 'ok'}<span class="st"><span class="sr-only" data-testid="source-state">{healthWord(s.health)}</span></span>{:else}<span class="st {healthTone(s.health)}"><i></i><span data-testid="source-state">{healthWord(s.health)}</span></span>{/if}
					<div class="acts">
						<Button variant="secondary" size="sm" type="button" disabled={testing === s.id} aria-busy={testing === s.id} onclick={() => test(s)}>{t('settings.sources.test')}</Button>
						{#if s.state === 'disabled'}
							<Button variant="secondary" size="sm" type="button" onclick={() => setPaused(s, false)}>{t('settings.sources.resume')}</Button>
						{:else}
							<Button variant="secondary" size="sm" type="button" onclick={() => setPaused(s, true)}>{t('settings.sources.pause')}</Button>
							{#if s.state === 'paused'}<Button variant="secondary" size="sm" type="button" disabled={testing === s.id} aria-busy={testing === s.id} onclick={() => test(s, 'try')} data-testid="source-try">{t('settings.sources.tryNow')}</Button>{/if}
						{/if}
						<Button variant="secondary" size="sm" href={to(`/settings/sources/${s.id}`)}>{t('settings.sources.health')}</Button>
						<Button variant="secondary" size="sm" type="button" data-edit-change onclick={() => startChange(s)}>{t('settings.sources.changeButton')}</Button>
						<Button variant="secondary" size="sm" type="button" onclick={() => remove(s)} aria-label={t('settings.sources.removeName', { name: s.name })}>{t('settings.sources.remove')}</Button>
					</div>
				</div>
				<p class="note" class:bad-text={s.health.state !== 'working' && s.health.state !== 'disabled'} class:sr-only={s.health.state === 'working' || s.health.state === 'disabled' || s.state === 'paused' || (results[s.id] && !results[s.id].ok)} data-testid="source-text">{serverWords(s.health.text)}{until(s.health)}</p>
				{#if s.health.block === 'bot_check' || results[s.id]?.text.includes("site's bot check")}<p class="note"><a href="#connect-bridge" data-testid="source-bridge-link">{t('settings.sources.bridge')}</a></p>{/if}
				{#if results[s.id]}
					{#if !results[s.id].ok}<p class="note bad-text" role="status" data-testid="source-test">{serverWords(results[s.id].text)}<ConnectHelp text={results[s.id].text} /></p>{/if}
				{/if}
			</li>
		{/each}
	</ul>
	{#if !partOff('sources')}
		<ConnectBridge onchange={load} />
		{#if sources.length}<SourceCheck />{/if}
	{/if}
{/if}

<style>
	.testline { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 10px; }
	.lead {
		margin-bottom: var(--s-4);
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
	.fl { list-style: none; margin: 0; padding: 0; border: var(--sheet-edge); border-radius: var(--sheet-r); background: var(--sheet-bg);  box-shadow: var(--sheet-shadow); overflow: hidden; }
	.dl { padding: 14px 16px 14px 18px; }
	.dl + .dl { box-shadow: inset 0 1px 0 var(--line); }
	.dl:hover { background: color-mix(in srgb, var(--text) 3%, transparent); }
	.main { display: grid; grid-template-columns: 24px minmax(0, 1fr) auto auto auto; align-items: center; column-gap: 16px; }
	.dl .ic { width: 24px; height: 24px; background: none; border-radius: 0; color: color-mix(in srgb, var(--text) 55%, transparent); }
	.top { display: flex; align-items: center; gap: 8px; min-width: 0; }
	.name { font: 600 15px/1.3 var(--font-ui); letter-spacing: -0.01em; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.sub { margin-top: 3px; color: var(--text-muted); font: 400 13px/1.3 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.facts { margin-top: 2px; color: var(--text-muted); font: 400 12px/1.35 var(--font-ui); }
	.num { display: flex; justify-content: flex-end; min-width: 72px; }
	.numin { display: flex; align-items: baseline; gap: 4px; white-space: nowrap; font-variant-numeric: tabular-nums; }
	.num b { font: 600 15px/1 var(--font-ui); } .num small { color: var(--text-muted); font: 400 12px/1 var(--font-ui); }
	.st { display: inline-flex; align-items: center; gap: 6px; color: var(--text-muted); font: 500 12px/1 var(--font-ui); white-space: nowrap; }
	.st i { width: 7px; height: 7px; border-radius: 50%; background: var(--text-faint); }
	.st.ok i { background: var(--ok); box-shadow: 0 0 0 3px color-mix(in srgb, var(--ok) 18%, transparent); }
	.st.wait i { background: var(--wait); box-shadow: 0 0 0 3px color-mix(in srgb, var(--wait) 22%, transparent); }
	.st.fail i, .st.bad i { background: var(--fail); box-shadow: 0 0 0 3px color-mix(in srgb, var(--fail) 22%, transparent); }
	/* Always shown: on a narrow sheet the buttons wrap instead of running past its clipped edge. */
	.acts { display: flex; flex-wrap: wrap; justify-content: flex-end; align-items: center; gap: 6px; min-width: 0; }
	@media (hover: hover) and (min-width: 761px) {
		.dl { position: relative; }
	}
	.note { margin: 8px 0 0 40px; font: 400 13px/1.35 var(--font-ui); color: var(--text-muted); }
	.dl .attempt-review { margin: 10px 0 0 40px; }
	@media (max-width: 760px) {
		.main { grid-template-columns: 24px auto minmax(0, 1fr); grid-template-areas: 'ic grow grow' 'ic st num' 'ic acts acts'; row-gap: 6px; }
		.main > .ic { grid-area: ic; align-self: start; } .main > .grow { grid-area: grow; } .main > .st { grid-area: st; justify-self: start; }
		.main > .num { grid-area: num; justify-self: end; min-width: 0; } .main > .acts { grid-area: acts; justify-self: stretch; flex-wrap: wrap; margin-top: 4px; }
		.top { flex-wrap: wrap; row-gap: 4px; } .name { white-space: normal; overflow-wrap: anywhere; }
		.main > .acts > :global(.ui-btn) { flex: 1 1 auto; }
		/* Notes keep the name column indent, so they line up with the name and buttons. */
	}
	.name { color: inherit; text-decoration: none; } .name:hover { text-decoration: underline; }
	.note { margin-top: 6px; }
</style>
