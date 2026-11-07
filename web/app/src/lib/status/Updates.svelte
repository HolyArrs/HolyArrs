<script lang="ts">
	import { session } from '#lib/session.svelte.ts';
	// The Updates tab: where this HolyArrs stands against the newest release,
	// what is new since it, how an update is done (HolyArrs never updates
	// itself), the built-in trailer reader's state, and every release.
	import { page } from '$app/state';
	import { api, message } from '../api.ts';
	import { ago, dateTime } from '../format.ts';
	import { to } from '../paths.ts';
	import { toasts } from '../toasts.svelte.ts';
	import { RELEASE_PAGES, releaseLink } from '../update.ts';
	import type { HealthCheck, UpdateInfo } from '../types.ts';
	import Button from '../ui/Button.svelte';
	import Releases from './Releases.svelte';
	import GoBack from './GoBack.svelte';

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
		/** Whether this HolyArrs runs in a container: Docker, Podman, Kubernetes (null until About answers). */
		docker: boolean | null;
		/** The trailers health check: the trailer reader's last good lookup and any failures. */
		reader: HealthCheck | undefined;
		/** False when the health checks call failed: a missing reader check then means unknown, not absent. */
		checksKnown?: boolean;
		canAct: boolean;
		onupd: (u: Update) => void;
	} = $props();

	// Dev builds only: ?releases=demo shows the one-release-behind state with made-up notes (Releases shows its demo history too).
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
			toasts.push("Couldn't check for updates", 'fail', { body: message(e) });
		} finally {
			busy = false;
		}
	}

	type Standing = 'loading' | 'off' | 'behind' | 'problem' | 'none' | 'current' | 'unchecked';
	const standing = $derived<Standing>(
		!upd ? 'loading' : !upd.on ? 'off' : upd.newer ? 'behind' : upd.problem ? 'problem' : upd.no_release ? 'none' : upd.checked_at ? 'current' : 'unchecked'
	);
	const said: Record<Standing, string> = {
		loading: '…',
		off: 'The update check is off, so HolyArrs never asks whether a newer version is out.',
		behind: 'A newer release is out. Updating keeps your data and settings: they live in the config folder, not in the program.',
		problem: 'The last check did not work.',
		none: 'No HolyArrs release is published yet.',
		current: 'This is the newest release.',
		unchecked: 'Not checked yet.'
	};
	const notes = $derived((upd?.newer && upd.notes ? upd.notes : '').split('\n').filter(Boolean));
	const link = $derived(releaseLink(upd?.link));

	// The subject is "Trailer reader" plus ", last good lookup <when>" once one worked.
	const readerSays = $derived.by(() => {
		const rest = (reader?.subject ?? '').replace(/^Trailer reader,?\s*/, '');
		return rest ? rest[0].toUpperCase() + rest.slice(1) : 'No good lookup yet';
	});
	const readerTone = $derived(!reader ? 'none' : reader.state === 'ok' ? (reader.message ? 'wait' : 'ok') : reader.state === 'warning' ? 'wait' : 'fail');
</script>

<section class="st-plate band" data-standing={standing} aria-label="Your version" data-testid="status-updates">
	<div class="vers">
		<span class="v"><small>This HolyArrs</small><b>v{upd?.version ?? '…'}</b></span>
		{#if standing === 'behind' && upd?.latest}
			<span class="arrow" aria-hidden="true"><svg viewBox="0 0 20 20"><path d="M4 10h12m-5-5 5 5-5 5" /></svg></span>
			<span class="v new"><small>Newest</small><b>v{upd.latest}</b></span>
		{:else if standing === 'current'}
			<span class="chip ok">Up to date</span>
		{/if}
	</div>
	<p class="say">{said[standing]}{#if standing === 'problem' && upd?.problem} <span class="why">{upd.problem}</span>{/if}</p>
	<div class="acts">
		{#if upd?.checked_at}<span class="when">Checked <time datetime={upd.checked_at} title={dateTime(upd.checked_at)}>{ago(upd.checked_at)}</time></span>{/if}
		{#if link}<a class="xbtn" href={link} target="_blank" rel="noopener noreferrer">What changed</a>{/if}
		{#if canAct && upd?.on}<Button {busy} onclick={check}>{upd.checked_at ? 'Check again' : 'Check now'}</Button>{/if}
		{#if canAct && upd && !upd.on}<Button href={to('/settings/health')}>Turn the check on</Button>{/if}
	</div>
</section>

{#if notes.length}
	<section class="st-plate" aria-labelledby="s-new">
		<h2 id="s-new">New since yours</h2>
		<ul class="notes">{#each notes as line (line)}<li>{line}</li>{/each}</ul>
	</section>
{/if}

{#if !session.held('go_back')}<GoBack version={real?.version} {canAct} />{/if}

<Releases {upd} />

<style>
	ul { margin: 0; padding: 0; list-style: none; }

	/* band */
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
	/* Outside link in a new tab, drawn like the shared primary Button (whose link form has no target). */
	.xbtn { display: inline-flex; align-items: center; justify-content: center; min-height: 36px; padding: 0 16px; border-radius: var(--r-btn); background: var(--accent-fill); color: var(--text-on-red); font: var(--t-headline); font-size: 15px; letter-spacing: -0.01em; text-decoration: none; white-space: nowrap; }
	.xbtn:hover { background: var(--accent-fill-hover); }
	.xbtn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (pointer: coarse) { .xbtn { min-height: 44px; } }
	.vers .chip { margin-top: 14px; }

	/* new since yours */
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
