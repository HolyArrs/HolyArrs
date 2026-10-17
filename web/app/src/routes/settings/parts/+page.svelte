<script lang="ts">
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { ago } from '#lib/format.ts';
	import { live } from '#lib/live.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { refreshParts } from '#lib/parts.svelte.ts';
	import { partNeeds, partStops, type PartName, type PartsAnswer, type PartView } from '#lib/parts.ts';
	import { isConfirm, linkAction, linkTarget, type Confirm } from '#lib/controls.ts';
	import { linkLabel, runTone, runWord, splitStates } from '#lib/health.ts';
	import type { EventRecord, HealthStates, StageState } from '#lib/types.ts';

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
	// The whole app stopped; nothing more can be done from this page.
	let gone = $state('');

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
		const t = setInterval(load, 15000);
		const offs = [live.on('health', load)];
		return () => {
			clearInterval(t);
			offs.forEach((off) => off());
		};
	});

	/** run sends a control; one that asks first waits for the second step on this page. */
	async function run(key: string, label: string, path: string, then: (r: unknown) => void) {
		busy = key;
		try {
			const r = await api.post<unknown>(path, {});
			if (isConfirm(r)) asking = { key, label, path, confirm: r, then };
			else then(r);
		} catch (err) {
			toasts.push(message(err), 'fail');
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
			toasts.push(message(err), 'fail');
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
		run(`part-${p.part}`, on ? `Turn ${p.name} on` : `Turn ${p.name} off`, `/parts/${p.part}/${on ? 'on' : 'off'}`, done(`${p.name} is ${on ? 'on' : 'off'}.`));
	}

	function restartPart(p: PartView) {
		run(`restart-${p.part}`, `Restart ${p.name}`, `/parts/${p.part}/restart`, done(`${p.name} restarted.`));
	}

	function stage(s: StageState, action: 'pause' | 'resume' | 'restart') {
		const name = s.key.replace(/^stage:/, '');
		run(`stage-${name}-${action}`, `${action} ${s.name}`, `/stages/${encodeURIComponent(name)}/${action}`, done(`${s.name}: done.`));
	}

	function reconnect(l: StageState) {
		const t = linkTarget(l.key);
		if (!t) return;
		run(`link-${l.key}`, `${linkAction[t.kind]} ${l.name}`, `/links/${t.kind}/${encodeURIComponent(t.id)}/restart`, (r) => {
			const o = (r ?? {}) as { ok?: boolean; text?: string; message?: string; error?: string };
			toasts.push(`${l.name}: ${o.text || o.message || o.error || 'answered.'}`, o.ok === false ? 'fail' : 'ok');
			void load();
		});
	}

	function app(how: 'restart' | 'safe-mode' | 'shutdown', label: string) {
		run(`app-${how}`, label, `/app/${how}`, (r) => {
			gone = (r as { text?: string })?.text ?? 'HolyArrs is stopping.';
		});
	}

	const paused = (s: StageState) => s.state === 'off';
	const pausable = (s: StageState) => !!parts?.stages?.includes(s.key.replace(/^stage:/, ''));

	// The second step takes the focus (and scrolls into view) when it shows.
	$effect(() => {
		if (asking) queueMicrotask(() => document.getElementById('confirm-yes')?.focus());
	});
</script>

<svelte:head><title>Parts and controls · HolyArrs</title></svelte:head>

<header class="page-head">
	<div>
		<h1>Parts and controls</h1>
		<p>Turn parts on and off, restart one piece without stopping the rest, and restart HolyArrs.</p>
	</div>
</header>

{#if !allowed}
	<p class="notice warn">Only the owner and admins can use these controls.</p>
{:else if gone}
	<p class="notice warn" role="status" data-testid="app-gone">{gone}</p>
{:else if error}
	<p class="notice bad" role="alert">{error}</p>
{:else if !parts}
	<div class="sk" style="height: 240px"></div>
{:else}
	{#if safeMode}
		<p class="notice warn" data-testid="parts-safe-mode">
			HolyArrs runs in safe mode: only the core runs, and part switches and stage controls are refused until a normal start. Use
			<b>Restart HolyArrs</b> below to leave safe mode.
		</p>
	{/if}

	{#if asking}
		<div class="card confirm" role="alertdialog" aria-labelledby="h-confirm" aria-describedby="confirm-text" data-testid="confirm">
			<h2 id="h-confirm" class="h2">{asking.label}?</h2>
			<p id="confirm-text" class="small">{asking.confirm.text.replace(/, send the same request again.*$/, '.')}</p>
			<div class="row">
				<button id="confirm-yes" class="btn btn-danger" type="button" onclick={confirmNow} disabled={busy === asking.key} aria-busy={busy === asking.key}>Yes, {asking.label.toLowerCase()}</button>
				<button class="btn btn-quiet" type="button" onclick={() => (asking = null)}>Cancel</button>
			</div>
			<p class="xs muted">This question lasts 60 seconds.</p>
		</div>
	{/if}

	<section class="card" aria-labelledby="h-parts" data-testid="parts">
		<h2 id="h-parts" class="h2">Parts</h2>
		<p class="small muted">A part that is off hides its screens and stops its work. Its data is kept, and turning it on brings everything back.</p>
		<ul class="rows list">
			{#each parts.parts as p (p.part)}
				<li data-testid="part" data-part={p.part} data-state={p.state}>
					<div class="grow">
						<div class="name">
							{p.name}
							<span class="chip {runTone(p.state)}">{runWord({ state: p.state, stuck: false })}</span>
						</div>
						<p class="xs muted">When off: {partStops[p.part as PartName]}</p>
						{#if partNeeds[p.part as PartName]}<p class="xs muted">{partNeeds[p.part as PartName]}</p>{/if}
						{#if p.reason}<p class="xs muted">{p.reason}</p>{/if}
					</div>
					<div class="row acts">
						{#if p.on}
							<button class="btn btn-ghost" type="button" onclick={() => restartPart(p)} disabled={!!busy} aria-label="Restart {p.name}">Restart</button>
							<button class="btn btn-quiet" type="button" onclick={() => switchPart(p, false)} disabled={!!busy} aria-label="Turn {p.name} off">Turn off</button>
						{:else}
							<button class="btn btn-red" type="button" onclick={() => switchPart(p, true)} disabled={!!busy} aria-label="Turn {p.name} on">Turn on</button>
						{/if}
					</div>
				</li>
			{/each}
		</ul>
	</section>

	<section class="card" aria-labelledby="h-stages" data-testid="control-stages">
		<h2 id="h-stages" class="h2">Stages</h2>
		<p class="small muted">Background work such as feed checks and imports. Pause one to hold it; restart one that seems stuck. The rest keeps running.</p>
		{#if split.stages.length === 0}
			<p class="small muted">No stages reported.</p>
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
								<button class="btn btn-ghost" type="button" onclick={() => stage(s, 'resume')} disabled={!!busy} aria-label="Resume {s.name}">Resume</button>
							{:else}
								<button class="btn btn-quiet" type="button" onclick={() => stage(s, 'pause')} disabled={!!busy} aria-label="Pause {s.name}">Pause</button>
							{/if}
							<button class="btn btn-quiet" type="button" onclick={() => stage(s, 'restart')} disabled={!!busy} aria-label="Restart {s.name}">Restart</button>
						</div>{:else}<span class="xs muted">Restarts with the whole app</span>{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</section>

	<section class="card" aria-labelledby="h-links" data-testid="control-links">
		<h2 id="h-links" class="h2">Linked apps</h2>
		<p class="small muted">Reconnect one Downloader, or test one Source, media server or subtitle site now. Nothing else is interrupted.</p>
		{#if split.links.length === 0}
			<p class="small muted">No Sources, Downloaders, subtitle sites or media servers yet.</p>
		{:else}
			<ul class="rows list">
				{#each split.links as l (l.key)}
					<li data-testid="control-link" data-state={l.state}>
						<div class="grow">
							<div class="name">{l.name} <span class="xs muted">{linkLabel(l.kind)}</span> <span class="chip {runTone(l.state)}">{runWord(l)}</span></div>
							{#if l.reason}<p class="xs muted">{l.reason}</p>{/if}
						</div>
						{#if linkTarget(l.key)}
							<button class="btn btn-ghost" type="button" onclick={() => reconnect(l)} disabled={!!busy} aria-label="{linkAction[l.kind]} {l.name}"
								>{linkAction[l.kind]}</button
							>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</section>

	<section class="card" aria-labelledby="h-app" data-testid="control-app">
		<h2 id="h-app" class="h2">The whole app</h2>
		<p class="small muted">
			These stop HolyArrs. Docker's restart policy starts it again in a few seconds, unless the policy is <code>no</code>; then start the
			container yourself. Each one asks first.
		</p>
		<div class="row acts" style="margin-top: var(--s-3)">
			<button class="btn btn-ghost" type="button" onclick={() => app('restart', 'Restart HolyArrs')} disabled={!!busy}>Restart HolyArrs</button>
			<button class="btn btn-ghost" type="button" onclick={() => app('safe-mode', 'Restart in safe mode')} disabled={!!busy}>Restart in safe mode</button>
			<button class="btn btn-danger" type="button" onclick={() => app('shutdown', 'Shut down')} disabled={!!busy}>Shut down</button>
		</div>
		<p class="xs muted">Safe mode starts only the core, once: sign-in, settings and the Library list. Use it when a part keeps breaking the start.</p>
	</section>

	<section class="card" aria-labelledby="h-used" data-testid="control-events">
		<h2 id="h-used" class="h2">Controls used</h2>
		{#if events.length === 0}
			<p class="small muted">No controls used yet.</p>
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
	</section>
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
	.confirm {
		border-color: var(--ha-red);
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
		.list .acts > :global(.btn) {
			min-height: var(--tap);
		}
	}
	code {
		font-family: var(--font-mono);
	}
</style>
