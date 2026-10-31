<script lang="ts">
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import SaveBar from '#lib/ui/SaveBar.svelte';
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { ago } from '#lib/format.ts';
	import { session } from '#lib/session.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { checkLabel, stateWord } from '#lib/health.ts';
	import { refreshSettingsStatus } from '#lib/settings-status.svelte.ts';
	import type { HealthChecks, HealthSettings, UpdateInfo } from '#lib/types.ts';
	import { updatePrivacy } from '#lib/update.ts';

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
			keep(
				await api.put<HealthSettings>('/settings/health', {
					...settings,
					every_minutes: Number(settings.every_minutes),
					flap_guard: Number(settings.flap_guard),
					low_space_mb: Number(settings.low_space_mb)
				})
			);
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

<svelte:head><title>Health checks · HolyArrs</title></svelte:head>

<PageHeader title="Health checks">
	{#snippet actions()}
		<Button type="button" onclick={runNow} disabled={running} aria-busy={running}>Check again</Button>
	{/snippet}
</PageHeader>

{#if error}
	<p class="notice bad" role="alert">{error}</p>
{:else if !checks}
	<SkSettings rows={3} />
{:else}
	<section class="card" id="health-checks-card" aria-labelledby="h-checks" data-testid="health-checks" tabindex="-1">
		<div class="card-head">
			<h2 id="h-checks" class="h2">Results</h2>
			<span class="chip {checks.worst === 'ok' ? 'ok' : checks.worst === 'warning' ? 'wait' : 'fail'}" data-testid="health-worst">{stateWord(checks.worst)}</span>
		</div>
		<p class="small muted">When something breaks or is fixed, the Alerts that speak for health hear of it.</p>
		{#if checks.sync?.length}
			<ul class="rows checks" data-testid="health-sync">
				{#each checks.sync as c (c.id)}
					<li data-testid="health-sync-row" data-state={c.health}>
						<span class="chip {c.health === 'degraded' ? 'fail' : 'wait'}">{c.health === 'degraded' ? 'Sync degraded' : 'Checking'}</span>
						<span class="grow small">
							<b>Downloads reader</b> {c.name}
							<br /><span class="muted"
								>{c.health === 'degraded'
									? 'The background reader is down; HolyArrs reads this Downloader itself until it is back.'
									: 'The last read is too old; a new one is on its way.'}</span
							>
						</span>
						{#if c.seen_at}<time class="mono xs muted" datetime={c.seen_at} title="Last read">{ago(c.seen_at)}</time>{/if}
					</li>
				{/each}
			</ul>
		{/if}
		{#if checks.checks.length === 0}
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
		<form class="sc-form" id="health-form" onsubmit={saveSettings} aria-label="Health check settings">
			<SettingsCard id="health-settings" title="Health check settings" description="How often HolyArrs checks, and when it looks for a newer version.">
				<label class="checkline" id="update-check-row"
					><input id="update-check" type="checkbox" bind:checked={settings.update_check} /> Check once a day for a newer HolyArrs</label
				>
				<p class="hint">Off: HolyArrs never asks. On: one small request a day to the release feed; nothing is ever installed by itself.</p>
				<p class="hint" data-testid="update-privacy">{updatePrivacy(update?.version ?? 'version')}</p>
				<Field id="hs-low" label="Warn when free space is under (MB)" hint="A health warning goes out when a Library folder's disk has less free space than this.">
					<input id="hs-low" class="input" type="number" min="0" bind:value={settings.low_space_mb} />
				</Field>
				{#if settingsError}<p class="form-error" role="alert">{settingsError}</p>{/if}
				{#snippet advanced()}
					<div class="two">
						<Field id="hs-every" label="Check every (minutes)" hint="Every check runs again after this many minutes.">
							<input id="hs-every" class="input" type="number" min="1" max="1440" bind:value={settings!.every_minutes} />
						</Field>
						<Field id="hs-guard" label="Alert after this many runs in a row" hint="A break or a fix must hold this long before an Alert goes out.">
							<input id="hs-guard" class="input" type="number" min="1" max="20" bind:value={settings!.flap_guard} />
						</Field>
					</div>
					<Field id="hs-upd" label="Release feed" hint="Empty: the HolyArrs project's releases.">
						<input id="hs-upd" class="input mono" bind:value={settings!.update_check_url} placeholder={settings!.update_check_default} autocomplete="off" />
					</Field>
					<label class="checkline"><input type="checkbox" bind:checked={settings!.update_check_local} /> The release feed may be on the local network</label>
					<label class="checkline"><input type="checkbox" bind:checked={settings!.update_prerelease} /> Tell me about test versions too (such as v1.2.0-rc1)</label>
				{/snippet}
			</SettingsCard>
		</form>
		<SaveBar {dirty} form="health-form" {saving} ondiscard={() => (settings = JSON.parse(saved))} />
	{/if}
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
		.checks li > :global(.chip),
		.checks li > time {
			align-self: flex-start;
		}
	}
</style>
