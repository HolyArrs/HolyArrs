<script lang="ts">
	import Words from '#lib/ui/Words.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import MoreOptions from '#lib/ui/MoreOptions.svelte';
	import SettingsLinks from '#lib/ui/SettingsLinks.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { refreshParts, partNeeds, partStops, type PartName, type PartsAnswer, type PartView } from '#lib/mediafiles/index.ts';
	import { isConfirm, linkAction, linkTarget, type Confirm } from '#lib/setup/index.ts';
	import { linkLabel, runTone, runWord, splitStates } from '#lib/health/index.ts';
	import type { HealthStates, StageState } from '#lib/health/index.ts';
	import { onMount } from 'svelte';
	import { testLabel } from '#lib/core/test-tag.ts';
	import { to } from '#lib/core/paths.ts';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { ago } from '#lib/core/format.ts';
	import { live } from '#lib/core/live.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import type { EventRecord } from '#lib/core/types.ts';

	let parts = $state<PartsAnswer | null>(null);
	let states = $state<HealthStates | null>(null);
	let events = $state<EventRecord[]>([]);
	let error = $state('');
	let busy = $state('');
	const split = $derived(splitStates(states?.states));
	const allowed = $derived(session.can('users'));
	const safeMode = $derived(!!parts?.safe_mode || session.s.safeMode);

	// The control waiting for its second step: what it does, in the server's words.
	let asking = $state<{ key: string; label: string; path: string; confirm: Confirm; then: (r: unknown) => void } | null>(null);

	async function load() {
		try {
			const [p, s, e] = await Promise.all([
				api.get<PartsAnswer>('/parts'),
				api.get<HealthStates>('/health/states'),
				api.get<{ events: EventRecord[] }>('/events?kind=control_used&limit=15').catch(() => ({ events: [] }))
			]);
			parts = p;
			states = s;
			events = e.events;
			error = '';
		} catch (err) {
			error = message(err);
		}
	}

	onMount(() => {
		if (!allowed) return;
		void load();
		const timer = setInterval(load, 15000);
		const offs = [live.on('health', load)];
		return () => {
			clearInterval(timer);
			offs.forEach((off) => off());
		};
	});

	/** Sends a control; one that asks first waits for the second step on this page. */
	async function run(key: string, label: string, path: string, then: (r: unknown) => void) {
		busy = key;
		try {
			const r = await api.post<unknown>(path, {});
			if (isConfirm(r)) asking = { key, label, path, confirm: r, then };
			else then(r);
		} catch (err) {
			toasts.push(t('settings.parts.couldnt', { action: label.charAt(0).toLowerCase() + label.slice(1) }), 'fail', { body: message(err) });
		} finally {
			busy = '';
		}
	}

	async function confirmNow() {
		if (!asking) return;
		const a = asking;
		busy = a.key;
		try {
			const r = await api.post<unknown>(a.path, { confirm: a.confirm.confirm });
			asking = null;
			a.then(r);
		} catch (err) {
			asking = null;
			toasts.push(t('settings.parts.couldnt', { action: a.label.charAt(0).toLowerCase() + a.label.slice(1) }), 'fail', { body: message(err) });
		} finally {
			busy = '';
		}
	}

	function done(text: string) {
		return (r: unknown) => {
			const said = r && typeof r === 'object' && typeof (r as { text?: unknown }).text === 'string' ? (r as { text: string }).text : text;
			toasts.push(said, 'ok');
			void load();
			void refreshParts();
		};
	}

	function switchPart(p: PartView, on: boolean) {
		run(`part-${p.part}`, on ? t('settings.parts.turnOnName', { name: p.name }) : t('settings.parts.turnOffName', { name: p.name }), `/parts/${p.part}/${on ? 'on' : 'off'}`, done(on ? t('settings.parts.turnedOn', { name: p.name }) : t('settings.parts.turnedOff', { name: p.name })));
	}

	function restartPart(p: PartView) {
		run(`restart-${p.part}`, t('settings.parts.restartName', { name: p.name }), `/parts/${p.part}/restart`, done(t('settings.parts.restarted', { name: p.name })));
	}

	function stage(s: StageState, action: 'pause' | 'resume' | 'restart') {
		const name = s.key.replace(/^stage:/, '');
		run(`stage-${name}-${action}`, t('settings.parts.stage.' + action, { name: s.name }), `/stages/${encodeURIComponent(name)}/${action}`, done(t('settings.parts.stageDone', { name: s.name })));
	}

	function reconnect(l: StageState) {
		const target = linkTarget(l.key);
		if (!target) return;
		run(`link-${l.key}`, `${linkAction[target.kind]} ${l.name}`, `/links/${target.kind}/${encodeURIComponent(target.id)}/restart`, (r) => {
			// The answer of the link's own test: the same words as its tag ("qBittorrent v5.1.2"), or Not connected and why.
			const o = (r ?? {}) as { ok?: boolean; text?: string; message?: string; error?: string; found?: string };
			const said = o.text || o.message || o.error || o.found || '';
			toasts.push(o.ok === false ? t('settings.parts.couldntConnect', { name: l.name }) : t('settings.parts.connected', { name: l.name }), o.ok === false ? 'fail' : 'ok', { body: o.ok === false ? said || undefined : testLabel(said) });
			void load();
		});
	}

	const paused = (s: StageState) => s.state === 'off';
	const pausable = (s: StageState) => !!parts?.stages?.includes(s.key.replace(/^stage:/, ''));

	// The second step takes the focus (and scrolls into view) when it shows.
	$effect(() => {
		if (asking) queueMicrotask(() => document.getElementById('confirm-yes')?.focus());
	});
</script>

<svelte:head><title>{t('settings.parts.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.parts.title')} />

{#if !allowed}
	<p class="notice warn">{t('settings.parts.ownerOnly')}</p>
{:else if error && !parts}
	<LoadError hasData={false} message={error} onretry={load} />
{:else if !parts}
	<SkSettings rows={3} />
{:else}
	{#if error}<LoadError hasData message={error} onretry={load} />{/if}
	{#if safeMode}
		<p class="notice warn" data-testid="parts-safe-mode">
			<Words key="settings.parts.safeMode" links={{ a: to('/settings/danger') }} />
		</p>
	{/if}

	{#if asking}
		<div class="card confirm" role="alertdialog" aria-labelledby="h-confirm" aria-describedby="confirm-text" data-testid="confirm">
			<h2 id="h-confirm" class="h2">{t('settings.parts.ask', { label: asking.label })}</h2>
			<p id="confirm-text" class="small">{asking.confirm.text.replace(/, send the same request again.*$/, '.')}</p>
			<div class="row">
				<Button variant="destructive" id="confirm-yes" type="button" onclick={confirmNow} disabled={busy === asking.key} aria-busy={busy === asking.key}>{t('settings.parts.yes', { action: asking.label.toLowerCase() })}</Button>
				<Button variant="plain" type="button" onclick={() => (asking = null)}>{t('settings.parts.cancel')}</Button>
			</div>
			<p class="xs muted">{t('settings.parts.expires')}</p>
		</div>
	{/if}

	<div data-testid="parts">
		<SettingsCard id="parts-card" title={t('settings.parts.title')} description={t('settings.parts.partsDesc')}>
			<ul class="rows list">
				{#each parts.parts as p (p.part)}
					<li data-testid="part" data-part={p.part} data-state={p.state}>
						<div class="grow">
							<div class="name">
								{p.name}
								<span class="chip {runTone(p.state)}">{runWord({ state: p.state, stuck: false })}</span>
							</div>
							<p class="xs muted">{t('settings.parts.whenOff', { what: partStops[p.part as PartName] })}</p>
							{#if partNeeds[p.part as PartName]}<p class="xs muted">{partNeeds[p.part as PartName]}</p>{/if}
							{#if p.reason}<p class="xs muted">{p.reason}</p>{/if}
						</div>
						<div class="row acts">
							{#if p.on}
								<Button type="button" onclick={() => restartPart(p)} disabled={!!busy} aria-label={t('settings.parts.restartName', { name: p.name })}>{t('settings.parts.restart')}</Button>
								<Button variant="plain" type="button" onclick={() => switchPart(p, false)} disabled={!!busy} aria-label={t('settings.parts.turnOffName', { name: p.name })}>{t('settings.parts.turnOff')}</Button>
							{:else}
								<Button variant="primary" type="button" onclick={() => switchPart(p, true)} disabled={!!busy} aria-label={t('settings.parts.turnOnName', { name: p.name })}>{t('settings.parts.turnOn')}</Button>
							{/if}
						</div>
					</li>
				{/each}
			</ul>
		</SettingsCard>
	</div>

	<div data-testid="control-stages">
		<SettingsCard id="stages-card" title={t('settings.parts.stages')} description={t('settings.parts.stagesDesc')}>
			{#if split.stages.length === 0}
				<p class="small muted">{t('settings.parts.noStages')}</p>
			{:else}
				<ul class="rows list">
					{#each split.stages as s (s.key)}
						<li data-testid="control-stage" data-state={s.state}>
							<div class="grow">
								<div class="name">{s.name} <span class="chip {runTone(s.state)}">{runWord(s)}</span></div>
								{#if s.reason}<p class="xs muted">{s.reason}</p>{/if}
							</div>
							{#if pausable(s)}<div class="row acts">
								{#if paused(s)}
									<Button type="button" onclick={() => stage(s, 'resume')} disabled={!!busy} aria-label={t('settings.parts.stage.resume', { name: s.name })}>{t('settings.parts.resume')}</Button>
								{:else}
									<Button variant="plain" type="button" onclick={() => stage(s, 'pause')} disabled={!!busy} aria-label={t('settings.parts.stage.pause', { name: s.name })}>{t('settings.parts.pause')}</Button>
								{/if}
								<Button type="button" onclick={() => stage(s, 'restart')} disabled={!!busy} aria-label={t('settings.parts.stage.restart', { name: s.name })}>{t('settings.parts.restart')}</Button>
							</div>{:else}<span class="xs muted">{t('settings.parts.withApp')}</span>{/if}
						</li>
					{/each}
				</ul>
			{/if}
		</SettingsCard>
	</div>

	<div data-testid="control-links">
		<SettingsCard id="links-card" title={t('settings.parts.links')} description={t('settings.parts.linksDesc')}>
			{#if split.links.length === 0}
				<p class="small muted">{t('settings.parts.noLinks')}</p>
			{:else}
				<ul class="rows list">
					{#each split.links as l (l.key)}
						<li data-testid="control-link" data-state={l.state}>
							<div class="grow">
								<div class="name">{l.name} <span class="xs muted">{linkLabel(l.kind)}</span> <span class="chip {runTone(l.state)}">{runWord(l)}</span></div>
								{#if l.reason}<p class="xs muted">{l.reason}</p>{/if}
							</div>
							{#if linkTarget(l.key)}
								<Button type="button" onclick={() => reconnect(l)} disabled={!!busy} aria-label="{linkAction[l.kind]} {l.name}"
									>{linkAction[l.kind]}</Button>
							{/if}
						</li>
					{/each}
				</ul>
			{/if}
		</SettingsCard>
	</div>

	<MoreOptions label={t('settings.parts.controls')}>
	<div data-testid="control-events">
		<SettingsCard title={t('settings.parts.controls')}>
			{#if events.length === 0}
				<p class="small muted">{t('settings.parts.nothingUsed')}</p>
			{:else}
				<ul class="rows list">
					{#each events as e (e.ID)}
						<li>
							<span class="grow small">{e.Summary}</span>
							<time class="xs muted" datetime={e.At}>{ago(e.At)}</time>
						</li>
					{/each}
				</ul>
			{/if}
		</SettingsCard>
	</div>
	</MoreOptions>

	<SettingsLinks label={t('settings.parts.wholeApp')} links={[{ href: '/settings/danger', label: t('settings.parts.dangerZone'), note: t('settings.parts.dangerNote') }]} />
{/if}

<style>
	.list {
		margin-top: var(--s-3);
	}
	.list li {
		align-items: flex-start;
		flex-wrap: wrap;
	}
	.list .grow {
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.name {
		font-weight: var(--weight-semibold);
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--s-2);
	}
	.acts {
		flex-wrap: wrap;
	}
	:global(.settings-content) .card.confirm {
		box-shadow: 0 0 0 1px var(--ha-red);
	}
	.confirm .row {
		margin-top: var(--s-3);
		flex-wrap: wrap;
	}
	@media (max-width: 600px) {
		.list li {
			align-items: stretch;
			flex-direction: column;
			gap: var(--s-2);
		}
		.list .grow,
		.list .acts {
			width: 100%;
		}
	}
</style>
