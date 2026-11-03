<script lang="ts">
	// The Updates tab: where this HolyArrs stands against the newest release,
	// what is new since it, how an update is done (HolyArrs never updates
	// itself), the bundled tools that do update themselves, and every release.
	import { page } from '$app/state';
	import { api, message } from '../api.ts';
	import { ago, dateTime } from '../format.ts';
	import { to } from '../paths.ts';
	import { toasts } from '../toasts.svelte.ts';
	import { RELEASE_PAGES, releaseLink } from '../update.ts';
	import type { HealthCheck, UpdateInfo } from '../types.ts';
	import Button from '../ui/Button.svelte';
	import Releases from './Releases.svelte';

	type Update = UpdateInfo & { notes?: string };
	let {
		upd: real,
		docker,
		ytdlp,
		checksKnown = true,
		canAct,
		onupd
	}: {
		upd: Update | null;
		/** Whether this HolyArrs runs in Docker (null until About answers). */
		docker: boolean | null;
		/** The trailers health check, which names yt-dlp's version. */
		ytdlp: HealthCheck | undefined;
		/** False when the health checks call failed: a missing yt-dlp check then means unknown, not absent. */
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
			toasts.push(message(e), 'fail');
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

	const ytVersion = $derived(ytdlp?.subject?.replace(/^yt-dlp\s*/, '') ?? '');
	const ytTone = $derived(!ytdlp ? 'none' : ytdlp.state === 'ok' ? 'ok' : ytdlp.state === 'warning' ? 'wait' : 'fail');
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

<div class="pair">
	<section class="st-plate" aria-labelledby="s-how">
		<h2 id="s-how">How to update</h2>
		<p class="lead">HolyArrs never changes its own program. Your databases, settings, keys and backups stay in the config folder through every update.</p>
		{#if docker !== true}
			{#if docker === null}<h3 class="kind">Without Docker</h3>{/if}
			<ol class="steps">
				<li><b>Stop HolyArrs</b><span>Finish or pause downloads first if you like; the queue carries on afterwards.</span></li>
				<li><b>Replace the program</b><span>Put the new build where the old one is. Leave the config folder alone.</span></li>
				<li><b>Start it again</b><span>The first start runs any database migration and shows the new version here.</span></li>
			</ol>
		{/if}
		{#if docker !== false}
			{#if docker === null}<h3 class="kind">In Docker</h3>{/if}
			<ol class="steps">
				<li><b>Get the new image</b><span>Pull the new HolyArrs image.</span></li>
				<li><b>Recreate the container</b><span>Same mounts, same ports. One command:</span><code class="cmd">docker compose up -d --force-recreate</code></li>
				<li><b>Open Status</b><span>The new version shows here; the first start runs any database migration.</span></li>
			</ol>
		{/if}
	</section>

	<section class="st-plate" aria-labelledby="s-tools">
		<h2 id="s-tools">Bundled tools</h2>
		<p class="lead">Small programs that ship inside HolyArrs and keep themselves current.</p>
		<ul class="tools">
			<li>
				<span class="tn">yt-dlp</span>
				<span class="tv">
					{#if ytdlp}
						<i class="st-dot {ytTone}" aria-hidden="true"></i>{ytVersion || 'version unknown'}
						{#if ytdlp.checked_at}<small>· checked <time datetime={ytdlp.checked_at} title={dateTime(ytdlp.checked_at)}>{ago(ytdlp.checked_at)}</time></small>{/if}
					{:else if !checksKnown}
						<i class="st-dot none" aria-hidden="true"></i>Version not known right now
					{:else}
						<i class="st-dot none" aria-hidden="true"></i>Not bundled in this build
					{/if}
				</span>
				<span class="td">Finds trailers. Once a week HolyArrs fetches its newest official release, checks it and keeps the previous copy if the new one does not work.{#if canAct} <a class="lnk" href={to('/settings/this-app')}>Setting</a>{/if}</span>
				{#if ytdlp && ytdlp.state !== 'ok' && ytdlp.message}<span class="why">{ytdlp.message}</span>{/if}
			</li>
		</ul>
	</section>
</div>

<Releases {upd} />

<style>
	ul, ol { margin: 0; padding: 0; list-style: none; }
	.pair { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--s-4); align-items: stretch; }

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
	.kind { margin: 14px 0 8px; font: 600 13px/1.3 var(--font-ui); color: var(--text); }
	.kind:first-of-type { margin-top: 0; }

	/* new since yours */
	.notes { display: flex; flex-direction: column; gap: 8px; }
	.notes li { position: relative; padding-left: 18px; font-size: 14.5px; line-height: 1.5; color: var(--text); }
	.notes li::before { content: ''; position: absolute; left: 2px; top: 9px; width: 6px; height: 6px; border-radius: 50%; background: var(--accent, var(--text)); }

	/* how to update */
	.lead { margin: 0 0 14px; font-size: 14px; line-height: 1.5; color: var(--text-muted); }
	.steps { counter-reset: step; display: flex; flex-direction: column; }
	.steps li { counter-increment: step; position: relative; display: flex; flex-direction: column; gap: 3px; padding: 12px 0 12px 38px; border-top: 1px solid var(--line); font-size: 14px; color: var(--text-muted); }
	.steps li:first-child { border-top: 0; padding-top: 2px; }
	.steps li::before { content: counter(step); position: absolute; left: 0; top: 10px; display: grid; place-items: center; width: 26px; height: 26px; border-radius: 50%; background: var(--surface-2, var(--surface-1)); color: var(--text); font: 600 12.5px/1 var(--font-ui); }
	.steps li:first-child::before { top: 0; }
	.steps b { font-weight: 600; color: var(--text); }
	code { font-family: var(--font-mono, ui-monospace, monospace); font-size: 12.5px; padding: 2px 6px; border-radius: 6px; background: var(--surface-2, var(--surface-1)); color: var(--text); }
	.cmd { display: block; width: fit-content; max-width: 100%; margin-top: 6px; padding: 8px 12px; overflow-x: auto; white-space: nowrap; }

	/* bundled tools */
	.tools li { display: grid; grid-template-columns: 90px minmax(0, 1fr); gap: 4px 14px; font-size: 14px; }
	.tn { font-weight: 600; color: var(--text); }
	.tv { display: flex; align-items: center; gap: 8px; color: var(--text); font-variant-numeric: tabular-nums; }
	.tv small { font-size: 12.5px; color: var(--text-muted); }
	.td { grid-column: 2; font-size: 13px; line-height: 1.5; color: var(--text-muted); }
	.tools .why { grid-column: 2; font-size: 13px; color: var(--wait-text, var(--text)); }
	.lnk { color: var(--accent-text, var(--text)); font-weight: 600; text-decoration: none; }
	.lnk:hover { text-decoration: underline; }

	@media (max-width: 900px) {
		.pair { grid-template-columns: minmax(0, 1fr); }
		.band { grid-template-columns: minmax(0, 1fr); gap: 10px; padding: 18px 20px; }
		.acts { justify-content: flex-start; }
		.when { flex-basis: 100%; margin: 0; }
	}
	@media (max-width: 767px) {
		.band { padding: 16px 14px; }
		.v b { font-size: 24px; }
		.tools li { grid-template-columns: minmax(0, 1fr); }
		.td, .tools .why { grid-column: 1; }
	}
</style>
