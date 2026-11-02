<script lang="ts">
	// Status: is every linked part working, and did every background job run?
	// Reads only the existing health, schedules, lists, alerts and metadata
	// routes. Preview pickers: ?bg=1..13 (page background), ?surface=1..13
	// (section plates), ?pixel=1 (pixel headings), ?cards= (Parts deck).
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { checkMetadata } from '#lib/metadata-health.svelte.ts';
	import type { AlertView, HealthChecks, HealthStates, OutsideList } from '#lib/types.ts';
	import type { ScheduleRow } from '#lib/schedules.ts';
	import LookPills from '#lib/status/LookPills.svelte';
	import Sections from '#lib/status/StatusSections.svelte';
	import { bgOf, buildRows, groupOrder, groupRows, headline, needsAttention, surfaceOf, type StatusInput, type StatusRow } from '#lib/status/status.ts';

	const tab = $derived(['jobs', 'about'].find((t) => t === page.url.searchParams.get('tab')) ?? 'parts');
	const bg = $derived(bgOf(page.url.searchParams.get('bg')));
	const surface = $derived(surfaceOf(page.url.searchParams.get('surface')));
	const pixel = $derived(page.url.searchParams.get('pixel') === '1');

	let input = $state<StatusInput | null>(null);
	let error = $state('');
	let busy = $state<string | null>(null);
	const canAct = $derived(session.can('settings'));

	const rows = $derived(input ? buildRows({ ...input, now: Date.now() }) : []);
	const groups = $derived(groupRows(rows));
	const problems = $derived(needsAttention(rows));
	const top = $derived(headline(rows));
	const labelOf = (r: StatusRow) => groupOrder.find((g) => g.id === r.group)?.label ?? '';

	async function load() {
		const [states, checks, sched, lists, alerts, meta] = await Promise.allSettled([
			api.get<HealthStates>('/health/states'),
			api.get<HealthChecks>('/health/checks'),
			api.get<{ schedules: ScheduleRow[] }>('/schedules'),
			api.get<{ outside_lists: OutsideList[] }>('/outside-lists'),
			api.get<{ alerts: AlertView[] }>('/alerts'),
			checkMetadata(true)
		]);
		const val = <T,>(r: PromiseSettledResult<T>) => (r.status === 'fulfilled' ? r.value : undefined);
		if (states.status === 'rejected' && sched.status === 'rejected') {
			error = message(states.reason);
			return;
		}
		error = '';
		input = {
			states: val(states)?.states,
			checks: val(checks)?.checks,
			schedules: val(sched)?.schedules,
			lists: val(lists)?.outside_lists,
			alerts: val(alerts)?.alerts,
			metadataUp: meta.status === 'fulfilled' ? meta.value : false,
			metadataAt: new Date().toISOString()
		};
	}

	async function check(r: StatusRow) {
		const c = r.check;
		if (!c) return;
		busy = r.key;
		try {
			if (c.kind === 'job') {
				const res = await api.post<{ already: boolean }>(`/schedules/${c.job}/run`);
				toasts.push(res.already ? `${r.name} is already waiting or running.` : `${r.name} started.`, 'ok');
			} else if (c.kind === 'checks') {
				await api.post('/health/checks/run');
			} else {
				const res = await api.post<{ ok: boolean; text: string }>(`/${c.kind}s/${c.id}/test`);
				toasts.push(res.ok ? `${r.name} answered.` : res.text || `${r.name} did not answer.`, res.ok ? 'ok' : 'fail');
			}
			await load();
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = null;
		}
	}

	async function checkAll() {
		busy = 'all';
		try {
			await api.post('/health/checks/run');
			await load();
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = null;
		}
	}

	function setTab(t: string) {
		const u = new URL(page.url.href);
		u.searchParams.set('tab', t);
		void goto(u.pathname + u.search);
	}

	onMount(() => {
		void load();
		// Live pushes cover health and jobs; the 60s poll catches lists, alerts and the metadata server.
		const offs = [live.on('health', load), live.on('job', load)];
		const t = setInterval(() => {
			if (document.visibilityState === 'visible') void load();
		}, 60_000);
		const vis = () => document.visibilityState === 'visible' && void load();
		document.addEventListener('visibilitychange', vis);
		return () => {
			clearInterval(t);
			offs.forEach((off) => off());
			document.removeEventListener('visibilitychange', vis);
		};
	});
</script>

<svelte:head><title>Status · HolyArrs</title></svelte:head>

<div class="backdrop" data-bg={bg} aria-hidden="true"></div>
<div class="status" data-testid="status-page" data-surface={surface} data-pixel={pixel || undefined}>
	<header class="sbar">
		<div class="title">
			<h1>Status</h1>
			{#if input}<p class="head {problems.length ? 'bad' : 'good'}" data-testid="status-headline">{top}</p>{/if}
		</div>
		{#if canAct}
			<button type="button" class="btn" onclick={checkAll} disabled={busy === 'all'} aria-busy={busy === 'all'}>Check all</button>
		{/if}
	</header>

	{#if error}
		<p class="notice" role="alert">{error}</p>
	{:else if !input}
		<p class="muted">Loading…</p>
	{:else}
		<Sections {groups} {tab} ontab={setTab} {canAct} {busy} oncheck={check} {labelOf} />
	{/if}
</div>

<LookPills />

<style>
	/* Page backgrounds (?bg=1..10). Each sits fixed behind the page. */
	.backdrop {
		--base: var(--bg, var(--surface-0, var(--surface-1)));
		--acc: var(--accent, #6b8cff);
		position: fixed; inset: 0; z-index: -1; pointer-events: none; background: var(--base);
	}
	.backdrop::after { content: ''; position: absolute; inset: 0; pointer-events: none; }
	/* 1 Glow + dots */
	.backdrop[data-bg='1'] {
		background:
			radial-gradient(60% 50% at 85% 0%, color-mix(in srgb, var(--acc) 14%, transparent), transparent 70%),
			radial-gradient(50% 45% at 0% 100%, color-mix(in srgb, var(--ok) 10%, transparent), transparent 70%),
			radial-gradient(circle at 1px 1px, color-mix(in srgb, var(--text) 9%, transparent) 1px, transparent 1.5px) 0 0 / 22px 22px, var(--base);
	}
	/* 2 Aurora: big soft colour blobs */
	.backdrop[data-bg='2'] {
		background:
			radial-gradient(40% 35% at 20% 15%, color-mix(in srgb, #7c5cff 22%, transparent), transparent 70%),
			radial-gradient(35% 30% at 80% 25%, color-mix(in srgb, #00c2a8 18%, transparent), transparent 70%),
			radial-gradient(45% 40% at 60% 90%, color-mix(in srgb, #ff6b8b 14%, transparent), transparent 70%), var(--base);
	}
	/* 3 Fine grid */
	.backdrop[data-bg='3'] {
		background:
			linear-gradient(color-mix(in srgb, var(--text) 6%, transparent) 1px, transparent 1px) 0 0 / 32px 32px,
			linear-gradient(90deg, color-mix(in srgb, var(--text) 6%, transparent) 1px, transparent 1px) 0 0 / 32px 32px, var(--base);
	}
	.backdrop[data-bg='3']::after { background: radial-gradient(70% 60% at 50% 0%, transparent, var(--base)); }
	/* 4 Diagonal pinstripe */
	.backdrop[data-bg='4'] { background: repeating-linear-gradient(135deg, color-mix(in srgb, var(--text) 4%, transparent) 0 1px, transparent 1px 12px), var(--base); }
	.backdrop[data-bg='4']::after { background: linear-gradient(180deg, transparent 40%, var(--base)); }
	/* 5 Spotlight from above */
	.backdrop[data-bg='5'] { background: radial-gradient(55% 45% at 50% -5%, color-mix(in srgb, var(--text) 12%, transparent), transparent 70%), var(--base); }
	.backdrop[data-bg='5']::after { background: radial-gradient(120% 90% at 50% 30%, transparent 55%, rgb(0 0 0 / 0.35)); }
	/* 6 Contour rings */
	.backdrop[data-bg='6'] { background: repeating-radial-gradient(circle at 85% 10%, transparent 0 22px, color-mix(in srgb, var(--acc) 9%, transparent) 22px 23px), var(--base); }
	.backdrop[data-bg='6']::after { background: radial-gradient(80% 80% at 85% 10%, transparent 30%, var(--base) 80%); }
	/* 7 Mesh gradient */
	.backdrop[data-bg='7'] {
		background:
			radial-gradient(at 0% 0%, color-mix(in srgb, #4f7cff 18%, transparent) 0, transparent 50%),
			radial-gradient(at 100% 0%, color-mix(in srgb, #b45cff 16%, transparent) 0, transparent 50%),
			radial-gradient(at 100% 100%, color-mix(in srgb, #ff8a4c 12%, transparent) 0, transparent 50%),
			radial-gradient(at 0% 100%, color-mix(in srgb, #22c3a6 14%, transparent) 0, transparent 50%), var(--base);
	}
	/* 8 Carbon weave */
	.backdrop[data-bg='8'] {
		background:
			linear-gradient(27deg, rgb(0 0 0 / 0.18) 5px, transparent 5px) 0 5px / 20px 20px,
			linear-gradient(207deg, rgb(0 0 0 / 0.18) 5px, transparent 5px) 10px 0 / 20px 20px,
			linear-gradient(27deg, rgb(255 255 255 / 0.03) 5px, transparent 5px) 0 10px / 20px 20px,
			linear-gradient(207deg, rgb(255 255 255 / 0.03) 5px, transparent 5px) 10px 5px / 20px 20px,
			linear-gradient(90deg, rgb(0 0 0 / 0.06) 10px, transparent 10px) 0 0 / 20px 20px, var(--base);
	}
	/* 9 Paper grain, warm */
	.backdrop[data-bg='9'] { background: linear-gradient(180deg, color-mix(in srgb, #c9a46a 7%, var(--base)), var(--base)); }
	.backdrop[data-bg='9']::after {
		opacity: 0.7;
		background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='160' height='160'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='.75' numOctaves='3' stitchTiles='stitch'/%3E%3CfeColorMatrix values='0 0 0 0 .5 0 0 0 0 .45 0 0 0 0 .4 0 0 0 .16 0'/%3E%3C/filter%3E%3Crect width='160' height='160' filter='url(%23n)'/%3E%3C/svg%3E");
	}
	/* 10 Starfield */
	.backdrop[data-bg='10'] {
		background:
			radial-gradient(1px 1px at 10% 20%, color-mix(in srgb, var(--text) 50%, transparent), transparent),
			radial-gradient(1px 1px at 30% 70%, color-mix(in srgb, var(--text) 35%, transparent), transparent),
			radial-gradient(1.5px 1.5px at 55% 35%, color-mix(in srgb, var(--text) 45%, transparent), transparent),
			radial-gradient(1px 1px at 75% 80%, color-mix(in srgb, var(--text) 30%, transparent), transparent),
			radial-gradient(1px 1px at 90% 15%, color-mix(in srgb, var(--text) 40%, transparent), transparent),
			radial-gradient(60% 50% at 70% 0%, color-mix(in srgb, #3b5bff 12%, transparent), transparent 70%), var(--base);
		background-size: 220px 220px, 260px 260px, 300px 300px, 240px 240px, 280px 280px, 100% 100%;
	}

	/* Section surfaces (?surface=1..10); 1 is the raised plate the sections draw themselves. */
	/* 2 Flat hairline */
	.status[data-surface='2'] :global(section) { background: color-mix(in srgb, var(--surface-1) 70%, transparent); box-shadow: none; backdrop-filter: none; border-radius: 10px; }
	/* 3 Frosted glass */
	.status[data-surface='3'] :global(section) {
		background: color-mix(in srgb, var(--surface-1) 45%, transparent); backdrop-filter: blur(28px) saturate(1.6);
		border-color: color-mix(in srgb, #fff 14%, transparent); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.18), 0 20px 50px -30px rgb(0 0 0 / 0.6);
	}
	/* 4 Sunken well */
	.status[data-surface='4'] :global(section) {
		background: color-mix(in srgb, #000 14%, var(--surface-1)); border-color: transparent;
		box-shadow: inset 0 2px 10px rgb(0 0 0 / 0.30), inset 0 0 0 1px rgb(0 0 0 / 0.20), 0 1px 0 rgb(255 255 255 / 0.06);
	}
	/* 5 Soft relief */
	.status[data-surface='5'] :global(section) {
		background: var(--surface-1); border-color: transparent; border-radius: 26px;
		box-shadow: 10px 10px 24px rgb(0 0 0 / 0.18), -8px -8px 20px rgb(255 255 255 / 0.04), inset 0 1px 0 rgb(255 255 255 / 0.06);
	}
	/* 6 Solid slab with accent strip */
	.status[data-surface='6'] :global(section) {
		background: var(--surface-1); border-radius: 12px; border-top: 3px solid var(--accent, var(--text)); backdrop-filter: none;
		box-shadow: 0 2px 0 rgb(0 0 0 / 0.10), 0 16px 30px -22px rgb(0 0 0 / 0.5);
	}
	/* 7 Corner brackets */
	.status[data-surface='7'] :global(section) {
		border-color: transparent; border-radius: 4px; box-shadow: none;
		background:
			linear-gradient(var(--text-muted), var(--text-muted)) top left / 18px 2px no-repeat,
			linear-gradient(var(--text-muted), var(--text-muted)) top left / 2px 18px no-repeat,
			linear-gradient(var(--text-muted), var(--text-muted)) top right / 18px 2px no-repeat,
			linear-gradient(var(--text-muted), var(--text-muted)) top right / 2px 18px no-repeat,
			linear-gradient(var(--text-muted), var(--text-muted)) bottom left / 18px 2px no-repeat,
			linear-gradient(var(--text-muted), var(--text-muted)) bottom left / 2px 18px no-repeat,
			linear-gradient(var(--text-muted), var(--text-muted)) bottom right / 18px 2px no-repeat,
			linear-gradient(var(--text-muted), var(--text-muted)) bottom right / 2px 18px no-repeat,
			color-mix(in srgb, var(--surface-1) 75%, transparent);
	}
	/* 8 Gradient edge */
	.status[data-surface='8'] :global(section) {
		border: 1px solid transparent; border-radius: 18px; backdrop-filter: none;
		background: linear-gradient(var(--surface-1), var(--surface-1)) padding-box, linear-gradient(135deg, #6b8cff, #b45cff 40%, #22c3a6) border-box;
		box-shadow: 0 18px 40px -26px color-mix(in srgb, #6b8cff 60%, transparent);
	}
	/* 9 Paper card */
	.status[data-surface='9'] :global(section) {
		border-radius: 6px; border-color: color-mix(in srgb, #c9a46a 25%, var(--line)); backdrop-filter: none;
		background: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='120' height='120'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='.9' numOctaves='2'/%3E%3CfeColorMatrix values='0 0 0 0 .5 0 0 0 0 .45 0 0 0 0 .4 0 0 0 .12 0'/%3E%3C/filter%3E%3Crect width='120' height='120' filter='url(%23n)'/%3E%3C/svg%3E"), color-mix(in srgb, #c9a46a 5%, var(--surface-1));
		box-shadow: 4px 4px 0 color-mix(in srgb, #000 12%, transparent);
	}
	/* 10 Technical dashed */
	.status[data-surface='10'] :global(section) { background: transparent; border: 1px dashed var(--line-strong); border-radius: 2px; box-shadow: none; backdrop-filter: none; }
	.status[data-surface='10'] :global(section h2) { font-family: var(--font-mono, ui-monospace, monospace); font-size: 14px; letter-spacing: 0.1em; text-transform: uppercase; border-bottom-style: dashed; }

	/* Retro set, in HolyArrs colours. */
	@font-face { font-family: 'HolyArrs Pixel'; src: url('/fonts/FK-Raster-Grotesk-Compact-Blended.woff2') format('woff2'); font-display: swap; }
	/* 11 CRT: scanlines and a soft glow at the edges */
	.backdrop[data-bg='11'] {
		background:
			repeating-linear-gradient(180deg, rgb(0 0 0 / 0.22) 0 1px, transparent 1px 3px),
			radial-gradient(90% 70% at 50% 40%, color-mix(in srgb, var(--acc) 10%, transparent), transparent 75%), var(--base);
	}
	.backdrop[data-bg='11']::after { background: radial-gradient(130% 100% at 50% 50%, transparent 60%, rgb(0 0 0 / 0.55)); }
	/* 12 Pixel grid: chunky checker, faded */
	.backdrop[data-bg='12'] {
		background: conic-gradient(color-mix(in srgb, var(--text) 5%, transparent) 25%, transparent 0 50%, color-mix(in srgb, var(--text) 5%, transparent) 0 75%, transparent 0) 0 0 / 16px 16px, var(--base);
	}
	.backdrop[data-bg='12']::after { background: linear-gradient(180deg, transparent 20%, var(--base) 90%); }
	/* 13 Horizon: sun and floor grid */
	.backdrop[data-bg='13'] {
		background:
			radial-gradient(circle at 50% 108%, color-mix(in srgb, var(--acc) 38%, transparent) 0 14%, transparent 14.2%),
			linear-gradient(180deg, var(--base) 55%, color-mix(in srgb, var(--acc) 14%, var(--base)) 100%);
	}
	.backdrop[data-bg='13']::after {
		top: 62%;
		background:
			linear-gradient(color-mix(in srgb, var(--acc) 40%, transparent) 1px, transparent 1px) 0 0 / 100% 28px,
			linear-gradient(90deg, color-mix(in srgb, var(--acc) 40%, transparent) 1px, transparent 1px) 50% 0 / 60px 100%;
		transform: perspective(300px) rotateX(60deg); transform-origin: top; opacity: 0.6;
		-webkit-mask-image: linear-gradient(transparent, #000 40%); mask-image: linear-gradient(transparent, #000 40%);
	}
	/* 11 Window: hard edge, solid offset shadow, title strip */
	.status[data-surface='11'] :global(section) {
		border-radius: 0; border: 2px solid var(--text); backdrop-filter: none; background: var(--surface-1);
		box-shadow: 6px 6px 0 color-mix(in srgb, var(--accent, var(--text)) 70%, transparent);
	}
	.status[data-surface='11'] :global(section h2) { margin-top: -18px; padding-top: 10px; background: color-mix(in srgb, var(--accent, var(--text)) 16%, transparent); border-bottom: 2px solid var(--text); box-shadow: none; }
	/* 12 Bevel: raised 3D edges like old desktop panels, dark */
	.status[data-surface='12'] :global(section) {
		border-radius: 2px; border: 0; backdrop-filter: none; background: var(--surface-1);
		box-shadow: inset 2px 2px 0 rgb(255 255 255 / 0.16), inset -2px -2px 0 rgb(0 0 0 / 0.45), 0 0 0 1px rgb(0 0 0 / 0.5);
	}
	.status[data-surface='12'] :global(section h2) { border-bottom: 0; box-shadow: inset 0 -2px 0 rgb(0 0 0 / 0.35), 0 2px 0 rgb(255 255 255 / 0.06); }
	/* 13 Console: dark screen, glowing rule lines */
	.status[data-surface='13'] :global(section) {
		border-radius: 6px; backdrop-filter: none; border: 1px solid color-mix(in srgb, var(--accent, #6b8cff) 45%, transparent);
		background: repeating-linear-gradient(180deg, rgb(255 255 255 / 0.02) 0 1px, transparent 1px 3px), #0b0d10;
		box-shadow: 0 0 0 1px rgb(0 0 0 / 0.6), 0 0 24px -8px color-mix(in srgb, var(--accent, #6b8cff) 50%, transparent), inset 0 0 30px rgb(0 0 0 / 0.6);
		color: #e8ebf0;
	}
	.status[data-surface='13'] :global(section h2) { color: #f2f4f7; border-bottom-color: color-mix(in srgb, var(--accent, #6b8cff) 35%, transparent); }
	/* Pixel headings (?pixel=1): the site font on titles, tabs and big numbers only */
	.status[data-pixel] :global(:is(h1, section h2, [role='tab'], .val, .sv, .gv, .rv)) {
		font-family: 'HolyArrs Pixel', var(--font-ui); letter-spacing: 0.16em; font-weight: 400; text-transform: uppercase;
	}
	.status[data-pixel] :global(section h2) { padding-left: 24px; padding-right: 24px; }
	.status[data-pixel] h1 { font-size: 30px; }

	.status {
		max-width: 1600px; margin: 0 auto; padding: var(--topbar, 0px) var(--gutter) calc(150px + env(safe-area-inset-bottom, 0px)); box-sizing: border-box;
		display: flex; flex-direction: column; gap: 16px;
	}
	.sbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 14px 0; }
	h1 { margin: 0; font: 600 26px/1.15 var(--font-ui); color: var(--text); }
	.head { margin: 2px 0 0; font-size: 14px; }
	.head.good { color: var(--text-muted); }
	.head.bad { color: var(--fail-text); font-weight: 500; }
	.muted { margin: 0; color: var(--text-muted); font-size: 14px; }
	.notice { color: var(--fail-text); }
	.btn {
		min-height: 44px; padding: 0 16px; border: 1px solid var(--line); border-radius: var(--r-pill);
		background: color-mix(in srgb, var(--surface-1) 72%, transparent); color: var(--text); font: 500 14px/1 var(--font-ui); cursor: pointer;
	}
	.btn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.btn:disabled { opacity: 0.6; }
	@media (max-width: 767px) { h1 { font-size: 22px; } }
</style>
