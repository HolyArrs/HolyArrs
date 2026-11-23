<script lang="ts">
	import { editForm, withoutEditing } from '#lib/ui/edit-in-place.ts';
	import { ask } from '#lib/ui/ask.svelte.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import ConnectHelp from '#lib/ui/ConnectHelp.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import TestTag from '#lib/ui/TestTag.svelte';
	import { PlexServerPick, type MediaServerView } from '#lib/mediaservers/index.ts';
	import { onMount } from 'svelte';
	import { api } from '#lib/core/api.ts';
	import { recheckSettingsStatus } from '#lib/alerts/index.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { ago } from '#lib/core/format.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';

	type Kind = MediaServerView['kind'];
	const kindWord: Record<Kind, string> = { plex: 'Plex', jellyfin: 'Jellyfin', emby: 'Emby', subsonic: 'Navidrome / Subsonic', kodi: 'Kodi' };
	const portHint: Record<Kind, string> = { plex: 'http://192.168.1.10:32400', jellyfin: 'http://192.168.1.10:8096', emby: 'http://192.168.1.10:8096', subsonic: 'http://192.168.1.10:4533', kodi: 'http://192.168.1.10:8080' };

	let servers = $state<MediaServerView[]>([]);
	let loaded = $state(false);
	let hasData = $state(false);
	let error = $state('');
	let editing = $state<MediaServerView | null>(null);
	let isNew = $state(false);
	const blank = () => ({
		name: '',
		kind: 'jellyfin' as Kind,
		url: '',
		token: '',
		username: '',
		enabled: true,
		on_import: true,
		on_rename: true,
		on_delete: true,
		path_from: '',
		path_to: '',
		allow_local_network: true,
		web_url: '',
		plain_password: false,
		use_plex_sign_in: false
	});
	let form = $state(blank());
	let busy = $state(false);
	let formError = $state('');
	let testing = $state(0);
	// The last Test of each server, shown as a tag on its card.
	let results = $state<Record<number, { ok: boolean; text: string; extra?: string; hover?: string }>>({});

	async function load() {
		try {
			servers = (await api.get<{ media_servers: MediaServerView[] }>('/media-servers')).media_servers;
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
		if (isOwner) {
			void loadWatch();
			void loadLabels();
			void loadTracks();
		}
	});

	// Default tracks: owner only, shown once a Plex, Jellyfin or Emby server is linked.
	type TrackSettings = { enabled: boolean; audio: string[]; subtitles: string[] };
	type Lang = { code: string; name: string };
	let tracks = $state<TrackSettings | null>(null);
	let trackForm = $state<TrackSettings>({ enabled: false, audio: [], subtitles: [] });
	let trackLangs = $state<Lang[]>([]);
	let trackBusy = $state(false);
	const MAX_TRACK_LANGS = 5;
	const hasTrackServer = $derived(servers.some((m) => m.kind === 'plex' || m.kind === 'jellyfin' || m.kind === 'emby'));
	const tracksChanged = $derived(!!tracks && JSON.stringify(tracks) !== JSON.stringify(trackForm));
	function langWord(code: string): string {
		if (code === 'off') return t('settings.mediaServers.tracksOff');
		const fallback = trackLangs.find((l) => l.code === code)?.name ?? code;
		try {
			return new Intl.DisplayNames([document.documentElement.lang || 'en'], { type: 'language' }).of(code) ?? fallback;
		} catch {
			return fallback;
		}
	}
	async function loadTracks() {
		try {
			const [s, l] = await Promise.all([api.get<TrackSettings>('/settings/default-tracks'), api.get<{ languages: Lang[] }>('/subtitles/languages')]);
			trackLangs = l.languages;
			tracks = s;
			trackForm = { enabled: s.enabled, audio: [...s.audio], subtitles: [...s.subtitles] };
		} catch {
			tracks = null;
		}
	}
	// The next language not in the list yet, so Add never repeats one.
	function addTrackLang(which: 'audio' | 'subtitles') {
		const list = trackForm[which];
		const page = (document.documentElement.lang || 'en').split('-')[0];
		const next = [page, 'en', 'es', 'fr', 'de', 'it', 'pt', 'ja', 'ko', 'zh', ...trackLangs.map((l) => l.code)].find((c) => trackLangs.some((l) => l.code === c) && !list.includes(c));
		if (next) trackForm[which] = [...list, next];
	}
	function moveTrackLang(which: 'audio' | 'subtitles', i: number, by: number) {
		const list = [...trackForm[which]];
		const j = i + by;
		if (j < 0 || j >= list.length) return;
		[list[i], list[j]] = [list[j], list[i]];
		trackForm[which] = list;
	}
	async function saveTracks(e: SubmitEvent) {
		e.preventDefault();
		trackBusy = true;
		try {
			const s = await api.put<TrackSettings>('/settings/default-tracks', trackForm);
			tracks = s;
			trackForm = { enabled: s.enabled, audio: [...s.audio], subtitles: [...s.subtitles] };
			toasts.push(t('settings.mediaServers.tracksSaved'), 'ok', s.enabled ? { body: t('settings.mediaServers.tracksSavedBody') } : undefined);
		} catch (err) {
			toasts.push(t('settings.mediaServers.tracksFailed'), 'fail', { body: message(err) });
		} finally {
			trackBusy = false;
		}
	}

	// Plex labels: owner only, shown once a Plex server is linked.
	type LabelSettings = { enabled: boolean; label: string; askers: boolean };
	let labels = $state<LabelSettings | null>(null);
	let labelForm = $state<LabelSettings>({ enabled: false, label: 'holyarrs', askers: false });
	let labelBusy = $state(false);
	let labelRunBusy = $state(false);
	const hasPlex = $derived(servers.some((m) => m.kind === 'plex'));
	const labelsChanged = $derived(!!labels && (labels.enabled !== labelForm.enabled || labels.label !== labelForm.label.trim() || labels.askers !== labelForm.askers));
	async function loadLabels() {
		try {
			labels = await api.get<LabelSettings>('/settings/plex-labels');
			labelForm = { ...labels };
		} catch {
			labels = null;
		}
	}
	async function saveLabels(e: SubmitEvent) {
		e.preventDefault();
		labelBusy = true;
		try {
			labels = await api.put<LabelSettings>('/settings/plex-labels', { ...labelForm, label: labelForm.label.trim() });
			labelForm = { ...labels };
			toasts.push(t('settings.mediaServers.labelSaved'), 'ok');
		} catch (err) {
			toasts.push(t('settings.mediaServers.labelFailed'), 'fail', { body: message(err) });
		} finally {
			labelBusy = false;
		}
	}
	async function runLabels() {
		labelRunBusy = true;
		try {
			await api.post('/settings/plex-labels/run');
			toasts.push(t('settings.mediaServers.labelExistingStarted'), 'ok');
		} catch (err) {
			toasts.push(t('settings.mediaServers.labelExistingFailed'), 'fail', { body: message(err) });
		} finally {
			labelRunBusy = false;
		}
	}

	// Read watch history: owner only. Turning it off deletes what was read, so it asks first.
	const isOwner = $derived(session.s.role === 'owner');
	let watchOn = $state<boolean | null>(null);
	let watchBusy = $state(false);
	async function loadWatch() {
		try {
			watchOn = (await api.get<{ enabled: boolean }>('/settings/watch-history')).enabled;
		} catch {
			watchOn = null;
		}
	}
	async function setWatch(e: Event) {
		const box = e.currentTarget as HTMLInputElement;
		const want = box.checked;
		if (!want && !(await ask(t('settings.mediaServers.watchConfirmOff'), { yes: t('ui.ask.turnOff'), danger: true }))) {
			box.checked = true;
			return;
		}
		watchBusy = true;
		try {
			await api.put('/settings/watch-history', { enabled: want });
			watchOn = want;
			toasts.push(want ? t('settings.mediaServers.watchTurnedOn') : t('settings.mediaServers.watchTurnedOff'), 'ok');
		} catch (err) {
			box.checked = !want;
			toasts.push(t('settings.mediaServers.watchFailed'), 'fail', { body: message(err) });
		} finally {
			watchBusy = false;
		}
	}

	function startAdd() {
		isNew = true;
		editing = { ...blank(), id: 0 } as MediaServerView;
		form = blank();
		formError = '';
	}

	function startChange(m: MediaServerView) {
		isNew = false;
		editing = m;
		form = { ...m, token: '', username: '', use_plex_sign_in: false }; // Empty keeps the stored token.
		formError = '';
	}

	async function save(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		formError = '';
		try {
			// The Plex sign-in stands in for the token only for a picked Plex server.
			if (isNew) await api.post('/media-servers', { ...form, use_plex_sign_in: form.kind === 'plex' && form.use_plex_sign_in });
			else {
				await api.put(`/media-servers/${editing!.id}`, form);
				const id = editing!.id;
				results = Object.fromEntries(Object.entries(results).filter(([k]) => Number(k) !== id));
			}
			toasts.push(t('settings.mediaServers.saved', { name: form.name }), 'ok');
			editing = null;
			await load();
			void recheckSettingsStatus();
		} catch (err) {
			formError = message(err);
		} finally {
			busy = false;
		}
	}

	async function test(m: MediaServerView) {
		testing = m.id;
		try {
			const r = await api.post<{ found: string }>(`/media-servers/${m.id}/test`);
			// Plex names its libraries instead of a version: the tag counts them.
			const libs = /^Libraries:/i.test(r.found ?? '') ? r.found.slice(10).split(',').filter((x) => x.trim()).length : 0;
			// Jellyfin and Emby answer "<server name> <version>": only the last word is read for the version (a name like "HTPC 2.0" is not).
			results = { ...results, [m.id]: { ok: true, text: libs ? '' : (r.found ?? '').trim().split(/\s+/).pop() ?? '', hover: r.found ?? '', extra: libs ? t(libs === 1 ? 'settings.mediaServers.libraryOne' : 'settings.mediaServers.libraryOther', { n: libs }) : '' } };
		} catch (err) {
			results = { ...results, [m.id]: { ok: false, text: message(err) } };
		} finally {
			testing = 0;
			await load();
			// Its state is read live: one fresh read clears the menu dot at once.
			void recheckSettingsStatus();
		}
	}

	async function remove(m: MediaServerView) {
		if (!(await ask(t('settings.mediaServers.confirmRemove', { name: m.name }), { yes: t('ui.ask.remove'), danger: true }))) return;
		try {
			await api.del(`/media-servers/${m.id}`);
			await load();
			void recheckSettingsStatus();
		} catch (err) {
			toasts.push(t('settings.mediaServers.couldntRemove'), 'fail', { body: message(err) });
		}
	}

	function moments(m: MediaServerView): string {
		const on = [m.on_import && t('settings.mediaServers.imports'), m.on_rename && t('settings.mediaServers.renames'), m.on_delete && t('settings.mediaServers.deletes')].filter(Boolean);
		return on.length ? t('settings.mediaServers.toldAbout', { list: on.join(', ') }) : t('settings.mediaServers.toldNothing');
	}
</script>

<svelte:head><title>{t('settings.mediaServers.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.mediaServers.title')}>
	{#snippet actions()}
		{#if !editing && servers.length}<Button icon="plus" variant="primary" type="button" onclick={startAdd}>{t('settings.mediaServers.add')}</Button>{/if}
	{/snippet}
</PageHeader>

{#if editing}
	<form class="sc-form" use:editForm={isNew ? null : editing.id} onsubmit={save} aria-label={isNew ? t('settings.mediaServers.add') : t('settings.mediaServers.change', { name: editing.name })}>
		<SettingsCard title={isNew ? t('settings.mediaServers.add') : t('settings.mediaServers.change', { name: editing.name })}>
			<Field id="ms-name" label={t('settings.mediaServers.name')} style="margin-top: var(--s-4)">
				<input id="ms-name" class="input" bind:value={form.name} maxlength="80" required />
			</Field>
			{#if isNew}
				<Field id="ms-kind" label={t('settings.mediaServers.kind')}>
					<select id="ms-kind" class="input" bind:value={form.kind}>
						<option value="plex">Plex</option>
						<option value="jellyfin">Jellyfin</option>
						<option value="emby">Emby</option>
						<option value="subsonic">Navidrome / Subsonic</option>
						<option value="kodi">Kodi</option>
					</select>
				</Field>
			{/if}
			{#if isNew && isOwner && form.kind === 'plex'}
				<PlexServerPick bind:url={form.url} bind:name={form.name} bind:usePlex={form.use_plex_sign_in} />
			{/if}
			<Field id="ms-url" label={t('settings.mediaServers.address')}>
				<input id="ms-url" class="input mono" bind:value={form.url} autocomplete="off" spellcheck="false" placeholder={portHint[form.kind]} />
			</Field>
			{#if form.kind === 'subsonic' || form.kind === 'kodi'}
				<Field id="ms-user" label={t('settings.mediaServers.userLabel')}>
					<input id="ms-user" class="input" bind:value={form.username} autocomplete="off" spellcheck="false" />
				</Field>
			{/if}
			{#if !(form.kind === 'plex' && form.use_plex_sign_in)}
			<Field
				id="ms-token"
				label={form.kind === 'plex' ? t('settings.mediaServers.plexTokenLabel') : form.kind === 'subsonic' || form.kind === 'kodi' ? t('settings.mediaServers.passwordLabel') : t('settings.mediaServers.apiKeyLabel')}
			>
				<input id="ms-token" class="input mono" type="password" bind:value={form.token} autocomplete="new-password" />
				<span class="hint">
					{form.kind === 'plex'
						? t('settings.mediaServers.plexTokenHint')
						: form.kind === 'subsonic'
							? t('settings.mediaServers.subsonicHint')
							: form.kind === 'kodi'
								? t('settings.mediaServers.kodiHint')
								: t('settings.mediaServers.apiKeyHint')}
				</span>
			</Field>
			{/if}
			{#if form.kind === 'subsonic'}
				<label class="checkline">
					<input type="checkbox" bind:checked={form.plain_password} />
					<span><b>{t('settings.mediaServers.plain')}</b><br /><span class="hint">{t('settings.mediaServers.plainHint')}</span></span>
				</label>
			{/if}
			{#if form.kind !== 'plex' && form.kind !== 'subsonic' && form.kind !== 'kodi'}
				<Field id="ms-web" label={t('settings.mediaServers.webUrl')}>
					<input id="ms-web" class="input mono" type="url" bind:value={form.web_url} autocomplete="off" spellcheck="false" placeholder="https://jellyfin.example.com" />
					<span class="hint">{t('settings.mediaServers.webUrlHint')}</span>
				</Field>
			{/if}
			<fieldset class="when">
				<legend class="label">{t('settings.mediaServers.after')}</legend>
				<label class="checkline"><input type="checkbox" bind:checked={form.on_import} /><span><b>{t('settings.mediaServers.anImport')}</b></span></label>
				<label class="checkline"><input type="checkbox" bind:checked={form.on_rename} /><span><b>{t('settings.mediaServers.aRename')}</b></span></label>
				<label class="checkline"><input type="checkbox" bind:checked={form.on_delete} /><span><b>{t('settings.mediaServers.aDelete')}</b><br /><span class="hint">{t('settings.mediaServers.aDeleteHint')}</span></span></label>
			</fieldset>
			{#if form.kind !== 'subsonic'}
				<div class="two">
					<Field id="ms-from" label={t('settings.mediaServers.pathFrom')}>
						<input id="ms-from" class="input mono" bind:value={form.path_from} placeholder="/data/movies" autocomplete="off" />
					</Field>
					<Field id="ms-to" label={t('settings.mediaServers.pathTo')}>
						<input id="ms-to" class="input mono" bind:value={form.path_to} placeholder="/movies" autocomplete="off" />
					</Field>
				</div>
				<p class="hint">{t('settings.mediaServers.pathHint')}</p>
			{/if}
			<label class="checkline">
				<input type="checkbox" bind:checked={form.allow_local_network} />
				<span><b>{t('settings.mediaServers.local')}</b></span>
			</label>
			<label class="checkline">
				<input type="checkbox" bind:checked={form.enabled} />
				<span><b>{t('settings.mediaServers.send')}</b></span>
			</label>
			{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
			<div class="row" style="margin-top: var(--s-5)">
				<Button variant="primary" type="submit" disabled={busy || !form.name.trim()} aria-busy={busy}>{t('settings.mediaServers.save')}</Button>
				<Button variant="plain" type="button" onclick={() => (editing = null)}>{t('settings.mediaServers.cancel')}</Button>
			</div>
		</SettingsCard>
	</form>
{/if}

{#if !loaded}
	<SkSettings rows={2} />
{:else if !hasData}
	<LoadError hasData={false} message={error} onretry={load} />
{:else if servers.length === 0 && !editing}
	{#if error}<LoadError hasData={true} message={error} onretry={load} />{/if}
	<EmptyState icon="tv" title={t('settings.mediaServers.none')} text={t('settings.mediaServers.noneText')} action={{ label: t('settings.mediaServers.add'), onclick: startAdd, icon: 'plus' }} />
{:else}
	{#if error}<LoadError hasData={true} message={error} onretry={load} />{/if}
	<ul class="list">
		{#each withoutEditing(servers, editing?.id) as m (m.id)}
			<li class="card" data-testid="media-server" data-edit-row={m.id}>
				<div class="head">
					<h2 class="h3">{m.name}</h2>
					<span class="chip">{kindWord[m.kind]}</span>
					{#if !m.enabled}<span class="chip">{t('settings.mediaServers.off')}</span>{/if}
					{#if results[m.id]}<TestTag ok={results[m.id].ok} text={results[m.id].text} hover={results[m.id].hover} app={kindWord[m.kind]} extra={results[m.id].extra} testid="media-test" />{/if}
				</div>
				<p class="small muted mono addr">{m.url}</p>
				<p class="small muted">{moments(m)}{m.path_from ? ` · ${t('settings.mediaServers.pathMap', { from: m.path_from, to: m.path_to })}` : ''}</p>
				{#if results[m.id] && !results[m.id].ok}
					<p class="small bad-text" data-testid="media-result">{serverWords(results[m.id].text)}<ConnectHelp text={results[m.id].text} /></p>
				{:else if m.last_error}
					<p class="small bad-text" data-testid="media-result">{t('settings.mediaServers.lastFailed', { error: serverWords(m.last_error) })}</p>
				{:else if m.last_ok_at}
					<p class="small muted" data-testid="media-result">{t('settings.mediaServers.lastChecked', { ago: ago(m.last_ok_at) })}</p>
				{/if}
				<div class="row actions">
					<Button type="button" disabled={testing === m.id} aria-busy={testing === m.id} onclick={() => test(m)}>{t('settings.mediaServers.test')}</Button>
					<Button variant="secondary" type="button" data-edit-change onclick={() => startChange(m)}>{t('settings.mediaServers.changeButton')}</Button>
					<Button variant="secondary" type="button" onclick={() => remove(m)} aria-label={t('settings.mediaServers.removeName', { name: m.name })}>{t('settings.mediaServers.remove')}</Button>
				</div>
			</li>
		{/each}
	</ul>
{/if}

{#if loaded && isOwner && watchOn !== null && !editing}
	<section class="card watch" data-testid="watch-history">
		<h2 class="h3">{t('settings.mediaServers.watchTitle')}</h2>
		<label class="checkline">
			<input type="checkbox" checked={watchOn} disabled={watchBusy} aria-busy={watchBusy} onchange={setWatch} data-testid="watch-history-switch" />
			<span><b>{t('settings.mediaServers.watchRead')}</b><br /><span class="hint">{t('settings.mediaServers.watchHint')}</span></span>
		</label>
	</section>
{/if}

{#if loaded && isOwner && hasPlex && labels && !editing}
	<form class="card watch" data-testid="plex-labels" onsubmit={saveLabels} aria-label={t('settings.mediaServers.labelTitle')}>
		<h2 class="h3">{t('settings.mediaServers.labelTitle')}</h2>
		<label class="checkline">
			<input type="checkbox" bind:checked={labelForm.enabled} data-testid="plex-labels-switch" />
			<span><b>{t('settings.mediaServers.labelSwitch')}</b><br /><span class="hint">{t('settings.mediaServers.labelHint')}</span></span>
		</label>
		{#if labelForm.enabled}
			<Field id="ms-label" label={t('settings.mediaServers.labelText')}>
				<input id="ms-label" class="input" bind:value={labelForm.label} maxlength="64" required autocomplete="off" spellcheck="false" />
			</Field>
			<label class="checkline">
				<input type="checkbox" bind:checked={labelForm.askers} />
				<span><b>{t('settings.mediaServers.labelAskers')}</b></span>
			</label>
		{/if}
		<div class="row actions">
			<Button variant="primary" type="submit" disabled={labelBusy || !labelsChanged || (labelForm.enabled && !labelForm.label.trim())} aria-busy={labelBusy}>{t('settings.mediaServers.save')}</Button>
			{#if labels.enabled}
				<Button type="button" disabled={labelRunBusy} aria-busy={labelRunBusy} onclick={runLabels} data-testid="plex-labels-run">{t('settings.mediaServers.labelExisting')}</Button>
			{/if}
		</div>
	</form>
{/if}

{#if loaded && isOwner && hasTrackServer && tracks && !editing}
	<form class="card watch" data-testid="default-tracks" onsubmit={saveTracks} aria-label={t('settings.mediaServers.tracksTitle')}>
		<h2 class="h3">{t('settings.mediaServers.tracksTitle')}</h2>
		<label class="checkline">
			<input type="checkbox" bind:checked={trackForm.enabled} data-testid="default-tracks-switch" />
			<span><b>{t('settings.mediaServers.tracksSwitch')}</b><br /><span class="hint">{t('settings.mediaServers.tracksHint')}</span></span>
		</label>
		{#if trackForm.enabled}
			{#each ['audio', 'subtitles'] as const as which (which)}
				<div class="field tracks">
					<span class="label" id="tracks-{which}">{which === 'audio' ? t('settings.mediaServers.tracksAudio') : t('settings.mediaServers.tracksSubs')}</span>
					<ol class="langs" aria-labelledby="tracks-{which}">
						{#each trackForm[which] as code, i (code)}
							<li>
								<span class="nth">{i + 1}</span>
								<select class="input" aria-label={t('settings.mediaServers.tracksPick', { n: i + 1 })} value={code} onchange={(e) => { const v = (e.currentTarget as HTMLSelectElement).value; trackForm[which] = v === 'off' ? [...trackForm[which].slice(0, i), 'off'] : trackForm[which].map((c, k) => (k === i ? v : c)); }} data-testid="tracks-{which}-{i}">
									{#if which === 'subtitles'}<option value="off">{langWord('off')}</option>{/if}
									{#each trackLangs.filter((l) => l.code === code || !trackForm[which].includes(l.code)) as l (l.code)}<option value={l.code}>{langWord(l.code)}</option>{/each}
								</select>
								<span class="move">
									<IconButton icon="chevronUp" size="sm" tooltip={false} label={t('settings.mediaServers.tracksUp', { name: langWord(code) })} disabled={i === 0} onclick={() => moveTrackLang(which, i, -1)} />
									<IconButton icon="chevronDown" size="sm" tooltip={false} label={t('settings.mediaServers.tracksDown', { name: langWord(code) })} disabled={i === trackForm[which].length - 1 || trackForm[which][i + 1] === 'off' || code === 'off'} onclick={() => moveTrackLang(which, i, 1)} />
									<IconButton icon="x" size="sm" tooltip={false} label={t('settings.mediaServers.tracksRemove', { name: langWord(code) })} onclick={() => (trackForm[which] = trackForm[which].filter((_, k) => k !== i))} />
								</span>
							</li>
						{/each}
					</ol>
					{#if trackForm[which].length < MAX_TRACK_LANGS && !trackForm[which].includes('off')}
						<div><Button variant="secondary" size="sm" icon="plus" type="button" onclick={() => addTrackLang(which)} data-testid="tracks-{which}-add">{t('settings.mediaServers.tracksAdd')}</Button></div>
					{/if}
				</div>
			{/each}
			<p class="hint">{t('settings.mediaServers.tracksJellyHint')}</p>
		{/if}
		<div class="row actions">
			<Button variant="primary" type="submit" disabled={trackBusy || !tracksChanged || (trackForm.enabled && !trackForm.audio.length && !trackForm.subtitles.length)} aria-busy={trackBusy}>{t('settings.mediaServers.save')}</Button>
		</div>
	</form>
{/if}

<style>
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
	.addr {
		overflow-wrap: anywhere;
	}
	.actions {
		margin-top: var(--s-3);
		flex-wrap: wrap;
	}
	.when {
		border: 0;
		padding: 0;
		margin: var(--s-5) 0 0;
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
	.watch {
		margin-top: var(--s-4);
	}
	.tracks {
		margin-top: var(--s-4);
	}
	.langs {
		display: flex;
		flex-direction: column;
		gap: var(--s-2);
		margin: var(--s-2) 0;
		padding: 0;
		list-style: none;
	}
	.langs li {
		display: flex;
		align-items: center;
		gap: var(--s-2);
	}
	.langs select {
		flex: 1;
		min-width: 0;
		max-width: 320px;
	}
	.nth {
		width: 1.5em;
		flex: none;
		text-align: end;
		color: var(--text-muted);
		font: var(--t-footnote);
	}
	.move {
		display: flex;
		gap: 2px;
		flex: none;
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
