<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import Releases from './Releases.svelte';
	import GoBack from './GoBack.svelte';
	import { RELEASE_PAGES, plainVersion, releaseLink } from './update-notice.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { page } from '$app/state';
	import { api, message } from '../core/api.ts';
	import { ago, dateTime } from '../core/format.ts';
	import { to } from '../core/paths.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import type { HealthCheck, UpdateInfo } from '../core/types.ts';

	type Update = UpdateInfo & { notes?: string };
	let {
		upd: real,
		docker,
		reader,
		checksKnown = true,
		canAct,
		onupd
	}: {
		upd: Update | null;
		/** True in a container; null until the app info loads. */
		docker: boolean | null;
		/** The trailers health check. */
		reader: HealthCheck | undefined;
		/** False when the health call failed, so a missing reader check means unknown. */
		checksKnown?: boolean;
		canAct: boolean;
		onupd: (u: Update) => void;
	} = $props();

	// Dev builds only: ?releases=demo fakes a one-release-behind state.
	const upd: Update | null = $derived(
		import.meta.env.DEV && page.url.searchParams.get('releases') === 'demo' && real
			? { ...real, on: true, newer: true, version: '1.3.2', latest: '1.4.0', checked_at: new Date().toISOString(), link: `${RELEASE_PAGES}tag/v1.4.0`, problem: undefined, no_release: false, notes: ['Status page with Health, Parts and Jobs.', 'Requests page shows live steps for each ask.', 'History keeps sign-ins and settings changes for a year.'].join('\n') }
			: real
	);
	let busy = $state(false);
	async function check() {
		busy = true;
		try {
			onupd(await api.post<Update>('/app/update/check'));
		} catch (e) {
			toasts.push(t('updates.check.failed'), 'fail', { body: message(e) });
		} finally {
			busy = false;
		}
	}

	type Standing = 'loading' | 'off' | 'behind' | 'problem' | 'none' | 'current' | 'unchecked';
	const standing = $derived<Standing>(
		!upd ? 'loading' : !upd.on ? 'off' : upd.newer ? 'behind' : upd.problem ? 'problem' : upd.no_release ? 'none' : upd.checked_at ? 'current' : 'unchecked'
	);
	const said = $derived<Record<Standing, string>>({
		loading: '…',
		off: t('updates.said.off'),
		behind: t('updates.said.behind'),
		problem: t('updates.said.problem'),
		none: t('updates.said.none'),
		current: t('updates.said.current'),
		unchecked: t('updates.said.unchecked')
	});
	const notes = $derived((upd?.newer && upd.notes ? upd.notes : '').split('\n').filter(Boolean));
	const link = $derived(releaseLink(upd?.link));

	// The subject starts with "Trailer reader"; keep only what follows it.
	const readerSays = $derived.by(() => {
		const rest = (reader?.subject ?? '').replace(/^Trailer reader,?\s*/, '');
		return rest ? rest[0].toUpperCase() + rest.slice(1) : t('updates.reader.none');
	});
	const readerTone = $derived(!reader ? 'none' : reader.state === 'ok' ? (reader.message ? 'wait' : 'ok') : reader.state === 'warning' ? 'wait' : 'fail');
</script>

<section class="st-plate band" data-standing={standing} aria-label={t('updates.band.label')} data-testid="status-updates">
	<div class="vers">
		<span class="v"><small>{t('updates.band.this')}</small><b>{upd?.version ? plainVersion(upd.version) : '…'}</b></span>
		{#if standing === 'behind' && upd?.latest}
			<span class="arrow" aria-hidden="true"><svg viewBox="0 0 20 20"><path d="M4 10h12m-5-5 5 5-5 5" /></svg></span>
			<span class="v new"><small>{t('updates.releases.newest')}</small><b>{plainVersion(upd.latest)}</b></span>
		{:else if standing === 'current'}
			<span class="chip ok">{t('updates.band.upToDate')}</span>
		{/if}
	</div>
	<p class="say">{said[standing]}{#if standing === 'problem' && upd?.problem} <span class="why">{upd.problem}</span>{/if}</p>
	<div class="acts">
		{#if upd?.checked_at}<span class="when">{t('updates.band.checked')} <time datetime={upd.checked_at} title={dateTime(upd.checked_at)}>{ago(upd.checked_at)}</time></span>{/if}
		{#if link}<a class="xbtn" href={link} target="_blank" rel="noopener noreferrer">{t('updates.band.whatChanged')}</a>{/if}
		{#if canAct && upd?.on}<Button {busy} onclick={check}>{upd.checked_at ? t('updates.band.checkAgain') : t('updates.band.checkNow')}</Button>{/if}
		{#if canAct && upd && !upd.on}<Button href={to('/settings/health')}>{t('updates.band.turnOn')}</Button>{/if}
	</div>
</section>

{#if notes.length}
	<section class="st-plate" aria-labelledby="s-new">
		<h2 id="s-new">{t('updates.newSince')}</h2>
		<ul class="notes">{#each notes as line (line)}<li>{line}</li>{/each}</ul>
	</section>
{/if}

{#if !session.held('go_back')}<GoBack version={real?.version} {canAct} />{/if}

<Releases {upd} />

<style>
	ul { margin: 0; padding: 0; list-style: none; }

	.band { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 8px 28px; padding: 20px 24px; }
	.band[data-standing='behind'] { border-color: color-mix(in srgb, var(--wait) 45%, transparent); background: linear-gradient(135deg, color-mix(in srgb, var(--wait) 10%, color-mix(in srgb, var(--surface-1) 55%, transparent)), color-mix(in srgb, var(--surface-1) 55%, transparent) 55%); }
	.vers { display: flex; align-items: center; gap: 16px; }
	.v { display: flex; flex-direction: column; gap: 2px; }
	.v small { font-size: 12px; color: var(--text-muted); }
	.v b { font: 600 28px/1.1 var(--font-ui); letter-spacing: -0.01em; color: var(--text); font-variant-numeric: tabular-nums; }
	.v.new b { color: var(--wait-text, var(--text)); }
	.arrow { display: grid; place-items: center; width: 28px; height: 28px; margin-top: 14px; color: var(--text-muted); }
	.arrow svg { width: 18px; height: 18px; fill: none; stroke: currentColor; stroke-width: 1.8; stroke-linecap: round; stroke-linejoin: round; }
	.say { margin: 0; max-width: 60ch; font-size: 14.5px; line-height: 1.5; color: var(--text); }
	.why { color: var(--text-muted); }
	.acts { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; justify-content: flex-end; }
	.when { margin-right: 8px; font-size: 12.5px; color: var(--text-muted); white-space: nowrap; }
	/* Looks like the primary Button, which cannot open a new tab. */
	.xbtn { display: inline-flex; align-items: center; justify-content: center; min-height: 36px; padding: 0 16px; border-radius: var(--r-btn); background: var(--accent-fill); color: var(--text-on-red); font: var(--t-headline); font-size: 15px; letter-spacing: -0.01em; text-decoration: none; white-space: nowrap; }
	.xbtn:hover { background: var(--accent-fill-hover); }
	.xbtn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (pointer: coarse) { .xbtn { min-height: 44px; } }
	.vers .chip { margin-top: 14px; }

	.notes { display: flex; flex-direction: column; gap: 8px; }
	.notes li { position: relative; padding-left: 18px; font-size: 14.5px; line-height: 1.5; color: var(--text); }
	.notes li::before { content: ''; position: absolute; left: 2px; top: 9px; width: 6px; height: 6px; border-radius: 50%; background: var(--accent, var(--text)); }

	@media (max-width: 900px) {
		.band { grid-template-columns: minmax(0, 1fr); gap: 10px; padding: 18px 20px; }
		.acts { justify-content: flex-start; }
		.when { flex-basis: 100%; margin: 0; }
	}
	@media (max-width: 767px) {
		.band { padding: 16px 14px; }
		.v b { font-size: 24px; }
	}
</style>
