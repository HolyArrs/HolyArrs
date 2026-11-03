<script lang="ts">
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import AdminLinkButton from '#lib/ui/AdminLinkButton.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Field from '#lib/ui/Field.svelte';
	import Table from '#lib/ui/Table.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { api, message, ApiError } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { ago, bytes, dateTime, duration, words } from '#lib/format.ts';
	import { session } from '#lib/session.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { linkLabel, problems, runTone, runWord, splitStates, stateWord } from '#lib/health.ts';
	import type { Health, HealthChecks, HealthStates, UpdateInfo } from '#lib/types.ts';
	import { IMAGE, goBackBackup, imageTag, releaseLink, updateWords, type BackupRow } from '#lib/update.ts';
	import { setAutoplaySetting } from '#lib/autoplay.svelte.ts';

	// Autoplay trailers: one app-wide switch, owner and settings people only.
	let autoplay = $state<boolean | null>(null);
	let autoplayBusy = $state(false);
	// Keep yt-dlp up to date: the weekly update from yt-dlp's own releases.
	let keepYTDLP = $state(true);
	// Show Library art on the sign-in page (off: trending only).
	let signinArt = $state(false);
	// Picture cache size (GB): covers and posters kept on this server.
	let cacheGB = $state(5);
	// History keep times in days (0 keeps forever).
	let keepRoutine = $state(30);
	let keepOther = $state(0);
	const keepChoices = [0, 7, 14, 30, 90, 180, 365];
	const keepWord = (d: number) => (d === 0 ? 'Forever' : d === 365 ? '1 year' : `${d} days`);
	async function loadAutoplay() {
		try {
			const s = await api.get<{ autoplay_trailers: boolean; keep_ytdlp_updated?: boolean; signin_library_art?: boolean; artwork_cache_gb?: number; history_routine_days?: number; history_other_days?: number }>('/settings/ui');
			keepRoutine = s.history_routine_days ?? 30;
			keepOther = s.history_other_days ?? 0;
			cacheGB = s.artwork_cache_gb ?? 5;
			autoplay = s.autoplay_trailers;
			keepYTDLP = s.keep_ytdlp_updated !== false;
			signinArt = s.signin_library_art === true;
		} catch {
			autoplay = null;
		}
	}
	async function saveKeepYTDLP(on: boolean) {
		autoplayBusy = true;
		try {
			keepYTDLP = (await api.put<{ keep_ytdlp_updated: boolean }>('/settings/ui', { keep_ytdlp_updated: on })).keep_ytdlp_updated;
			toasts.push(keepYTDLP ? 'yt-dlp stays up to date' : 'yt-dlp is no longer updated automatically', 'ok');
		} catch (e) {
			toasts.push(message(e), 'fail');
			await loadAutoplay();
		} finally {
			autoplayBusy = false;
		}
	}
	async function saveCacheGB(gb: number) {
		autoplayBusy = true;
		try {
			cacheGB = (await api.put<{ artwork_cache_gb: number }>('/settings/ui', { artwork_cache_gb: gb })).artwork_cache_gb;
			toasts.push(`Pictures may use up to ${cacheGB} GB`, 'ok');
		} catch (e) {
			toasts.push(message(e), 'fail');
			await loadAutoplay();
		} finally {
			autoplayBusy = false;
		}
	}
	async function saveKeep(field: 'history_routine_days' | 'history_other_days', days: number) {
		autoplayBusy = true;
		try {
			const s = await api.put<{ history_routine_days: number; history_other_days: number }>('/settings/ui', { [field]: days });
			keepRoutine = s.history_routine_days;
			keepOther = s.history_other_days;
			toasts.push('History keep time saved', 'ok');
		} catch (e) {
			toasts.push(message(e), 'fail');
			await loadAutoplay();
		} finally {
			autoplayBusy = false;
		}
	}
	async function saveSigninArt(on: boolean) {
		autoplayBusy = true;
		try {
			signinArt = (await api.put<{ signin_library_art: boolean }>('/settings/ui', { signin_library_art: on })).signin_library_art;
			toasts.push(signinArt ? 'The sign-in page shows Library art' : 'The sign-in page shows trending art', 'ok');
		} catch (e) {
			toasts.push(message(e), 'fail');
			await loadAutoplay();
		} finally {
			autoplayBusy = false;
		}
	}
	async function saveAutoplay(on: boolean) {
		autoplayBusy = true;
		try {
			autoplay = (await api.put<{ autoplay_trailers: boolean }>('/settings/ui', { autoplay_trailers: on })).autoplay_trailers;
			setAutoplaySetting(autoplay);
			toasts.push(autoplay ? 'Trailers play by themselves' : 'Trailers no longer play by themselves', 'ok');
		} catch (e) {
			toasts.push(message(e), 'fail');
			await loadAutoplay();
		} finally {
			autoplayBusy = false;
		}
	}

	// Explore row titles: smart titles against plain ones, counted on this server only (owner).
	type TitleStat = { layer: string; shown: number; opens: number; asks: number; rate: string };
	let titleStats = $state<TitleStat[] | null>(null);
	const layerWords: Record<string, string> = { occasion: 'Occasion', time: 'Time of day', genre: 'Genre mix', taste: 'Your taste', reason: 'Row reason', plain: 'Plain title' };
	async function loadTitleStats() {
		try {
			const a = await api.get<{ layers: Omit<TitleStat, 'rate'>[] }>('/explore/title-stats');
			titleStats = a.layers.map((l) => ({ ...l, layer: layerWords[l.layer] ?? l.layer, rate: l.shown ? `${(((l.opens + l.asks) / l.shown) * 100).toFixed(1)}%` : '–' }));
		} catch {
			titleStats = null;
		}
	}

	let health = $state<Health | null>(null);
	let checks = $state<HealthChecks | null>(null);
	let states = $state<HealthStates | null>(null);
	const split = $derived(splitStates(states?.states));
	const bad = $derived(problems(checks?.checks));
	interface PathInfo {
		name: string;
		path: string;
		bytes?: number;
		free_bytes: number;
		kind?: string;
	}
	let update = $state<UpdateInfo | null>(null);
	let checking = $state(false);
	async function checkUpdate() {
		checking = true;
		try {
			update = await api.post<UpdateInfo>('/app/update/check');
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			checking = false;
		}
	}
	// Going back: the pre-update backup taken for the update to this
	// version (people who may change settings; the list is theirs).
	let goBack = $state<BackupRow | null>(null);
	async function loadGoBack() {
		if (!session.can('settings') || !health) return;
		try {
			const r = await api.get<{ backups: BackupRow[] }>('/backups');
			goBack = goBackBackup(r.backups, health.version);
		} catch {
			goBack = null;
		}
	}
	async function copy(text: string) {
		try {
			await navigator.clipboard.writeText(text);
			toasts.push('Copied.', 'ok');
		} catch {
			toasts.push('Copying is not allowed here; select the text instead.', 'fail');
		}
	}
	let storage = $state<{ app: PathInfo[]; library_folders: PathInfo[]; save_path: string } | null>(null);
	let storageNeedsProof = $state(false);
	let storageError = $state('');
	const freeWords = (n: number) => (n < 0 ? 'free space unknown' : `${bytes(n)} free`);
	let error = $state('');

	async function loadStorage() {
		if (!session.can('settings')) return;
		if (storageNeedsProof) return;
		if (session.s.via !== 'api_key' && !session.s.localRecent) {
			storage = null;
			storageError = '';
			storageNeedsProof = true;
			return;
		}
		try {
			storage = await api.get('/app/storage');
			storageError = '';
			storageNeedsProof = false;
		} catch (e) {
			storage = null;
			if (e instanceof ApiError && e.status === 403) {
				storageError = '';
				storageNeedsProof = true;
			} else storageError = message(e);
		}
	}

	async function load() {
		try {
			[health, checks, states] = await Promise.all([
				api.get<Health>('/health'),
				api.get<HealthChecks>('/health/checks'),
				api.get<HealthStates>('/health/states')
			]);
			await loadStorage();
			update = await api.get<UpdateInfo>('/app/update');
			await loadGoBack();
			error = '';
		} catch (e) {
			error = message(e);
		}
	}

	// Trakt ratings, watchers and comments on title pages (owner only):
	// the Client ID of a Trakt app plus a VIP account's access token, both
	// kept encrypted and only ever shown masked.
	let traktIdMasked = $state('');
	let traktVipMasked = $state('');
	let traktId = $state('');
	let traktVip = $state('');
	let traktErr = $state('');
	let traktBusy = $state(false);
	async function loadTrakt() {
		try {
			const [k, t] = await Promise.all([api.get<{ key: string }>('/settings/trakt-key'), api.get<{ token: string }>('/settings/trakt-token')]);
			traktIdMasked = k.key;
			traktVipMasked = t.token;
		} catch {
			/* The card shows Not set. */
		}
	}
	async function saveTraktKeys(e: SubmitEvent) {
		e.preventDefault();
		traktErr = '';
		if (!traktId.trim() && !traktVip.trim()) {
			traktErr = 'Paste the Client ID, the access token, or both.';
			return;
		}
		traktBusy = true;
		try {
			if (traktId.trim()) traktIdMasked = (await api.put<{ key: string }>('/settings/trakt-key', { key: traktId.trim() })).key;
			if (traktVip.trim()) traktVipMasked = (await api.put<{ token: string }>('/settings/trakt-token', { token: traktVip.trim() })).token;
			traktId = '';
			traktVip = '';
			toasts.push('Trakt saved.', 'ok');
		} catch (err) {
			traktErr = err instanceof ApiError && err.status === 403 ? 'Sign in with your password to change Trakt.' : message(err);
		} finally {
			traktBusy = false;
		}
	}

	// URL base: the path HolyArrs lives under behind a reverse proxy. A saved
	// base applies on the next start, unless HOLYARRS_URL_BASE or -url-base is set.
	let urlBase = $state<{ saved: string; in_use: string; restart_needed: boolean } | null>(null);
	let urlBaseTyped = $state('');
	let urlBaseBusy = $state(false);
	let urlBaseErr = $state('');
	let urlBaseGone = $state(false);
	async function loadURLBase() {
		try {
			urlBase = await api.get<{ saved: string; in_use: string; restart_needed: boolean }>('/settings/url-base');
			urlBaseTyped = urlBase.saved;
		} catch (err) {
			// An older server without the setting: no card rather than an error.
			if (err instanceof ApiError && err.status === 404) urlBaseGone = true;
			else urlBaseErr = message(err);
		}
	}
	async function saveURLBase(e: SubmitEvent) {
		e.preventDefault();
		urlBaseBusy = true;
		urlBaseErr = '';
		try {
			urlBase = await api.put<{ saved: string; in_use: string; restart_needed: boolean }>('/settings/url-base', { url_base: urlBaseTyped.trim() });
			urlBaseTyped = urlBase.saved;
			toasts.push(urlBase.restart_needed ? 'URL base saved. Restart HolyArrs to use it.' : 'URL base saved.', 'ok');
		} catch (err) {
			urlBaseErr = message(err);
		} finally {
			urlBaseBusy = false;
		}
	}

	onMount(() => {
		void load();
		if (session.s.role === 'owner') void loadTrakt();
		if (session.can('settings')) void loadAutoplay();
		if (session.can('settings')) void loadURLBase();
		if (session.s.role === 'owner') void loadTitleStats();
		const t = setInterval(load, 30000);
		const offs = [live.on('health', load)];
		return () => {
			clearInterval(t);
			offs.forEach((off) => off());
		};
	});
</script>

<svelte:head><title>This app · HolyArrs</title></svelte:head>

<PageHeader title="This app" />

{#if error && !health}
	<LoadError hasData={false} message={error} onretry={load} />
{:else if !health}
	<SkSettings rows={3} />
{:else}
	{#if error}<LoadError hasData message={error} onretry={load} />{/if}
	<section class="card" id="h-about-card" aria-labelledby="h-about">
		<div class="card-head">
			<h2 id="h-about" class="h2">HolyArrs</h2>
			<span class="chip {health.ok ? 'ok' : 'fail'}">{health.ok ? 'Healthy' : 'Database not answering'}</span>
		</div>
		<dl class="facts">
			<dt>Version</dt>
			<dd>{health.version}</dd>
			<dt>Database schema</dt>
			<dd>{health.schema}</dd>
			<dt>Running for</dt>
			<dd>{duration(health.uptime_s)}</dd>
			<dt>Database writes</dt>
			<dd>{health.writes} (95% wait under {health.write_wait_p95_ms} ms)</dd>
			<dt>URL base</dt>
			<dd data-testid="url-base">{health.url_base || 'None (served at /)'} <span class="xs muted">· set with {'HOLYARRS_URL_BASE'} or -url-base</span></dd>
			<dt>Live update tabs</dt>
			<dd>{health.live_clients} · this tab: {words(live.status.state)}</dd>
		</dl>
	</section>

	{#if autoplay !== null}
		<section class="card" id="autoplay-card" aria-labelledby="h-autoplay" data-testid="autoplay-setting">
			<div class="card-head"><h2 id="h-autoplay" class="h2">Autoplay trailers</h2></div>
			<label class="checkline">
				<input type="checkbox" checked={autoplay} disabled={autoplayBusy} onchange={(e) => saveAutoplay(e.currentTarget.checked)} />
				<span><b>Play trailers by themselves</b><br /><span class="hint">On Explore the featured title and the larger card that opens over a poster play a muted trailer. Never for Kid accounts, with Reduce motion on, or on slow or data-saving connections. Applies to everyone.</span></span>
			</label>
			<label class="checkline" data-testid="ytdlp-update-setting">
				<input type="checkbox" checked={keepYTDLP} disabled={autoplayBusy} onchange={(e) => saveKeepYTDLP(e.currentTarget.checked)} />
				<span><b>Keep yt-dlp up to date</b><br /><span class="hint">Trailers are found with yt-dlp. Once a week HolyArrs fetches its newest official release, checks it and keeps the previous copy if the new one does not work. Its version and the last update show under Health.</span></span>
			</label>
			<label class="checkline" data-testid="signin-art-setting">
				<input type="checkbox" checked={signinArt} disabled={autoplayBusy} onchange={(e) => saveSigninArt(e.currentTarget.checked)} />
				<span><b>Show Library art on the sign-in page</b><br /><span class="hint">The sign-in page shows trending movies and shows. On, it shows wide pictures from your Library first. Anyone who opens the page can see them, and no rating filter applies.</span></span>
			</label>
			<label class="checkline" data-testid="artwork-cache-setting">
				<select aria-label="Picture storage" value={cacheGB} disabled={autoplayBusy} onchange={(e) => saveCacheGB(Number(e.currentTarget.value))}>
					{#each [1, 2, 5, 10, 20] as gb (gb)}<option value={gb}>{gb} GB</option>{/each}
				</select>
				<span><b>Picture storage</b><br /><span class="hint">Posters, covers and photos are kept on this server so pages open with their pictures. Past this size the pictures seen longest ago are removed; they come back when a page needs them.</span></span>
			</label>
		</section>
	{/if}

	{#if autoplay !== null}
		<section class="card" id="history-keep-card" aria-labelledby="h-history-keep" data-testid="history-keep-setting">
			<div class="card-head"><h2 id="h-history-keep" class="h2">History</h2></div>
			<label class="checkline">
				<select aria-label="Keep routine work" value={keepRoutine} disabled={autoplayBusy} onchange={(e) => saveKeep('history_routine_days', Number(e.currentTarget.value))}>
					{#each keepChoices.includes(keepRoutine) ? keepChoices : [...keepChoices, keepRoutine] as d (d)}<option value={d}>{keepWord(d)}</option>{/each}
				</select>
				<span><b>Keep routine work</b><br /><span class="hint">Searches, feed checks, refreshed details and folder scans. Older ones are removed once a day.</span></span>
			</label>
			<label class="checkline">
				<select aria-label="Keep everything else" value={keepOther} disabled={autoplayBusy} onchange={(e) => saveKeep('history_other_days', Number(e.currentTarget.value))}>
					{#each keepChoices.includes(keepOther) ? keepChoices : [...keepChoices, keepOther] as d (d)}<option value={d}>{keepWord(d)}</option>{/each}
				</select>
				<span><b>Keep everything else</b><br /><span class="hint">Downloads, imports, upgrades, problems, sign-ins and settings changes.</span></span>
			</label>
		</section>
	{/if}

	{#if update}
		<section class="card" id="update-card" tabindex="-1" aria-labelledby="h-update" data-testid="update">
			<div class="card-head">
				<h2 id="h-update" class="h2">Updates</h2>
				{#if update.newer}<span class="chip wait">New version</span>{/if}
			</div>
			<p class="small" class:notice={update.newer} data-testid="update-words">{updateWords(update)}</p>
			{#if update.newer && releaseLink(update.link)}
				<p class="small"><a href={releaseLink(update.link)} rel="noopener noreferrer" target="_blank">What is new in {update.latest}</a></p>
			{:else if update.newer}
				<p class="small" data-testid="update-version">Version {update.latest}</p>
			{/if}
			<p class="xs muted">HolyArrs only tells you; it never updates by itself. Pull the new image when it suits you.</p>
			{#if goBack?.from_version}
				{@const tag = imageTag(goBack.from_version)}
				<div class="go-back" data-testid="go-back">
					<h3 class="h3">Go back to {goBack.from_version}</h3>
					<p class="small">HolyArrs kept a copy of its data from just before the update to {goBack.to_version} ({dateTime(goBack.created_at)}). To go back:</p>
					<ol class="small">
						<li>
							Change the image back to
							<span class="cmd"><code data-testid="go-back-image">{IMAGE}:{tag}</code><Button variant="plain" size="sm" type="button" onclick={() => copy(`${IMAGE}:${tag}`)} aria-label="Copy the image name">Copy</Button></span>
							If you pull from another registry, keep its name and change only the tag to <code>{tag}</code>.
						</li>
						<li>Start the container again with that image (with Compose: <code>docker compose up -d</code>).</li>
						<li>{goBack.from_version} sees that its data is newer and shows a page instead. There, type the one-time code from <code>docker logs &lt;container&gt;</code> and press <b>Put back the data from before the update</b>. HolyArrs then starts on that data.</li>
					</ol>
					<p class="xs muted">Changes made since the update are not in that copy. The newer data is not deleted: it moves to the backups folder (before-restore-&lt;time&gt;). Without the page: <code>docker exec &lt;container&gt; /holyarrs restore --pre-update</code>, then <code>docker restart &lt;container&gt;</code>.</p>
				</div>
			{/if}
			{#if update.on && session.can('settings')}
				<div class="row" style="margin-top: var(--s-3)">
					<Button type="button" onclick={checkUpdate} disabled={checking} aria-busy={checking}>Check for updates now</Button>
				</div>
			{/if}
		</section>
	{/if}

	<section class="card" aria-labelledby="h-checks" data-testid="health-summary">
		<div class="card-head">
			<h2 id="h-checks" class="h2">Health</h2>
			{#if checks}<span class="chip {checks.worst === 'ok' ? 'ok' : checks.worst === 'warning' ? 'wait' : 'fail'}" data-testid="health-worst">{stateWord(checks.worst)}</span>{/if}
		</div>
		<p class="small muted">
			{#if !checks || checks.checks.length === 0}No health results yet.{:else if bad.length === 0}All {checks.checks.length} checks are fine.{:else}{bad.length} of {checks.checks.length} checks need a look.{/if}
		</p>
		<div class="row" style="margin-top: var(--s-3)"><Button href={to('/settings/health')} iconAfter="chevronRight">See the health checks</Button></div>
	</section>

	<section class="card" aria-labelledby="h-stages" data-testid="stages">
		<div class="card-head">
			<h2 id="h-stages" class="h2">Stages</h2>
			{#if states?.safe_mode}<span class="chip wait">Safe mode</span>{/if}
		</div>
		<p class="small muted">Background work that can fail on its own. A stage that crashes starts again by itself after a short wait.</p>
		{#if split.stages.length === 0}
			<p class="small muted" style="margin-top: var(--s-3)">No stages reported.</p>
		{:else}
			<ul class="rows checks">
				{#each split.stages as s (s.key)}
					<li data-testid="stage" data-state={s.state}>
						<span class="chip {runTone(s.state)}">{runWord(s)}</span>
						<span class="grow small">
							<b>{s.name}</b>
							{#if s.reason}<br /><span class="muted">{s.reason}</span>{/if}
							{#if s.restarts > 0}<br /><span class="xs muted">Started again {s.restarts} {s.restarts === 1 ? 'time' : 'times'}</span>{/if}
						</span>
					</li>
				{/each}
			</ul>
		{/if}
	</section>

	<div data-testid="links">
		<SettingsCard title="Linked apps">
			{#if split.links.length === 0}
				<p class="small muted">No Sources, Downloaders, subtitle sites or media servers yet.</p>
			{:else}
				<ul class="rows checks">
					{#each split.links as l (l.key)}
						<li data-testid="link" data-state={l.state}>
							<span class="chip {runTone(l.state)}">{runWord(l)}</span>
							<span class="grow small">
								<b>{l.name}</b> <span class="xs muted">{linkLabel(l.kind)}</span>
								{#if l.reason}<br /><span class="muted">{l.reason}</span>{/if}
							</span>
						</li>
					{/each}
				</ul>
			{/if}
		</SettingsCard>
	</div>

	<div data-testid="storage">
		<SettingsCard id="storage-card" title="Storage" description="Paths as HolyArrs sees them (inside the container in Docker). Everything HolyArrs keeps lives in the config folder.">
			{#if storage}
				<ul class="rows checks">
					{#each storage.app as p (p.name)}
						<li data-testid="storage-path">
							<span class="grow small">
								<b>{p.name}</b>{#if p.bytes} <span class="xs muted">· {bytes(p.bytes)}</span>{/if}
								<br /><code class="xs">{p.path}</code>
							</span>
							<span class="xs muted">{freeWords(p.free_bytes)}</span>
						</li>
					{/each}
					{#each storage.library_folders as p (p.path)}
						<li data-testid="storage-media">
							<span class="grow small"><b>{p.name}</b> <span class="xs muted">Library folder</span><br /><code class="xs">{p.path}</code></span>
							<span class="xs muted">{freeWords(p.free_bytes)}</span>
						</li>
					{/each}
					{#if storage.save_path}
						<li>
							<span class="grow small"><b>Downloader save folder</b><br /><code class="xs">{storage.save_path}</code></span>
						</li>
					{/if}
				</ul>
			{:else if storageNeedsProof}
				<p class="small muted" style="margin-top: var(--s-2)">Storage paths are protected. Confirm your HolyArrs password in My account, then return here.</p>
				<AdminLinkButton href={to('/settings/account')} style="margin-top: var(--s-3)">Open My account</AdminLinkButton>
			{:else if storageError}
				<p class="notice bad small" role="alert" style="margin-top: var(--s-3)">{storageError}</p>
			{:else}
				<p class="small muted" style="margin-top: var(--s-2)">Paths are shown to people who may change settings.</p>
			{/if}
		</SettingsCard>
	</div>

	{#if session.can('settings') && !urlBaseGone}
		<SettingsCard id="url-base-card" title="URL base" description="The path HolyArrs lives under behind a reverse proxy, for example /holyarrs. Leave it empty to use the whole address.">
			<form class="sc-urlbase" onsubmit={saveURLBase} data-testid="url-base-form">
				<Field id="url-base" label="URL base" hint={urlBase ? `In use now: ${urlBase.in_use || 'none'}` : undefined}>
					<input id="url-base" class="input mono" bind:value={urlBaseTyped} placeholder="/holyarrs" autocomplete="off" spellcheck="false" maxlength="200" />
				</Field>
				{#if urlBase?.restart_needed}<p class="notice warn" role="status">Restart HolyArrs to use it. A URL base set when HolyArrs starts (HOLYARRS_URL_BASE or -url-base) wins over this one.</p>{/if}
				{#if urlBaseErr}<p class="form-error" role="alert">{urlBaseErr}</p>{/if}
				<div class="row acts"><Button variant="primary" type="submit" disabled={!urlBase || urlBaseTyped.trim() === urlBase.saved} aria-busy={urlBaseBusy}>Save</Button></div>
			</form>
		</SettingsCard>
	{/if}

	{#if session.s.role === 'owner'}
		<section class="card" id="trakt-reviews-card" aria-labelledby="h-trakt-reviews" data-testid="trakt-reviews-setting">
			<div class="card-head">
				<h2 id="h-trakt-reviews" class="h2">Trakt</h2>
				<span class="chip {traktIdMasked && traktVipMasked ? 'ok' : ''}">{traktIdMasked && traktVipMasked ? 'On' : 'Off'}</span>
			</div>
			<p class="small muted">
				With both set, title pages get a Trakt tab under Ratings &amp; reviews: its score, how many people are watching now, and comments. Trakt's API needs a VIP account. Both are stored encrypted and never shown again in full.
			</p>
			<form onsubmit={saveTraktKeys} style="margin-top: var(--s-4)">
				<Field id="trakt-id" label={traktIdMasked ? 'Replace the Client ID' : 'Client ID of your Trakt app'}>
					<input id="trakt-id" class="input mono" type="password" bind:value={traktId} autocomplete="off" spellcheck="false" />
				</Field>
				<Field id="trakt-token" label={traktVipMasked ? 'Replace the VIP access token' : 'VIP access token'}>
					<input id="trakt-token" class="input mono" type="password" bind:value={traktVip} autocomplete="off" spellcheck="false" />
				</Field>
				{#if traktErr}<p class="form-error" role="alert">{traktErr}</p>{/if}
				<Button type="submit" disabled={traktBusy} aria-busy={traktBusy}>Save</Button>
			</form>
		</section>
	{/if}

	{#if session.s.role === 'owner' && titleStats}
		<SettingsCard title="Explore row titles" description="Last 30 days. Rows with a smart title next to rows with their plain title: opens and asks per row shown. Counted on this server only.">
			<Table
				caption="Explore row titles"
				columns={[{ key: 'layer', label: 'Title from' }, { key: 'shown', label: 'Shown', align: 'end' }, { key: 'opens', label: 'Opens', align: 'end' }, { key: 'asks', label: 'Asks', align: 'end' }, { key: 'rate', label: 'Per row', align: 'end' }]}
				rows={titleStats}
				rowKey={(r) => r.layer}
				empty="No Explore rows counted yet."
			/>
		</SettingsCard>
	{/if}

	{#if session.can('users')}
		<SettingsCard title="Logs">
			<p class="small muted" style="margin-top: var(--s-2)">
				HolyArrs keeps its newest log lines in the config folder (up to 10 MB). Keys, tokens, passwords and links with sign-in details are hidden.
			</p>
			<div class="row" style="margin-top: var(--s-3)">
				<Button href={to(`/settings/logs`)}>Read the log</Button>
				<AdminLinkButton variant="plain" href={to(`/api/v1/logs/download`)} download>Download the log</AdminLinkButton>
			</div>
		</SettingsCard>
	{/if}

	<p class="xs muted" style="margin-top: var(--s-5)">
		HolyArrs is free software under the GPL-3.0.
	</p>
{/if}

<style>
	.checks li {
		align-items: flex-start;
	}
	.checks .grow {
		min-width: 0;
		overflow-wrap: anywhere;
	}
	@media (max-width: 600px) {
		.checks li {
			align-items: stretch;
			flex-direction: column;
			gap: var(--s-2);
		}
		.checks .grow {
			width: 100%;
		}
		.checks li > :global(.chip) {
			align-self: flex-start;
		}
	}
	.go-back {
		margin-top: var(--s-4);
		padding-top: var(--s-3);
		border-top: 1px solid var(--line);
	}
	.go-back ol {
		margin: var(--s-2) 0;
		padding-left: 1.4em;
		display: grid;
		gap: var(--s-2);
	}
	.go-back .cmd {
		display: inline-flex;
		align-items: center;
		gap: var(--s-2);
		flex-wrap: wrap;
		max-width: 100%;
	}
	code {
		font-family: var(--font-mono);
		color: var(--text);
		overflow-wrap: anywhere;
	}
</style>
