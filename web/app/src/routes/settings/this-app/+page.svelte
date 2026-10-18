<script lang="ts">
	import Field from '#lib/ui/Field.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import AdminLinkButton from '#lib/ui/AdminLinkButton.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
	import { api, message, ApiError } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { ago, bytes, duration, words } from '#lib/format.ts';
	import { session } from '#lib/session.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { checkLabel, linkLabel, runTone, runWord, splitStates, stateWord } from '#lib/health.ts';
	import type { Health, HealthChecks, HealthSettings, HealthStates, UpdateInfo } from '#lib/types.ts';
	import { releaseLink, updatePrivacy, updateWords } from '#lib/update.ts';

	let health = $state<Health | null>(null);
	let checks = $state<HealthChecks | null>(null);
	let states = $state<HealthStates | null>(null);
	const split = $derived(splitStates(states?.states));
	let settings = $state<HealthSettings | null>(null);
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
	let storage = $state<{ app: PathInfo[]; library_folders: PathInfo[]; save_path: string } | null>(null);
	let storageNeedsProof = $state(false);
	let storageError = $state('');
	const freeWords = (n: number) => (n < 0 ? 'free space unknown' : `${bytes(n)} free`);
	let error = $state('');
	let running = $state(false);
	let saving = $state(false);
	let settingsError = $state('');

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
			if (!settings && session.can('settings')) settings = await api.get<HealthSettings>('/settings/health');
			await loadStorage();
			update = await api.get<UpdateInfo>('/app/update');
			error = '';
		} catch (e) {
			error = message(e);
		}
	}

	async function runNow() {
		running = true;
		try {
			checks = await api.post<HealthChecks>('/health/checks/run');
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			running = false;
		}
	}

	async function saveSettings(e: SubmitEvent) {
		e.preventDefault();
		if (!settings) return;
		saving = true;
		settingsError = '';
		try {
			settings = await api.put<HealthSettings>('/settings/health', {
				...settings,
				every_minutes: Number(settings.every_minutes),
				flap_guard: Number(settings.flap_guard),
				low_space_mb: Number(settings.low_space_mb)
			});
			update = await api.get<UpdateInfo>('/app/update');
			toasts.push('Health check settings saved.', 'ok');
		} catch (err) {
			settingsError = message(err);
		} finally {
			saving = false;
		}
	}

	onMount(() => {
		void load();
		const t = setInterval(load, 30000);
		const offs = [live.on('health', load)];
		return () => {
			clearInterval(t);
			offs.forEach((off) => off());
		};
	});
</script>

<svelte:head><title>This app · HolyArrs</title></svelte:head>

<PageHeader title="This app" subtitle="Version and health of this HolyArrs.">
	{#snippet actions()}
		<Button type="button" onclick={runNow} disabled={running} aria-busy={running}>Check again</Button>
	{/snippet}
</PageHeader>

{#if error}
	<p class="notice bad" role="alert">{error}</p>
{:else if !health}
	<div class="sk" style="height: 240px"></div>
{:else}
	<section class="card" aria-labelledby="h-about">
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

	{#if update}
		<section class="card" aria-labelledby="h-update" data-testid="update">
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
			{#if update.on && session.can('settings')}
				<div class="row" style="margin-top: var(--s-3)">
					<Button type="button" onclick={checkUpdate} disabled={checking} aria-busy={checking}>Check for updates now</Button>
				</div>
			{/if}
		</section>
	{/if}

	<section class="card" aria-labelledby="h-checks" data-testid="health-checks">
		<div class="card-head">
			<h2 id="h-checks" class="h2">Health checks</h2>
			{#if checks}<span class="chip {checks.worst === 'ok' ? 'ok' : checks.worst === 'warning' ? 'wait' : 'fail'}" data-testid="health-worst">{stateWord(checks.worst)}</span>{/if}
		</div>
		<p class="small muted">Library folders, free space, Sources, Downloaders and backups, checked every few minutes. When something breaks or is fixed, the Alerts that speak for health hear of it.</p>
		{#if !checks || checks.checks.length === 0}
			<p class="small muted" style="margin-top: var(--s-3)">No results yet. Check again to run them now.</p>
		{:else}
			<ul class="rows checks">
				{#each checks.checks as c (c.key)}
					<li data-testid="health-check" data-state={c.state}>
						<span class="chip {c.state === 'ok' ? 'ok' : c.state === 'warning' ? 'wait' : 'fail'}">{stateWord(c.state)}</span>
						<span class="grow small">
							<b>{checkLabel(c.kind)}</b> {c.subject}
							{#if c.message}<br /><span class="muted">{c.message}</span>{/if}
						</span>
						<time class="mono xs muted" datetime={c.since} title="Since">{ago(c.since)}</time>
					</li>
				{/each}
			</ul>
		{/if}
	</section>

	{#if settings}
		<form class="sc-form" onsubmit={saveSettings} aria-label="Health check settings">
			<SettingsCard title="Health check settings">
				<div class="two">
					<Field id="hs-every" label="Check every (minutes)">
						<input id="hs-every" class="input" type="number" min="1" max="1440" bind:value={settings.every_minutes} />
					</Field>
					<Field id="hs-guard" label="Alert after this many runs in a row">
						<input id="hs-guard" class="input" type="number" min="1" max="20" bind:value={settings.flap_guard} />
					</Field>
				</div>
				<p class="hint">A break or a fix must hold for that many runs before an Alert goes out, so a Downloader that blips once sends nothing.</p>
				<Field id="hs-low" label="Warn when free space is under (MB)">
					<input id="hs-low" class="input" type="number" min="0" bind:value={settings.low_space_mb} />
				</Field>
				<label class="checkline" style="margin-top: var(--s-4)"
					><input type="checkbox" bind:checked={settings.update_check} /> Check once a day for a newer HolyArrs</label
				>
				<p class="hint">Off: HolyArrs never asks. On: one small request a day to the release feed below; nothing is ever installed by itself.</p>
				<p class="hint" data-testid="update-privacy">{updatePrivacy(update?.version ?? 'version')}</p>
				<Field id="hs-upd" label="Release feed" hint="Empty: the HolyArrs project's releases.">
					<input id="hs-upd" class="input mono" bind:value={settings.update_check_url} placeholder={settings.update_check_default} autocomplete="off" />
				</Field>
				<label class="checkline"><input type="checkbox" bind:checked={settings.update_check_local} /> The release feed may be on the local network</label>
				<label class="checkline"><input type="checkbox" bind:checked={settings.update_prerelease} /> Tell me about test versions too (such as v1.2.0-rc1)</label>
				{#if settingsError}<p class="form-error" role="alert">{settingsError}</p>{/if}
				<div class="row" style="margin-top: var(--s-4)">
					<Button variant="primary" type="submit" disabled={saving} aria-busy={saving}>Save</Button>
				</div>
			</SettingsCard>
		</form>
	{/if}

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
		<SettingsCard title="Storage" description="Paths as HolyArrs sees them (inside the container in Docker). Everything HolyArrs keeps lives in the config folder.">
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
		HolyArrs is free software under the GPL-3.0. Movie data from TMDB. This product uses the TMDB API but is not endorsed or certified by TMDB.
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
	.two {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--s-3);
		margin-top: var(--s-4);
	}
	.two :global(.field + .field) {
		margin-top: 0;
	}
	@media (max-width: 600px) {
		.two {
			grid-template-columns: 1fr;
		}
		.checks li {
			align-items: stretch;
			flex-direction: column;
			gap: var(--s-2);
		}
		.checks .grow {
			width: 100%;
		}
		.checks li > :global(.chip),
		.checks li > time {
			align-self: flex-start;
		}
	}
	code {
		font-family: var(--font-mono);
		color: var(--text);
		overflow-wrap: anywhere;
	}
</style>
