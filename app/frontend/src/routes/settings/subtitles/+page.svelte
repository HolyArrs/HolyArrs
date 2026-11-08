<script lang="ts">
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
	import { LanguageWants as WishEditor, accountMissing, accountWords, kindCovers, siteBody, siteForm, siteState, wishProblem, type Language, type SiteForm, type SiteKind, type SiteView, type SubtitleSettings } from '#lib/subtitles/index.ts';
	import { onMount } from 'svelte';
	import { session } from '#lib/core/session.svelte.ts';
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
			saved = JSON.stringify(settings);
			toasts.push('Subtitle settings saved', 'ok');
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
			toasts.push(`${saved.name} saved`, 'ok');
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
			toasts.push(paused ? `${s.name} paused` : `${s.name} resumed`, 'ok', paused ? { body: 'Searches skip it' } : undefined);
			await loadSites();
		} catch (err) {
			toasts.push(`Couldn't ${paused ? 'pause' : 'resume'} Subtitle site`, 'fail', { body: message(err) });
		}
	}

	async function remove(s: SiteView) {
		if (!confirm(`Remove ${s.name}? Saved subtitles stay.`)) return;
		try {
			await api.del(`/subtitle-sites/${s.ID}`);
			await loadSites();
		} catch (err) {
			toasts.push("Couldn't remove Subtitle site", 'fail', { body: message(err) });
		}
	}
</script>

<svelte:head><title>Subtitles · HolyArrs</title></svelte:head>

<PageHeader title="Subtitles" />

{#if !loaded}
	<SkSettings rows={2} />
{:else if error && !settings}
	<LoadError hasData={false} message={error} onretry={load} />
{:else if settings}
	{#if error}<LoadError hasData message={error} onretry={load} />{/if}
	<form class="sc-form" id="subtitle-form" onsubmit={saveSettings} aria-label="Default languages">
		<SettingsCard id="lang-card" title="Default languages" description="For titles without their own. Empty means no subtitles.">
			<WishEditor bind:wants={settings.languages} {langs} id="def" />
			{#if settings.languages.length}<p class="xs muted hint-block">
				<b>Hearing-impaired:</b> Normal also counts a hearing-impaired subtitle as there; Hearing-impaired only waits for one with sound descriptions; Either takes any.
				<b>Forced only</b> means only the lines for foreign speech or signs.
			</p>{/if}
	
			{#snippet advanced()}
				<div class="two">
					<Field id="pm-ep" label="Pass mark for episodes" hint="Auto-saved at this score or higher">
						<input id="pm-ep" class="input narrow" type="number" min="0" max="100" bind:value={settings!.pass_mark_episode} />
					</Field>
					<Field id="pm-mov" label="Pass mark for movies" hint="Auto-saved at this score or higher">
						<input id="pm-mov" class="input narrow" type="number" min="0" max="100" bind:value={settings!.pass_mark_movie} />
					</Field>
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
	
			{/snippet}
		</SettingsCard>
		{#if setError}<p class="form-error" role="alert">{setError}</p>{/if}
	</form>
	{#if !session.held('subtitle_sites')}
	<section class="sites-sec" id="sites-card" aria-labelledby="h-sites" tabindex="-1">
		<div class="sites-head">
			<div>
				<h2 id="h-sites" class="h2">Subtitle sites</h2>
				<p class="small muted">Lowest priority number first. Failing sites rest, then retry.</p>
			</div>
			{#if !editing && sites.length}<Button icon="plus" variant="primary" type="button" onclick={startAdd}>Add a Subtitle site</Button>{/if}
		</div>

		{#if editing && form}
			<form class="card" onsubmit={save} aria-labelledby="h-site-form">
				<h3 id="h-site-form" class="h3">{isNew ? 'Add a Subtitle site' : `Change ${editing.name}`}</h3>
				{#if isNew}
					<fieldset class="group">
						<legend class="label">Which site</legend>
						{#each kinds.filter((x) => !x.later) as k (k.kind)}
							<label class="checkline">
								<input type="radio" name="site-kind" checked={form.kind === k.kind} disabled={k.later} onchange={() => pickKind(k)} />
								<span>
									<b>{k.name}</b> ({kindCovers(k)}){#if k.needs_account}{' '}<span class="chip fail">Needs your account</span>{/if}
									{#if k.note}<br /><span class="hint">{k.note}</span>{/if}
								</span>
							</label>
						{/each}
					</fieldset>
				{/if}
				<div class="two">
					<Field id="st-name" label="Name">
						<input id="st-name" class="input" bind:value={form.name} maxlength="100" required />
					</Field>
					<Field id="st-prio" label="Priority" hint="1–100, lowest first">
						<input id="st-prio" class="input narrow" type="number" min="1" max="100" bind:value={form.priority} />
					</Field>
				</div>
				<Field id="st-url" label="Address" style="margin-top: var(--s-4)">
					<input id="st-url" class="input mono" bind:value={form.url} autocomplete="off" spellcheck="false" required />
				</Field>
				{#if formKind?.needs_account}
					<fieldset class="group account" data-testid="site-account">
						<legend class="label">Your account</legend>
						<p class="small muted">
							This site needs your own account with it. The details are kept encrypted and never shown again.{#if !isNew && editing.has_account}
								Leave them empty to keep the stored ones.{/if}
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
					<summary>Limits</summary>
					<div class="two">
						<Field id="st-gap" label="Wait between calls (ms)">
							<input id="st-gap" class="input narrow" type="number" min="0" max="60000" bind:value={form.limits.gap_ms} />
						</Field>
						<Field id="st-per" label="Calls per window, at most">
							<input id="st-per" class="input narrow" type="number" min="1" max="10000" bind:value={form.limits.per_window} />
						</Field>
						<Field id="st-win" label="Window (seconds)">
							<input id="st-win" class="input narrow" type="number" min="60" max="86400" bind:value={form.limits.window_seconds} />
						</Field>
						<Field id="st-day" label="Downloads a day (0 for no limit)">
							<input id="st-day" class="input narrow" type="number" min="0" max="100000" bind:value={form.limits.downloads_per_day} />
						</Field>
					</div>
				</details>
				<label class="checkline">
					<input type="checkbox" bind:checked={form.allow_local_network} />
					<span><b>Reach the local network</b><br /><span class="hint">For a site at home</span></span>
				</label>
				<label class="checkline">
					<input type="checkbox" bind:checked={form.enabled} />
					<span><b>On</b></span>
				</label>
				{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
				<div class="row" style="margin-top: var(--s-5)">
					<Button variant="primary" type="submit" disabled={busy || !form.name.trim() || !form.url.trim()} aria-busy={busy}>Save</Button>
					<Button variant="plain" type="button" onclick={() => ((editing = null), (form = null))}>Cancel</Button>
				</div>
			</form>
		{/if}

		{#if sites.length === 0 && !editing}
			<EmptyState compact icon="subtitle" title="No Subtitle sites" text="Add one to find missing subtitles." action={{ label: 'Add a Subtitle site', icon: 'plus', onclick: startAdd }} />
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
							{k?.name ?? s.kind} · {k ? kindCovers(k) : ''} · priority {s.priority}
							{#if k?.needs_account} · account {s.has_account ? 'saved' : 'not set'}{/if}
							{#if s.allow_local_network} · may reach the local network{/if}
							{#if s.limits.downloads_per_day} · {s.downloads_today} of {s.limits.downloads_per_day} downloads today{/if}
						</p>
						{#if results[s.ID]}
							<div class="testline" data-testid="subtitle-site-test"><TestTag ok={results[s.ID].ok} text={results[s.ID].text} app={k?.name} extra={results[s.ID].ok ? testCount(results[s.ID].text) : ''} />{#if !results[s.ID].ok}<span class="small bad-text">{results[s.ID].text}</span>{/if}</div>
						{/if}
						<div class="row actions">
							<Button type="button" disabled={testing === s.ID} aria-busy={testing === s.ID} onclick={() => test(s)} aria-label="Test {s.name}">Test</Button>
							{#if s.state === 'disabled'}
								<Button variant="plain" type="button" onclick={() => setPaused(s, false)} aria-label="Turn {s.name} on">Turn on</Button>
							{:else}
								<Button variant="plain" type="button" onclick={() => setPaused(s, true)} aria-label="Pause {s.name}">Pause</Button>
							{/if}
							<Button variant="plain" type="button" onclick={() => startChange(s)} aria-label="Change {s.name}">Change</Button>
							<Button variant="plain" type="button" onclick={() => remove(s)} aria-label="Remove {s.name}">Remove</Button>
						</div>
					</li>
				{/each}
			</ul>
		{/if}
	</section>
	{/if}

	<div class="sc-form sub-more">
		<SettingsCard id="when-to-look" title="When to look" description="New files search at once, then on a timer">
			<label class="checkline">
				<input id="better-subs" form="subtitle-form" type="checkbox" bind:checked={settings.better} />
				<span><b>Look for better subtitles</b><br /><span class="hint">Re-searches recent ones under 99</span></span>
			</label>
			{#snippet advanced()}
			<div class="two">
				<Field id="s-hours" label="Look for missing subtitles every (hours)">
					<input id="s-hours" form="subtitle-form" class="input narrow" type="number" min="1" max="168" bind:value={settings!.search_hours} />
				</Field>
				<Field id="s-batch" label="Files per run, at most" hint="Protects site rate limits">
					<input id="s-batch" form="subtitle-form" class="input narrow" type="number" min="1" max="1000" bind:value={settings!.batch} />
				</Field>
			</div>
				<label class="checkline">
					<input id="slow-down" form="subtitle-form" type="checkbox" bind:checked={settings!.adaptive} />
					<span><b>Slow down after three weeks</b><br /><span class="hint">Then weekly</span></span>
				</label>
				{#if settings!.better}
					<div class="two">
						<Field id="b-hours" label="Every (hours)">
							<input id="b-hours" form="subtitle-form" class="input narrow" type="number" min="6" max="168" bind:value={settings!.better_hours} />
						</Field>
						<Field id="b-days" label="For subtitles saved in the last (days)">
							<input id="b-days" form="subtitle-form" class="input narrow" type="number" min="0" max="30" bind:value={settings!.better_days} />
						</Field>
					</div>
				{/if}
			{/snippet}
		</SettingsCard>
	</div>
	<MoreOptions>
		<div class="sc-form">
			<SettingsCard id="inside-video" title="Inside the video">
				<label class="checkline">
					<input id="embedded" form="subtitle-form" type="checkbox" bind:checked={settings.use_built_in} />
					<span><b>Count subtitles inside the video</b><br /><span class="hint">Skips languages already in the file</span></span>
				</label>
				{#snippet advanced()}
					<label class="checkline">
						<input id="no-picture" form="subtitle-form" type="checkbox" bind:checked={settings!.ignore_picture_tracks} disabled={!settings!.use_built_in} />
						<span><b>Leave out picture subtitles</b><br /><span class="hint">Many players can't show disc subtitles</span></span>
					</label>
					<Field id="hi-suffix" label="Hearing-impaired files are named" hint="File ending players read as SDH">
						<select id="hi-suffix" form="subtitle-form" class="input narrow" bind:value={settings!.hi_suffix}>
							<option value="hi">.hi (Movie.en.hi.srt)</option>
							<option value="cc">.cc (Movie.en.cc.srt)</option>
							<option value="sdh">.sdh (Movie.en.sdh.srt)</option>
						</select>
					</Field>
				{/snippet}
			</SettingsCard>
			<SettingsCard id="sync-fix" title="Sync fix" description="Starting values for fixing timing">
				<p class="small muted">
					A subtitle out of time is fixed by hand on its movie or episode: shift every line, or change the frame rate. HolyArrs does
					not listen to the sound to line subtitles up.
				</p>
				{#snippet advanced()}
					<div class="three">
						<Field id="sy-step" label="Shift buttons move (ms)">
							<input id="sy-step" form="subtitle-form" class="input narrow" type="number" min="10" max="60000" bind:value={settings!.sync_step_ms} />
						</Field>
						<Field id="sy-from" label="Frame rate from">
							<input id="sy-from" form="subtitle-form" class="input narrow" type="number" min="1" max="120" step="0.001" bind:value={settings!.sync_from_fps} />
						</Field>
						<Field id="sy-to" label="Frame rate to">
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
	/* No languages yet: the note and Add a language share one row. */
	:global(#lang-card > .body:has(> .wants > li.small)) { flex-direction: row !important; align-items: center; justify-content: space-between; min-height: 64px; }
	:global(#lang-card > .body:has(> .wants > li.small) > *) { margin: 0 !important; box-shadow: none !important; padding-block: 12px !important; }
	:global(#lang-card > .body:has(> .wants > li.small) > .add) { padding-left: 0 !important; }
</style>
