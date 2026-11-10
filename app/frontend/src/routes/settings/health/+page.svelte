<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import SaveBar from '#lib/ui/SaveBar.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { checkName, stateWord } from '#lib/health/index.ts';
	import { refreshSettingsStatus } from '#lib/alerts/index.ts';
	import type { HealthChecks, HealthSettings } from '#lib/health/index.ts';
	import { updatePrivacy } from '#lib/updates/index.ts';
	import { onMount } from 'svelte';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { live } from '#lib/core/live.svelte.ts';
	import { ago } from '#lib/core/format.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import type { UpdateInfo } from '#lib/core/types.ts';

	let checks = $state<HealthChecks | null>(null);
	let settings = $state<HealthSettings | null>(null);
	let saved = $state('');
	const dirty = $derived(!!settings && JSON.stringify(settings) !== saved);
	let update = $state<UpdateInfo | null>(null);
	let error = $state('');
	let running = $state(false);
	let saving = $state(false);
	let settingsError = $state('');

	function keep(s: HealthSettings) {
		settings = s;
		saved = JSON.stringify(s);
	}

	async function load() {
		try {
			checks = await api.get<HealthChecks>('/health/checks');
			if (!settings && session.can('settings')) keep(await api.get<HealthSettings>('/settings/health'));
			update = await api.get<UpdateInfo>('/app/update').catch(() => update);
			error = '';
		} catch (e) {
			error = message(e);
		}
	}

	async function runNow() {
		running = true;
		try {
			checks = await api.post<HealthChecks>('/health/checks/run');
			void refreshSettingsStatus();
		} catch (e) {
			toasts.push(t('settings.health.couldntRun'), 'fail', { body: message(e) });
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
			keep(
				await api.put<HealthSettings>('/settings/health', {
					...settings,
					every_minutes: Number(settings.every_minutes),
					flap_guard: Number(settings.flap_guard),
					low_space_mb: Number(settings.low_space_mb)
				})
			);
			update = await api.get<UpdateInfo>('/app/update');
			toasts.push(t('settings.health.saved'), 'ok');
		} catch (err) {
			settingsError = message(err);
		} finally {
			saving = false;
		}
	}

	onMount(() => {
		void load();
		const timer = setInterval(load, 30000);
		const offs = [live.on('health', load)];
		return () => {
			clearInterval(timer);
			offs.forEach((off) => off());
		};
	});
	/** Keeps the last two parts of a folder path (the full path is in the tooltip); other words stay as they are. */
	function shortPath(t: string): string {
		if (!/[\\/]/.test(t)) return t;
		const parts = t.split(/[\\/]/).filter(Boolean);
		return parts.length > 2 ? '…/' + parts.slice(-2).join('/') : t;
	}
</script>

<svelte:head><title>{t('settings.health.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.health.title')}>
	{#snippet actions()}
		<Button type="button" onclick={runNow} disabled={running} aria-busy={running}>{t('settings.health.checkAgain')}</Button>
	{/snippet}
</PageHeader>

{#if !checks}
	{#if error}
		<LoadError hasData={false} message={error} onretry={load} />
	{:else}
		<SkSettings rows={3} />
	{/if}
{:else}
	{#if error}<LoadError hasData={true} message={error} onretry={load} />{/if}
	<section class="card" id="health-checks-card" aria-labelledby="h-checks" data-testid="health-checks" tabindex="-1">
		<div class="card-head">
			<h2 id="h-checks" class="h2">{t('settings.health.results')} <span class="sub">· {t('settings.health.resultsSub')}</span></h2>
			<span class="chip {checks.worst === 'ok' ? 'ok' : checks.worst === 'warning' ? 'wait' : 'fail'}" data-testid="health-worst">{stateWord(checks.worst)}</span>
		</div>
		{#if checks.sync?.length}
			<ul class="rows checks" data-testid="health-sync">
				{#each checks.sync as c (c.id)}
					<li data-testid="health-sync-row" data-state={c.health}>
						<span class="grow small">
							<b>{t('settings.health.tracking')}</b> {c.name}
							<br /><span class="muted"
								>{c.health === 'degraded'
									? t('settings.health.readerDown')
									: t('settings.health.readStale')}</span
							>
						</span>
						<span class="chip {c.health === 'degraded' ? 'fail' : 'wait'}">{c.health === 'degraded' ? t('settings.health.syncDegraded') : t('settings.health.checking')}</span>
						<time class="xs muted" datetime={c.seen_at ?? ''} title={t('settings.health.lastRead')}>{c.seen_at ? ago(c.seen_at) : ''}</time>
					</li>
				{/each}
			</ul>
		{/if}
		{#if checks.checks.length === 0}
			<p class="small muted" style="margin-top: var(--s-3)">{t('settings.health.none')}</p>
		{:else}
			<ul class="rows checks">
				{#each checks.checks as c (c.key)}
					<li data-testid="health-check" data-state={c.state}>
						<span class="grow small">
							<b>{checkName(c.kind, c.subject).label}</b>{#if checkName(c.kind, c.subject).subject}{' '}<span class="subj" title={checkName(c.kind, c.subject).subject}>{shortPath(checkName(c.kind, c.subject).subject)}</span>{/if}
							{#if c.message}<br /><span class="muted">{c.message}</span>{/if}
						</span>
						<span class="chip {c.state === 'ok' ? 'ok' : c.state === 'warning' ? 'wait' : 'fail'}">{stateWord(c.state)}</span>
						<time class="xs muted" datetime={c.since} title={t('settings.health.since')}>{ago(c.since)}</time>
					</li>
				{/each}
			</ul>
		{/if}
	</section>

	{#if settings}
		<form class="sc-form" id="health-form" onsubmit={saveSettings} aria-label={t('settings.health.settings')}>
			<SettingsCard id="health-settings" title={t('settings.health.settings')}>
				<label class="checkline" id="update-check-row"
					><input id="update-check" type="checkbox" bind:checked={settings.update_check} /> {t('settings.health.updateCheck')}</label
				>
				<p class="hint">{t('settings.health.onlyChecks')}</p>
				<p class="hint" data-testid="update-privacy">{updatePrivacy(update?.version ?? 'version')}</p>
				{#if settings.update_check}
					<Field id="hs-days" label={t('settings.health.every')}>
						<select id="hs-days" class="input" bind:value={settings.update_check_every_days}>
							<option value={1}>{t('settings.health.day')}</option>
							<option value={3}>{t('settings.health.days', { n: 3 })}</option>
							<option value={7}>{t('settings.health.days', { n: 7 })}</option>
							<option value={14}>{t('settings.health.days', { n: 14 })}</option>
							<option value={30}>{t('settings.health.days', { n: 30 })}</option>
						</select>
					</Field>
				{/if}
				<Field id="hs-low" label={t('settings.health.lowSpace')} hint={t('settings.health.lowSpaceHint')}>
					<input id="hs-low" class="input" type="number" min="0" bind:value={settings.low_space_mb} />
				</Field>
				{#if settingsError}<p class="form-error" role="alert">{settingsError}</p>{/if}
				{#snippet advanced()}
					<div class="two">
						<Field id="hs-every" label={t('settings.health.minutes')}>
							<input id="hs-every" class="input" type="number" min="1" max="1440" bind:value={settings!.every_minutes} />
						</Field>
						<Field id="hs-guard" label={t('settings.health.guard')} hint={t('settings.health.guardHint')}>
							<input id="hs-guard" class="input" type="number" min="1" max="20" bind:value={settings!.flap_guard} />
						</Field>
					</div>
					<Field id="hs-upd" label={t('settings.health.feed')} hint={t('settings.health.feedHint')}>
						<input id="hs-upd" class="input mono" bind:value={settings!.update_check_url} placeholder={settings!.update_check_default} autocomplete="off" />
					</Field>
					<label class="checkline"><input type="checkbox" bind:checked={settings!.update_check_local} /> {t('settings.health.feedLocal')}</label>
					<label class="checkline"><input type="checkbox" bind:checked={settings!.update_prerelease} /> {t('settings.health.prerelease')}</label>
				{/snippet}
			</SettingsCard>
		</form>
		<SaveBar {dirty} form="health-form" {saving} ondiscard={() => (settings = JSON.parse(saved))} />
	{/if}
{/if}

<style>
	/* One grid for every result: name and message, then the status in a fixed column, then the time. */
	.checks > li { display: grid !important; grid-template-columns: minmax(0, 1fr) 96px 72px; align-items: center; column-gap: 16px; min-height: 52px; padding: 10px 0 !important; }
	.checks > li > .grow { min-width: 0; overflow-wrap: anywhere; font-size: 14px; }
	.checks > li > .grow :global(.muted) { font-size: 13px; }
	.checks > li > time { text-align: right; font-variant-numeric: tabular-nums; }
	.sub { color: var(--text-muted); font-weight: 400; letter-spacing: 0; }
	.two {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--s-3);
	}
	.two :global(.field + .field) {
		margin-top: 0;
	}
	@media (max-width: 600px) {
		.two {
			grid-template-columns: 1fr;
		}
		.checks > li { grid-template-columns: minmax(0, 1fr) auto; grid-template-areas: 'name name' 'st time'; row-gap: 6px; }
		.checks > li > .grow { grid-area: name; } .checks > li > :global(.chip) { grid-area: st; } .checks > li > time { grid-area: time; }
		.sub { display: none; }
	}
</style>
