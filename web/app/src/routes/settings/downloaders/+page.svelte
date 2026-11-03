<script lang="ts">
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import TagInput from '#lib/ui/TagInput.svelte';
	import { forgetTags } from '#lib/tags.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import SaveBar from '#lib/ui/SaveBar.svelte';
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { ago } from '#lib/format.ts';
	import Icon from '#lib/Icon.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { live } from '#lib/live.svelte.ts';
	import {
		emptyForm,
		formBody,
		formFrom,
		healthTone,
		kindWords,
		pendingAttempts,
		type DownloaderAttempt,
		type DownloaderForm,
		type DownloaderHealth,
		type DownloaderSettings,
		type DownloaderView
	} from '#lib/downloaders.ts';

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
		if (kind === 'sabnzbd') return 'Must already exist in SABnzbd.';
		if (kind === 'transmission') return 'Stored as a Transmission label; Transmission 4 or newer is required.';
		if (kind === 'deluge') return 'Stored as a Deluge label; the connected daemon needs the Label plugin.';
		if (kind === 'nzbget') return 'Sent as the NZBGet category.';
		return 'Made in qBittorrent if missing.';
	}

	async function save(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		formError = '';
		try {
			const body = formBody(form);
			const saved = isNew ? await api.post<DownloaderView>('/downloaders', body) : await api.put<DownloaderView>(`/downloaders/${editing!.id}`, body);
			toasts.push(`Downloader ${saved.name} saved.`, 'ok');
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
			toasts.push(message(err), 'fail');
		}
	}

	async function remove(d: DownloaderView) {
		if (!confirm(`Remove the Downloader ${d.name}? Downloads already in it stay there; HolyArrs stops following them.`)) return;
		try {
			await api.del(`/downloaders/${d.id}`);
			await load();
		} catch (err) {
			toasts.push(message(err), 'fail');
		}
	}

	async function abandonAttempt(d: DownloaderView, attempt: DownloaderAttempt) {
		if (
			!confirm(
				`Stop HolyArrs from sending Release ${attempt.release_id} to Slot ${attempt.slot_id} and its covered Slots through ${d.name}? This does not cancel anything in ${kindWords[d.kind]}. Inspect that app before choosing a different Release.`
			)
		)
			return;
		abandoning = attempt.id;
		try {
			await api.post(`/downloaders/${d.id}/attempts/${attempt.id}/abandon`);
			toasts.push('The uncertain send was abandoned. Inspect the Downloader before choosing another Release.', 'ok');
			await loadAttempts(d);
		} catch (err) {
			toasts.push(message(err), 'fail');
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
			setError = 'Act on a stalled download no sooner than it counts as stalled.';
			return;
		}
		setBusy = true;
		try {
			settings = await api.put<DownloaderSettings>('/settings/downloaders', body);
			saved = JSON.stringify(settings);
			toasts.push('Downloader settings saved.', 'ok');
		} catch (err) {
			setError = message(err);
		} finally {
			setBusy = false;
		}
	}
</script>

<svelte:head><title>Downloaders · HolyArrs</title></svelte:head>

<PageHeader title="Downloaders">
	{#snippet actions()}
		{#if !editing && list.length}<Button icon="plus" variant="primary" type="button" onclick={startAdd}>Add a Downloader</Button>{/if}
	{/snippet}
</PageHeader>

<p class="small muted lead" id="downloaders-card" tabindex="-1">
	A Release goes to a Downloader of its kind, priority 1 first; Downloaders with the same priority take turns. One that does not answer rests for a while
	(one minute, then twice as long each time, up to 30 minutes) and the next one is used. Every Downloader is checked once a minute.
</p>

{#snippet moreFields()}
<div class="two">
	<Field id="dl-cat" label="Category">
		<input id="dl-cat" class="input" bind:value={form.category} maxlength="64" required />
		<span class="hint">{categoryHint(form.kind)} HolyArrs only follows downloads in it.</span>
	</Field>
	<Field id="dl-prio" label="Priority" hint="1 to 50; 1 is used first.">
		<input id="dl-prio" class="input" type="number" min="1" max="50" bind:value={form.priority} />
	</Field>
</div>
<Field id="dl-tags" label="Tags" hint="A title with one of these tags is sent only to Downloaders that have it. A title with none of your Downloaders' tags goes to the Downloaders without tags.">
	<TagInput id="dl-tags" bind:value={form.tags} describedby="dl-tags-hint" testid="dl-tags" />
</Field>
<details class="more" open={form.path_map.length > 0}>
	<summary>Different folder names</summary>
	<p class="hint">When the Downloader sees the download folder under another name (another container), say both names.</p>
	{#each form.path_map as p, i (i)}
		<div class="two">
			<Field id="pm-theirs-{i}" label="Downloader's path">
				<input id="pm-theirs-{i}" class="input mono" bind:value={p.theirs} placeholder="/data/downloads" />
			</Field>
			<Field id="pm-ours-{i}" label="HolyArrs' path">
				<input id="pm-ours-{i}" class="input mono" bind:value={p.ours} placeholder="/downloads" />
			</Field>
		</div>
	{/each}
	<Button variant="plain" type="button" style="margin-top: var(--s-3)" onclick={() => form.path_map.push({ theirs: '', ours: '' })}>Add a path pair</Button>
</details>
{/snippet}

{#if editing}
	<form class="sc-form" onsubmit={save} aria-label={isNew ? 'Add a Downloader' : `Change ${editing.name}`}>
		<SettingsCard title={isNew ? 'Add a Downloader' : `Change ${editing.name}`}>
			{#if isNew}
				<fieldset class="group">
					<legend class="label">Which app</legend>
					<div class="row kinds">
						<label class="checkline"><input type="radio" name="kind" checked={form.kind === 'qbittorrent'} onchange={() => pickKind('qbittorrent')} /><span><b>qBittorrent</b> (torrents)</span></label>
						<label class="checkline"><input type="radio" name="kind" checked={form.kind === 'transmission'} onchange={() => pickKind('transmission')} /><span><b>Transmission</b> (torrents)</span></label>
						<label class="checkline"><input type="radio" name="kind" checked={form.kind === 'deluge'} onchange={() => pickKind('deluge')} /><span><b>Deluge</b> (torrents)</span></label>
						<label class="checkline"><input type="radio" name="kind" checked={form.kind === 'sabnzbd'} onchange={() => pickKind('sabnzbd')} /><span><b>SABnzbd</b> (Usenet)</span></label>
						<label class="checkline"><input type="radio" name="kind" checked={form.kind === 'nzbget'} onchange={() => pickKind('nzbget')} /><span><b>NZBGet</b> (Usenet)</span></label>
					</div>
				</fieldset>
			{/if}
			<Field id="dl-name" label="Name" style="margin-top: var(--s-4)">
				<input id="dl-name" class="input" bind:value={form.name} maxlength="80" required />
			</Field>
			<Field id="dl-url" label="Address" hint="The address of its web page. For SABnzbd use the IP address, or add HolyArrs' name to its list of allowed host names.">
				<input id="dl-url" class="input mono" bind:value={form.url} autocomplete="off" spellcheck="false" required placeholder={addressPlaceholder(form.kind)} />
			</Field>
			{#if form.kind === 'qbittorrent'}
				<fieldset class="group" data-testid="dl-qbit-way">
					<legend class="label">Sign in with</legend>
					<div class="row kinds">
						<label class="checkline"><input type="radio" name="qbit-way" checked={qbitWay === 'key'} onchange={() => pickWay('key')} /><span>API key</span></label>
						<label class="checkline"><input type="radio" name="qbit-way" checked={qbitWay === 'login'} onchange={() => pickWay('login')} /><span>Username and password</span></label>
					</div>
				</fieldset>
			{/if}
			{#if hasUsername(form.kind)}
				{@const qbit = form.kind === 'qbittorrent'}
				<div class="two">
					<Field id="dl-user" label="Username">
						<input id="dl-user" class="input" bind:value={form.username} autocomplete="off" required={qbit} data-testid="dl-user" />
						<span class="hint">{qbit ? 'The qBittorrent Web UI username.' : `Leave empty when ${kindWords[form.kind]} does not require it from this network.`}</span>
					</Field>
					<Field id="dl-secret" label="Password" hint="Kept encrypted, never shown again. Leave as it is to keep the stored one.">
						<input id="dl-secret" class="input" type="password" bind:value={form.secret} autocomplete="new-password" required={qbit && isNew} data-testid="dl-secret" />
					</Field>
				</div>
			{:else}
				{@const key = form.kind === 'sabnzbd' || form.kind === 'qbittorrent'}
				<Field id="dl-secret" label={key ? 'API key' : 'Password'}>
					<input id="dl-secret" class="input" class:mono={key} type="password" bind:value={form.secret} autocomplete="new-password" spellcheck="false" required={isNew} data-testid="dl-secret" />
					<span class="hint">{form.kind === 'sabnzbd' ? 'In SABnzbd under Config, General.' : form.kind === 'qbittorrent' ? 'qBittorrent 5.2 or newer: Options, Web UI, API key.' : 'The Deluge Web UI password.'} Kept encrypted, never shown again. Moving the address to another host needs it again.</span>
				</Field>
			{/if}
			{#if form.kind === 'deluge'}
				<p class="notice">The Deluge Web UI must already be connected to the intended daemon, and that daemon must have the Label plugin enabled.</p>
			{/if}
			{#if form.kind === 'qbittorrent'}
				<details class="more"><summary>More options</summary>{@render moreFields()}</details>
			{:else}
				{@render moreFields()}
			{/if}
			<label class="checkline">
				<input type="checkbox" bind:checked={form.allow_local_network} />
				<span><b>May reach the local network</b><br /><span class="hint">On: it may run on this machine or your home network (the usual case). Off: only addresses on the internet.</span></span>
			</label>
			<label class="checkline" class:off={writableLocked}>
				<input type="checkbox" bind:checked={form.writable} disabled={writableLocked} data-testid="dl-form-writable" />
				<span><b>HolyArrs can change this Downloader</b><br /><span class="hint">{writableLocked ? 'Locked to read only by this server’s settings (HOLYARRS_READONLY_DOWNLOADERS).' : 'On: pause, resume, remove, moves and priority go to it. Off: HolyArrs still sends new downloads but never changes or removes what is in it.'}</span></span>
			</label>
			{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
			<div class="row" style="margin-top: var(--s-5)">
				<Button variant="primary" type="submit" disabled={busy || !form.name.trim() || !form.url.trim()} aria-busy={busy}>Save</Button>
				<Button variant="plain" type="button" onclick={() => (editing = null)}>Cancel</Button>
				{#if !isNew && editing}{@const cur = editing}<Button variant="plain" type="button" disabled={testing === cur.id} aria-busy={testing === cur.id} onclick={() => test(cur)} data-testid="dl-form-test">Test connection</Button>{/if}
				{#if !isNew && editing && results[editing.id]}<span class="small" class:muted={results[editing.id].ok} class:bad-text={!results[editing.id].ok} role="status">{results[editing.id].text}</span>{/if}
			</div>
		</SettingsCard>
	</form>
{/if}

{#if !loaded}
	<SkSettings rows={1} />
{:else if error && !listOk}
	<LoadError hasData={false} message={error} onretry={load} />
{:else if list.length === 0 && !editing}
	<EmptyState icon="downloader" title="No Downloader yet" text="HolyArrs needs a Downloader to fetch what it chooses. Add a torrent or Usenet app." action={{ label: 'Add a Downloader', onclick: startAdd, icon: 'plus' }} />
{:else}
	{#if error}<LoadError hasData={true} message={error} onretry={load} />{/if}
	<ul class="list">
		{#each list as d (d.id)}
			<li class="card" data-testid="downloader">
				<div class="head">
					<span class="ic"><Icon name="downloader" /></span>
					<div class="grow">
						<h2 class="h3">{d.name}</h2>
						<div class="mono xs muted ellipsis">{d.url}</div>
					</div>
					<span class="chip {healthTone(d.health.state)}" data-testid="downloader-state">{d.health.label}</span>
				</div>
				<p class="small" class:bad-text={d.health.state !== 'working' && d.health.state !== 'paused'} data-testid="downloader-text">{d.health.text}</p>
				<p class="xs muted">
					{kindWords[d.kind]} · {d.protocol === 'usenet' ? 'Usenet' : 'torrents'} · priority {d.priority} · category {d.category}
					{#if d.secret} · {d.kind === 'sabnzbd' || (d.kind === 'qbittorrent' && !d.username) ? 'key' : 'password'} {d.secret}{/if}
					{#if d.allow_local_network} · may reach the local network{/if}
					{#if d.tags?.length} · <span data-testid="downloader-tags">tags {d.tags.join(', ')}</span>{/if}
					· {d.writable && !d.writable_locked ? 'HolyArrs can change it' : d.writable_locked ? 'read only' : 'hands off'}
					· {d.active_downloads} active {d.active_downloads === 1 ? 'download' : 'downloads'}
					{#if d.checked_at} · checked {ago(d.checked_at)}{/if}
				</p>
				{#if results[d.id]}
					<p class="small" class:bad-text={!results[d.id].ok} class:ok-text={results[d.id].ok} role="status" data-testid="downloader-test">{results[d.id].text}</p>
				{/if}
				{#if pendingAttempts(attempts[d.id]).length > 0}
					<section class="notice bad attempt-review" aria-label="Uncertain sends" data-testid="downloader-attempts">
						<h3 class="h3">A send needs review</h3>
						<p class="small">HolyArrs could not confirm whether {kindWords[d.kind]} accepted it. Inspect {kindWords[d.kind]} before choosing a different Release.</p>
						{#each pendingAttempts(attempts[d.id]) as attempt (attempt.id)}
							<div class="attempt-row">
								<p class="small">Release {attempt.release_id} · Slot {attempt.slot_id} · started {ago(attempt.created_at)}</p>
								<Button variant="plain" type="button" disabled={abandoning === attempt.id} aria-busy={abandoning === attempt.id} onclick={() => abandonAttempt(d, attempt)}>Abandon this send</Button>
							</div>
						{/each}
						<p class="xs muted">Abandoning stops HolyArrs from retrying this exact Release and Slot, including covered Slots in the same Version. It does not cancel anything in {kindWords[d.kind]}.</p>
					</section>
				{:else if attemptErrors[d.id]}
					<p class="small bad-text" role="status">Could not check uncertain sends: {attemptErrors[d.id]}</p>
				{/if}
				<div class="row actions">
					<Button type="button" disabled={testing === d.id} aria-busy={testing === d.id} onclick={() => test(d)}>Test</Button>
					{#if d.health.state === 'paused'}
						<Button variant="plain" type="button" onclick={() => setPaused(d, false)}>Resume</Button>
					{:else}
						<Button variant="plain" type="button" onclick={() => setPaused(d, true)}>Pause</Button>
					{/if}
					<Button variant="plain" type="button" onclick={() => startChange(d)}>Change</Button>
					<Button variant="plain" type="button" onclick={() => remove(d)} aria-label="Remove {d.name}">Remove</Button>
				</div>
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
	<SettingsCard id="wrong-card" title="When downloads go wrong" description="What HolyArrs does with a download that stalls, fails or holds something it should not.">
		<span id="h-wrong" class="sr-only">When downloads go wrong</span>
		<label class="checkline">
			<input id="remove-failed" type="checkbox" bind:checked={settings.remove_failed} />
			<span><b>Remove failed downloads from the Downloader</b><br /><span class="hint">With their files. Off: they stay there for you to look at.</span></span>
		</label>
		<label class="checkline">
			<input id="auto-add" type="checkbox" bind:checked={settings.downloads_auto_add} />
			<span><b>Add matched titles to your Library</b><br /><span class="hint">When something already in a Downloader matches a movie or show for sure, add that title without searching. Sure matches already in Suggested are added the next time they are matched.</span></span>
		</label>
		<fieldset class="defaults" id="remove-defaults" tabindex="-1">
			<legend class="small muted">When you remove a download on the Downloads screen, the choices start as:</legend>
		<label class="checkline">
			<input type="checkbox" bind:checked={settings.remove_delete_files} />
			<span><b>Delete its files</b><br /><span class="hint">Off: the Downloader keeps the files.</span></span>
		</label>
		<label class="checkline">
			<input type="checkbox" bind:checked={settings.remove_turn_down} />
			<span><b>Add its Release to Turned down</b><br /><span class="hint">HolyArrs never picks that Release again for the movie.</span></span>
		</label>
		<label class="checkline">
			<input type="checkbox" bind:checked={settings.remove_search_again} />
			<span><b>Search again</b><br /><span class="hint">Only with Turned down: look for the next Release straight away.</span></span>
		</label>
		</fieldset>
		{#snippet advanced()}
			<div class="two">
				<Field id="st-stall" label="Stalled after (hours without progress)" hint="Waiting its turn or paused never counts.">
					<input id="st-stall" class="input" type="number" min="1" max="168" step="0.5" bind:value={settings.stalled_after_hours} />
				</Field>
				<Field id="st-act" label="Give up and try the next Release after (hours)" hint="Counted from the last progress.">
					<input id="st-act" class="input" type="number" min="1" max="720" step="0.5" bind:value={settings.stalled_act_hours} />
				</Field>
			</div>
			<label class="checkline">
				<input id="fail-programs" type="checkbox" bind:checked={settings.fail_dangerous} />
				<span
					><b>Fail downloads that hold programs or fakes</b><br /><span class="hint"
						>Files such as .exe, .bat, .scr or .lnk, also hidden as movie.mkv.exe; a password note or a web shortcut; a torrent with only archives and no video or audio. Checked as soon as the Downloader lists the files.</span
					></span
				>
			</label>
			<label class="checkline">
				<input id="replace-removed" type="checkbox" bind:checked={settings.replace_vanished} />
				<span><b>Replace downloads removed outside HolyArrs</b><br /><span class="hint">Off: the movie waits for your decision instead.</span></span>
			</label>
			<label class="checkline">
				<input id="try-next-dl" type="checkbox" bind:checked={settings.try_next_downloader} />
				<span><b>Try the next Downloader</b><br /><span class="hint">When the first one cannot take a Release, send it to the next one of the same kind.</span></span>
			</label>
			<Field id="st-gap" label="Seconds between sends to one Downloader" hint="A short wait between Releases sent to the same Downloader, so it is not flooded.">
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
	.list .card + .card {
		margin-top: 0;
	}
	.settings {
		margin-top: var(--s-6, 32px);
	}
	.defaults {
		border: 0;
		padding: 0;
		margin: var(--s-3) 0 0;
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
</style>
