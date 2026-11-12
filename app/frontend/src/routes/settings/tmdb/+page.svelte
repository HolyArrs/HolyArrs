<script lang="ts">
	import Words from '#lib/ui/Words.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import SaveBar from '#lib/ui/SaveBar.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import { checkMetadata, clearSourceOff, keyBody, keyForm, metadataHealth, metadataDownText, testWords, type KeyName, type MetadataAnswer, type Mode } from '#lib/metadata/index.ts';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { onMount } from 'svelte';
	import { to } from '#lib/core/paths.ts';

	// Settings > Metadata: where details come from (HolyArrs server, your own keys or off), the own keys, and what is sent.

	let answer = $state<MetadataAnswer | null>(null);
	let loadError = $state('');
	let checking = $state(false);
	let savingMode = $state(false);
	let savingKeys = $state(false);
	let keys = $state<Record<KeyName, string>>(keyForm({}));
	let savedKeys = $state('');
	let tests = $state<Partial<Record<KeyName, { ok: boolean; text: string; busy: boolean }>>>({});
	const dirty = $derived(!!answer && JSON.stringify(keys) !== savedKeys);
	const mode = $derived<Mode>(answer?.mode ?? 'full');

	const modes: { key: Mode; name: () => string; about: () => string }[] = [
		{ key: 'full', name: () => t('settings.tmdb.modeServer'), about: () => t('settings.tmdb.modeServerAbout') },
		{ key: 'own', name: () => t('settings.tmdb.modeOwn'), about: () => t('settings.tmdb.modeOwnAbout') },
		{ key: 'off', name: () => t('settings.tmdb.modeOff'), about: () => t('settings.tmdb.modeOffAbout') }
	];

	function take(a: MetadataAnswer) {
		answer = a;
		keys = keyForm(a.keys);
		savedKeys = JSON.stringify(keys);
	}

	async function load() {
		loadError = '';
		try {
			const [a] = await Promise.all([api.get<MetadataAnswer>('/settings/metadata'), checkMetadata()]);
			take(a);
		} catch (e) {
			loadError = message(e);
		}
	}
	onMount(load);

	async function choose(m: Mode) {
		if (!answer || savingMode || answer.locked || m === answer.mode) return;
		const before = answer.mode;
		answer.mode = m;
		savingMode = true;
		try {
			const a = await api.put<MetadataAnswer>('/settings/metadata', { mode: m });
			// Key edits not saved yet stay in the form.
			const kept = JSON.stringify(keys) !== savedKeys ? $state.snapshot(keys) : null;
			take(a);
			if (kept && m !== 'off') keys = kept;
			tests = {};
			clearSourceOff();
			void checkMetadata(true);
			toasts.push(t('settings.tmdb.modeSaved'), 'ok', { body: modes.find((x) => x.key === m)?.name() });
		} catch (e) {
			if (answer) answer.mode = before;
			toasts.push(t('settings.tmdb.couldntSave'), 'fail', { body: message(e) });
		} finally {
			savingMode = false;
		}
	}

	async function saveKeys(e: SubmitEvent) {
		e.preventDefault();
		savingKeys = true;
		try {
			take(await api.put<MetadataAnswer>('/settings/metadata', { keys: keyBody(keys) }));
			tests = {};
			clearSourceOff();
			toasts.push(t('settings.tmdb.keysSaved'), 'ok');
		} catch (err) {
			toasts.push(t('settings.tmdb.couldntSave'), 'fail', { body: message(err) });
		} finally {
			savingKeys = false;
		}
	}

	async function test(k: KeyName) {
		tests[k] = { ok: false, text: '', busy: true };
		try {
			const r = await api.post<{ ok: boolean; message: string }>('/settings/metadata/test', { key: k, value: keys[k], pin: k === 'tvdb_key' ? keys.tvdb_pin : '' });
			tests[k] = { ok: r.ok, text: testWords(r.message), busy: false };
		} catch (e) {
			tests[k] = { ok: false, text: message(e), busy: false };
		}
	}

	async function recheck() {
		checking = true;
		await checkMetadata(true);
		checking = false;
	}

	// Source names follow the person's language; an unknown id keeps the server's words.
	const sourceWords: Record<string, () => string> = {
		tmdb: () => t('settings.tmdb.src.tmdb'),
		tvmaze: () => t('settings.tmdb.src.tvmaze'),
		tvdb: () => t('settings.tmdb.src.tvdb'),
		music: () => t('settings.tmdb.src.music'),
		trailers: () => t('settings.tmdb.src.trailers'),
		ratings: () => t('settings.tmdb.src.ratings'),
		art: () => t('settings.tmdb.src.art'),
		charts: () => t('settings.tmdb.src.charts'),
		streaming: () => t('settings.tmdb.src.streaming'),
		wiki: () => t('settings.tmdb.src.wiki'),
		anime: () => t('settings.tmdb.src.anime')
	};
	const sourceName = (s: { id: string; name: string }) => sourceWords[s.id]?.() ?? s.name;
	const offRows = $derived((answer?.sources ?? []).filter((s) => s.state !== 'on'));
	const shownKeys = $derived<KeyName[]>(mode === 'own' ? ['tmdb_token', 'tvdb_key', 'mdblist_key', 'fanart_key'] : mode === 'full' ? ['tvdb_key'] : []);
	const keyLabel: Record<KeyName, () => string> = {
		tmdb_token: () => t('settings.tmdb.keyTmdb'),
		tvdb_key: () => t('settings.tmdb.keyTvdb'),
		tvdb_pin: () => t('settings.tmdb.keyTvdbPin'),
		mdblist_key: () => t('settings.tmdb.keyMdblist'),
		fanart_key: () => t('settings.tmdb.keyFanart')
	};
	const keyHint: Record<KeyName, () => string> = {
		tmdb_token: () => t('settings.tmdb.keyTmdbHint'),
		tvdb_key: () => t('settings.tmdb.keyTvdbHint'),
		tvdb_pin: () => t('settings.tmdb.keyTvdbPinHint'),
		mdblist_key: () => t('settings.tmdb.keyMdblistHint'),
		fanart_key: () => t('settings.tmdb.keyFanartHint')
	};
</script>

<svelte:head><title>{t('settings.tmdb.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.tmdb.title')} />

{#if !answer && !loadError}
	<SkSettings rows={3} />
{:else if !answer}
	<LoadError hasData={false} message={loadError} onretry={load} />
{:else}
	<SettingsCard id="metadata-mode" title={t('settings.tmdb.modeTitle')}>
		{#if answer.docker_value}
			<p class="notice bad" role="alert" data-testid="metadata-docker-value"><Icon name="alert" size={18} /><span>{t('settings.tmdb.dockerValue', { value: answer.docker_value })}</span></p>
		{/if}
		<div class="presets" role="radiogroup" aria-label={t('settings.tmdb.modeTitle')} aria-busy={savingMode} aria-describedby={answer.locked ? 'metadata-locked' : undefined}>
			{#each modes as m (m.key)}
				<label class="preset" class:on={mode === m.key} class:locked={answer.locked}>
					<input type="radio" name="metadata-mode" value={m.key} checked={mode === m.key} disabled={savingMode || answer.locked} onchange={() => choose(m.key)} />
					<span class="pn">{m.name()}</span>
					<span class="xs muted">{m.about()}</span>
				</label>
			{/each}
		</div>
		{#if answer.locked}
			<p class="xs muted locked-note" id="metadata-locked"><Icon name="key" size={14} />{t('settings.tmdb.locked')}</p>
		{/if}
		{#if mode === 'own'}
			<p class="notice own-note" role="note"><Icon name="info" size={18} /><span>{t('settings.tmdb.ownNote')}</span></p>
		{/if}
	</SettingsCard>

	{#if shownKeys.length}
	<SettingsCard id="metadata-keys" title={mode === 'own' ? t('settings.tmdb.keysTitle') : t('settings.tmdb.tvdbTitle')} description={mode === 'own' ? t('settings.tmdb.keysDesc') : t('settings.tmdb.tvdbDesc')}>
		<form id="metadata-keys-form" onsubmit={saveKeys} aria-label={t('settings.tmdb.keysTitle')}>
			{#each shownKeys as k (k)}
				{@const fromDocker = k === 'tmdb_token' && answer.tmdb_key_from_docker}
				<div class="srow keyrow">
					<div class="txt"><label class="label" for="mk-{k}">{keyLabel[k]()}</label><span id="mk-{k}-hint" class="hint">{fromDocker ? t('settings.tmdb.tmdbFromDocker') : keyHint[k]()}</span></div>
					<div class="ctl">
						<input id="mk-{k}" class="input mono" type="password" bind:value={keys[k]} autocomplete="new-password" spellcheck="false" disabled={fromDocker} aria-describedby="mk-{k}-hint" />
						<Button type="button" variant="secondary" size="sm" onclick={() => test(k)} disabled={tests[k]?.busy} aria-busy={tests[k]?.busy}>{t('settings.tmdb.testKey')}</Button>
					</div>
					{#if tests[k] && !tests[k].busy}
						<div class="below"><span class="st" class:fail={!tests[k].ok} role="status"><i></i>{tests[k].text}</span></div>
					{/if}
				</div>
				{#if k === 'tvdb_key'}
					<div class="srow keyrow">
						<div class="txt"><label class="label" for="mk-tvdb_pin">{keyLabel.tvdb_pin()}</label><span id="mk-tvdb_pin-hint" class="hint">{keyHint.tvdb_pin()}</span></div>
						<div class="ctl"><input id="mk-tvdb_pin" class="input mono" type="password" bind:value={keys.tvdb_pin} autocomplete="new-password" spellcheck="false" aria-describedby="mk-tvdb_pin-hint" /></div>
					</div>
				{/if}
			{/each}
			<p class="xs muted kept">{mode === 'own' ? t('settings.tmdb.keysKept') : t('settings.tmdb.keysKeptShort')}</p>
		</form>
	</SettingsCard>
	{/if}

	{#if offRows.length && mode !== 'full'}
		<SettingsCard id="metadata-off" title={t('settings.tmdb.offTitle')} description={t('settings.tmdb.offDesc')}>
			<ul class="sent">
				{#each offRows as s (s.id)}
					<li class="offrow"><span>{sourceName(s)}</span><span class="xs muted">{s.state === 'needs_key' ? t('settings.tmdb.needsKey') : t('settings.tmdb.off')}</span></li>
				{/each}
			</ul>
		</SettingsCard>
	{/if}

	{#if mode === 'full'}
		<SettingsCard id="metadata-card" title={t('settings.tmdb.server')}>
			<div class="srow">
				<div class="txt"><span class="label">{metadataHealth.up ? t('settings.tmdb.connected') : t('settings.tmdb.notAnswering')}</span>{#if !metadataHealth.up}<span class="hint" role="status">{metadataDownText()}</span>{/if}</div>
				<div class="ctl state">
					<span class="st" class:fail={!metadataHealth.up}><i></i>{metadataHealth.up ? t('settings.tmdb.answering') : t('settings.tmdb.down')}</span>
					<Button variant="secondary" size="sm" onclick={recheck} disabled={checking} aria-busy={checking}>{t('settings.tmdb.checkAgain')}</Button>
				</div>
			</div>
		</SettingsCard>

		<!-- Keep in step with the metadata privacy doc (tmdb-privacy.test.ts checks both). -->
		<SettingsCard id="metadata-sent" title={t('settings.tmdb.sentTitle')} description={t('settings.tmdb.sentDesc')}>
			<ul class="sent">
				<li><b>{t('settings.tmdb.tokenLabel')}</b> {t('settings.tmdb.tokenText')}</li>
				<li><b>{t('settings.tmdb.lookup')}</b> {t('settings.tmdb.lookupText')}</li>
				<li><b>{t('settings.tmdb.display')}</b> {t('settings.tmdb.displayText')}</li>
				<li><b>{t('settings.tmdb.headers')}</b> <Words key="settings.tmdb.headersText" /></li>
				<li><b>{t('settings.tmdb.country')}</b> <Words key="settings.tmdb.countryText" links={{ a: to('/settings/this-app') + '#count-install' }} /></li>
			</ul>
		</SettingsCard>
	{/if}

	<SettingsCard id="metadata-never" title={t('settings.tmdb.neverTitle')}>
		<ul class="sent">
			<li>{t('settings.tmdb.never1')}</li>
			<li>{t('settings.tmdb.never2')}</li>
			<li>{t('settings.tmdb.never3')}</li>
			<li>{t('settings.tmdb.never4')}</li>
		</ul>
	</SettingsCard>

	{#if answer.hosts?.length}
		<SettingsCard id="metadata-hosts" title={t('settings.tmdb.hostsTitle')} description={t('settings.tmdb.hostsDesc')}>
			<ul class="sent hosts">
				{#each answer.hosts as h (h)}<li><code>{h}</code></li>{/each}
			</ul>
		</SettingsCard>
	{/if}

	<SaveBar {dirty} form="metadata-keys-form" saving={savingKeys} ondiscard={() => (keys = JSON.parse(savedKeys))} />
{/if}

<style>
	.sent { margin: 0; padding-block: 0 !important; list-style: none; box-shadow: none !important; }
	.sent > li { padding: 13px 0; color: var(--text-muted); font: var(--t-subhead); font-size: 14px; }
	.sent > li + li { box-shadow: inset 0 1px 0 var(--line); }
	.sent b { color: var(--text); font-weight: 600; }
	.sent :global(code) { font-size: .9em; overflow-wrap: anywhere; }
	.hosts > li { padding: 9px 0; }
	.offrow { display: flex; align-items: baseline; justify-content: space-between; gap: var(--s-3); color: var(--text) !important; }
	.offrow > .xs { flex: none; }
	.state { display: flex; align-items: center; gap: 14px; }
	.st { display: inline-flex; align-items: center; gap: 6px; color: var(--text-muted); font: 500 13px/1.3 var(--font-ui); min-width: 0; }
	.st i { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--ok); box-shadow: 0 0 0 3px color-mix(in srgb, var(--ok) 18%, transparent); }
	.st.fail i { background: var(--fail); box-shadow: 0 0 0 3px color-mix(in srgb, var(--fail) 22%, transparent); }

	/* Same choice cards as Settings > Backups. */
	.presets { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: var(--s-3); margin-top: var(--s-4); }
	.preset { position: relative; display: flex; flex-direction: column; gap: 4px; padding: var(--s-4); border: 1px solid var(--line-strong); border-radius: var(--r-btn); cursor: pointer; transition: border-color var(--dur) var(--ease-out), background var(--dur-fast) var(--ease-out); }
	.preset:hover { background: var(--surface-3); }
	.preset.on { border-color: var(--ha-red); background: var(--ha-red-soft); }
	.preset.on .muted, .preset:hover .muted { color: var(--text); }
	.preset.locked { cursor: not-allowed; }
	.preset.locked:not(.on) { opacity: .55; }
	.preset.locked:hover:not(.on) { background: none; }
	.preset input { position: absolute; opacity: 0; inset: 0; cursor: inherit; }
	.preset:has(input:focus-visible) { box-shadow: var(--focus-ring); }
	.pn { font-weight: var(--weight-semibold); }
	.locked-note { display: flex; align-items: center; gap: 6px; margin-top: var(--s-3); }
	.own-note, .notice.bad { margin-top: var(--s-4); }

	.kept { padding: var(--s-3) 0 var(--s-2); box-shadow: inset 0 1px 0 var(--line); }
	@media (max-width: 700px) {
		.presets { grid-template-columns: 1fr; }
		/* Phones: the key gets the full width under its label, so no label wraps letter by letter. */
		.srow.keyrow { grid-template-columns: minmax(0, 1fr); row-gap: 8px; }
		.srow.keyrow > .ctl { justify-self: stretch; }
		.srow.keyrow > .ctl > .input { flex: 1; width: auto; }
	}
	@media (prefers-reduced-motion: reduce) {
		.preset { transition: none; }
	}
</style>
