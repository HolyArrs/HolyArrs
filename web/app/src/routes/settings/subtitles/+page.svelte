<script lang="ts">
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { dateTime } from '#lib/format.ts';
	import Icon from '#lib/Icon.svelte';
	import EmptyState from '#lib/EmptyState.svelte';
	import WishEditor from '#lib/WishEditor.svelte';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { live } from '#lib/live.svelte.ts';
	import {
		accountMissing,
		accountWords,
		kindCovers,
		siteBody,
		siteForm,
		siteState,
		wishProblem,
		type Language,
		type SiteForm,
		type SiteKind,
		type SiteView,
		type SubtitleSettings
	} from '#lib/subtitles.ts';

	let settings = $state<SubtitleSettings | null>(null);
	let langs = $state<Language[]>([]);
	let kinds = $state<SiteKind[]>([]);
	let sites = $state<SiteView[]>([]);
	let loaded = $state(false);
	let error = $state('');
	let setBusy = $state(false);
	let setError = $state('');

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

	onMount(() => {
		Promise.all([
			api.get<SubtitleSettings>('/settings/subtitles').then((s) => (settings = s)),
			api.get<{ languages: Language[] }>('/subtitles/languages').then((r) => (langs = r.languages)),
			api.get<{ kinds: SiteKind[] }>('/subtitle-sites/kinds').then((r) => (kinds = r.kinds)),
			loadSites()
		])
			.catch((e) => (error = message(e)))
			.finally(() => (loaded = true));
		const offs = [
			live.on('event', (d) => {
				if (String((d as { Kind?: string })?.Kind) === 'subtitle_site_paused') void loadSites();
			}),
			live.on('resync', () => void loadSites())
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
			toasts.push('Subtitle settings saved.', 'ok');
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
			formError = `This site needs your account: fill in ${needs.map((n) => accountWords[n].toLowerCase()).join(' and ')}.`;
			return;
		}
		busy = true;
		formError = '';
		try {
			const body = siteBody(form);
			const saved = isNew ? await api.post<SiteView>('/subtitle-sites', body) : await api.put<SiteView>(`/subtitle-sites/${editing!.ID}`, body);
			toasts.push(`Subtitle site ${saved.name} saved.`, 'ok');
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
			toasts.push(paused ? `${s.name} paused: searches skip it.` : `${s.name} is on again.`, 'ok');
			await loadSites();
		} catch (err) {
			toasts.push(message(err), 'fail');
		}
	}

	async function remove(s: SiteView) {
		if (!confirm(`Remove the Subtitle site ${s.name}? Subtitles already saved stay; its account details are forgotten.`)) return;
		try {
			await api.del(`/subtitle-sites/${s.ID}`);
			await loadSites();
		} catch (err) {
			toasts.push(message(err), 'fail');
		}
	}
</script>

<svelte:head><title>Subtitles · HolyArrs</title></svelte:head>

<header class="page-head">
	<div>
		<h1>Subtitles</h1>
		<p>Which subtitles every movie and show should have, how good a match must be, and where HolyArrs looks.</p>
	</div>
</header>

{#if !loaded}
	<div class="sk" style="height: 200px"></div>
{:else if error && !settings}
	<p class="notice bad" role="alert">{error}</p>
{:else if settings}
	<form class="card" onsubmit={saveSettings} aria-labelledby="h-langs">
		<h2 id="h-langs" class="h2">Default languages</h2>
		<p class="small muted">
			Every movie and show without languages of its own gets these. Change a title's own languages on its page. With no languages, nothing is looked for.
		</p>
		<WishEditor bind:wants={settings.languages} {langs} id="def" />
		<p class="xs muted hint-block">
			<b>Hearing-impaired:</b> Normal also counts a hearing-impaired subtitle as there; Hearing-impaired only waits for one with sound descriptions; Either takes any.
			<b>Forced only</b> means only the lines for foreign speech or signs.
		</p>

		<h2 id="h-pass" class="h2 sec">Pass marks</h2>
		<div class="two">
			<div class="field">
				<label for="pm-ep">Episodes</label>
				<input id="pm-ep" class="input narrow" type="number" min="0" max="100" bind:value={settings.pass_mark_episode} />
			</div>
			<div class="field">
				<label for="pm-mov">Movies</label>
				<input id="pm-mov" class="input narrow" type="number" min="0" max="100" bind:value={settings.pass_mark_movie} />
			</div>
		</div>
		<details class="more" data-testid="pass-mark-help">
			<summary>What the Pass mark means</summary>
			<p class="small">
				Every subtitle a site offers gets a match score from 0 to 100: points for the right name, year, season and episode, the same release group, source,
				resolution and codec as your file, the hearing-impaired and forced choice you asked for, a trusted uploader and a matching frame rate. A subtitle for
				another title, year or episode scores 0.
			</p>
			<p class="small">
				A search saves a subtitle on its own only when it scores the Pass mark or more. 90 for episodes means the show, year, season and episode must match; 70
				for movies means the name, year and some of the release details must. Lower marks take looser matches (more often out of sync); higher marks wait for a
				closer one. Choosing a subtitle by hand on a movie or show page may take any score.
			</p>
		</details>

		<h2 class="h2 sec">Inside the video</h2>
		<label class="checkline">
			<input type="checkbox" bind:checked={settings.use_built_in} />
			<span><b>Count subtitles inside the video</b><br /><span class="hint">A language already inside the file is not looked for again.</span></span>
		</label>
		<label class="checkline">
			<input type="checkbox" bind:checked={settings.ignore_picture_tracks} disabled={!settings.use_built_in} />
			<span><b>Leave out picture subtitles</b><br /><span class="hint">Picture tracks (from discs) do not count, since many players cannot show them.</span></span>
		</label>
		<div class="field">
			<label for="hi-suffix">Hearing-impaired files are named</label>
			<select id="hi-suffix" class="input narrow" bind:value={settings.hi_suffix}>
				<option value="hi">.hi (Movie.en.hi.srt)</option>
				<option value="cc">.cc (Movie.en.cc.srt)</option>
				<option value="sdh">.sdh (Movie.en.sdh.srt)</option>
			</select>
		</div>

		<h2 class="h2 sec">When to look</h2>
		<p class="small muted">A new file is searched at once. After that:</p>
		<div class="two">
			<div class="field">
				<label for="s-hours">Look for missing subtitles every (hours)</label>
				<input id="s-hours" class="input narrow" type="number" min="1" max="168" bind:value={settings.search_hours} />
			</div>
			<div class="field">
				<label for="s-batch">Files per run, at most</label>
				<input id="s-batch" class="input narrow" type="number" min="1" max="1000" bind:value={settings.batch} />
				<span class="hint">Keeps the Subtitle sites from being asked too much at once.</span>
			</div>
		</div>
		<label class="checkline">
			<input type="checkbox" bind:checked={settings.adaptive} />
			<span><b>Slow down after three weeks</b><br /><span class="hint">A subtitle not found for three weeks is then looked for once a week.</span></span>
		</label>
		<label class="checkline">
			<input type="checkbox" bind:checked={settings.better} />
			<span><b>Look for better subtitles</b><br /><span class="hint">Subtitles saved lately that scored under 99 are searched again; a new one must score higher.</span></span>
		</label>
		{#if settings.better}
			<div class="two">
				<div class="field">
					<label for="b-hours">Every (hours)</label>
					<input id="b-hours" class="input narrow" type="number" min="6" max="168" bind:value={settings.better_hours} />
				</div>
				<div class="field">
					<label for="b-days">For subtitles saved in the last (days)</label>
					<input id="b-days" class="input narrow" type="number" min="0" max="30" bind:value={settings.better_days} />
				</div>
			</div>
		{/if}

		<h2 class="h2 sec">Sync fix</h2>
		<p class="small muted">
			A subtitle out of time is fixed by hand on its movie or episode: shift every line, or change the frame rate. These are where the fix starts. HolyArrs does
			not listen to the sound to line subtitles up.
		</p>
		<div class="three">
			<div class="field">
				<label for="sy-step">Shift buttons move (ms)</label>
				<input id="sy-step" class="input narrow" type="number" min="10" max="60000" bind:value={settings.sync_step_ms} />
			</div>
			<div class="field">
				<label for="sy-from">Frame rate from</label>
				<input id="sy-from" class="input narrow" type="number" min="1" max="120" step="0.001" bind:value={settings.sync_from_fps} />
			</div>
			<div class="field">
				<label for="sy-to">Frame rate to</label>
				<input id="sy-to" class="input narrow" type="number" min="1" max="120" step="0.001" bind:value={settings.sync_to_fps} />
			</div>
		</div>
		{#if setError}<p class="form-error" role="alert">{setError}</p>{/if}
		<div class="row" style="margin-top: var(--s-5)">
			<button class="btn btn-red" type="submit" disabled={setBusy} aria-busy={setBusy}>Save settings</button>
		</div>
	</form>

	<section class="sites-sec" aria-labelledby="h-sites">
		<div class="sites-head">
			<div>
				<h2 id="h-sites" class="h2">Subtitle sites</h2>
				<p class="small muted">Where HolyArrs looks for subtitles, lowest priority number first. A site that fails rests for a while and is tried again later.</p>
			</div>
			{#if !editing}<button class="btn btn-red" type="button" onclick={startAdd}><Icon name="plus" size={18} /> Add a Subtitle site</button>{/if}
		</div>

		{#if editing && form}
			<form class="card" onsubmit={save} aria-labelledby="h-site-form">
				<h3 id="h-site-form" class="h3">{isNew ? 'Add a Subtitle site' : `Change ${editing.name}`}</h3>
				{#if isNew}
					<fieldset class="group">
						<legend class="label">Which site</legend>
						{#each kinds as k (k.kind)}
							<label class="checkline">
								<input type="radio" name="site-kind" checked={form.kind === k.kind} disabled={k.later} onchange={() => pickKind(k)} />
								<span>
									<b>{k.name}</b> ({kindCovers(k)}){#if k.later}{' '}<span class="chip">Later</span>{:else if k.needs_account}{' '}<span class="chip fail">Needs your account</span>{/if}
									{#if k.note}<br /><span class="hint">{k.note}</span>{/if}
								</span>
							</label>
						{/each}
					</fieldset>
				{/if}
				<div class="two">
					<div class="field">
						<label for="st-name">Name</label>
						<input id="st-name" class="input" bind:value={form.name} maxlength="100" required />
					</div>
					<div class="field">
						<label for="st-prio">Priority</label>
						<input id="st-prio" class="input narrow" type="number" min="1" max="100" bind:value={form.priority} />
						<span class="hint">1 to 100; lower is asked first.</span>
					</div>
				</div>
				<div class="field" style="margin-top: var(--s-4)">
					<label for="st-url">Address</label>
					<input id="st-url" class="input mono" bind:value={form.url} autocomplete="off" spellcheck="false" required />
				</div>
				{#if formKind?.needs_account}
					<fieldset class="group account" data-testid="site-account">
						<legend class="label">Your account</legend>
						<p class="small muted">
							This site needs your own account with it. The details are kept encrypted and never shown again.{#if !isNew && editing.has_account}
								Leave them empty to keep the stored ones.{/if}
						</p>
						{#each formKind.account ?? [] as a (a)}
							<div class="field">
								<label for="st-acct-{a}">{accountWords[a]}</label>
								{#if a === 'username'}
									<input id="st-acct-{a}" class="input" bind:value={form.username} autocomplete="off" />
								{:else if a === 'password'}
									<input id="st-acct-{a}" class="input" type="password" bind:value={form.password} autocomplete="new-password" />
								{:else}
									<input id="st-acct-{a}" class="input mono" type="password" bind:value={form.api_key} autocomplete="new-password" spellcheck="false" />
								{/if}
							</div>
						{/each}
					</fieldset>
				{/if}
				<details class="more">
					<summary>Limits</summary>
					<div class="two">
						<div class="field">
							<label for="st-gap">Wait between calls (ms)</label>
							<input id="st-gap" class="input narrow" type="number" min="0" max="60000" bind:value={form.limits.gap_ms} />
						</div>
						<div class="field">
							<label for="st-per">Calls per window, at most</label>
							<input id="st-per" class="input narrow" type="number" min="1" max="10000" bind:value={form.limits.per_window} />
						</div>
						<div class="field">
							<label for="st-win">Window (seconds)</label>
							<input id="st-win" class="input narrow" type="number" min="60" max="86400" bind:value={form.limits.window_seconds} />
						</div>
						<div class="field">
							<label for="st-day">Downloads a day (0: no cap)</label>
							<input id="st-day" class="input narrow" type="number" min="0" max="100000" bind:value={form.limits.downloads_per_day} />
						</div>
					</div>
				</details>
				<label class="checkline">
					<input type="checkbox" bind:checked={form.allow_local_network} />
					<span><b>May reach the local network</b><br /><span class="hint">Only for a site on this machine or your home network, such as the lab site. Never cloud addresses.</span></span>
				</label>
				<label class="checkline">
					<input type="checkbox" bind:checked={form.enabled} />
					<span><b>On</b><br /><span class="hint">Off: searches skip it until you turn it on.</span></span>
				</label>
				{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
				<div class="row" style="margin-top: var(--s-5)">
					<button class="btn btn-quiet" type="button" onclick={() => ((editing = null), (form = null))}>Cancel</button>
					<button class="btn btn-red" type="submit" disabled={busy || !form.name.trim() || !form.url.trim()} aria-busy={busy}>Save Subtitle site</button>
				</div>
			</form>
		{/if}

		{#if sites.length === 0 && !editing}
			<EmptyState icon="subtitle" title="No Subtitle site yet">
				<p>Add one so HolyArrs can look for the subtitles your movies and shows are missing.</p>
			</EmptyState>
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
								<div class="mono xs muted ellipsis">{s.url}</div>
							</div>
							<span class="chip {st.tone}" data-testid="subtitle-site-state">{st.label}</span>
						</div>
						<p class="small" data-testid="subtitle-site-text">{st.text}</p>
						<p class="xs muted">
							{k?.name ?? s.kind} · {k ? kindCovers(k) : ''} · priority {s.priority}
							{#if k?.needs_account} · account {s.has_account ? 'saved' : 'not set'}{/if}
							{#if s.allow_local_network} · may reach the local network{/if}
							{#if s.limits.downloads_per_day} · {s.downloads_today} of {s.limits.downloads_per_day} downloads today{/if}
						</p>
						{#if results[s.ID]}
							<p class="small" class:bad-text={!results[s.ID].ok} class:ok-text={results[s.ID].ok} role="status" data-testid="subtitle-site-test">{results[s.ID].text}</p>
						{/if}
						<div class="row actions">
							<button class="btn btn-ghost" type="button" disabled={testing === s.ID} aria-busy={testing === s.ID} onclick={() => test(s)} aria-label="Test {s.name}">Test</button>
							{#if s.state === 'disabled'}
								<button class="btn btn-quiet" type="button" onclick={() => setPaused(s, false)} aria-label="Turn {s.name} on">Turn on</button>
							{:else}
								<button class="btn btn-quiet" type="button" onclick={() => setPaused(s, true)} aria-label="Pause {s.name}">Pause</button>
							{/if}
							<button class="btn btn-quiet" type="button" onclick={() => startChange(s)} aria-label="Change {s.name}">Change</button>
							<button class="btn btn-quiet" type="button" onclick={() => remove(s)} aria-label="Remove {s.name}">Remove</button>
						</div>
					</li>
				{/each}
			</ul>
		{/if}
	</section>
{/if}

<style>
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
	.two .field + .field,
	.three .field + .field {
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
		margin-top: var(--s-6, 32px);
	}
	.sites-head {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		gap: var(--s-3);
		margin-bottom: var(--s-3);
	}
	.group {
		border: 0;
		padding: 0;
		margin: var(--s-4) 0 0;
	}
	.account .field {
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
		color: var(--fail);
	}
	.ok-text {
		color: var(--ok-text, inherit);
	}
	@media (max-width: 600px) {
		.two,
		.three {
			grid-template-columns: 1fr;
		}
		.sites-head {
			flex-direction: column;
		}
	}
</style>
