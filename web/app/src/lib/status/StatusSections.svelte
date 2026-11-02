<script lang="ts">
	// The Sections look, laid out the way the *arr status pages are: one titled
	// section after another. Status tab: Health (slim problem list), Parts (a
	// deck of cards), Disk space, About and Release. Jobs tab: one slim table.
	import { onMount } from 'svelte';
	import { api } from '../api.ts';
	import { ago, bytes, dateTime } from '../format.ts';
	import { until } from '../schedules.ts';
	import { to } from '../paths.ts';
	import { groupCounts, needsAttention, type StatusGroup, type StatusRow } from './status.ts';

	let {
		groups,
		tab,
		ontab,
		canAct,
		busy,
		oncheck,
		labelOf
	}: {
		groups: StatusGroup[];
		tab: 'parts' | 'jobs';
		ontab: (t: 'parts' | 'jobs') => void;
		canAct: boolean;
		busy: string | null;
		oncheck: (r: StatusRow) => void;
		labelOf: (r: StatusRow) => string;
	} = $props();

	type Path = { name: string; path: string; bytes?: number; free_bytes: number; kind?: string };
	type Update = { on: boolean; version: string; latest?: string; newer: boolean; link?: string; notes?: string; checked_at?: string; problem?: string; no_release?: boolean };

	let store = $state<{ app: Path[]; library_folders: Path[] } | null>(null);
	let upd = $state<Update | null>(null);
	let notesOpen = $state(false);

	const parts = $derived(groups.filter((g) => g.id !== 'jobs'));
	const jobs = $derived(groups.find((g) => g.id === 'jobs')?.rows ?? []);
	const problems = $derived(needsAttention(groups.flatMap((g) => g.rows)));
	// Disks: the config folder plus every library folder, each once.
	const disks = $derived.by(() => {
		if (!store) return [];
		const cfg = store.app.find((p) => p.name === 'Config folder');
		return [...(cfg ? [cfg] : []), ...store.library_folders];
	});
	// Without storage (it asks for a fresh password), fall back to the free-space checks.
	const diskChecks = $derived(groups.flatMap((g) => g.rows).filter((r) => r.key.startsWith('disk_space')));
	const dbBytes = $derived((store?.app ?? []).filter((p) => p.name.endsWith('database')).reduce((n, p) => n + (p.bytes ?? 0), 0));
	const libCount = $derived(store?.library_folders.length ?? 0);

	const icon: Record<string, string> = {
		sources: 'M4 6h12M4 10h12M4 14h8',
		downloaders: 'M10 3v10m0 0-4-4m4 4 4-4M4 16h12',
		library: 'M3 6l2-2h4l2 2h6v9H3z',
		media: 'M3 4h14v9H3zM7 16h6',
		subtitles: 'M3 5h14v10H3zM6 11h3m2 0h3',
		lists: 'M6 5h10M6 10h10M6 15h10M3 5h.01M3 10h.01M3 15h.01',
		alerts: 'M10 3a5 5 0 0 0-5 5v3l-2 3h14l-2-3V8a5 5 0 0 0-5-5zM8 16a2 2 0 0 0 4 0',
		metadata: 'M10 3c4 0 7 1.3 7 3s-3 3-7 3-7-1.3-7-3 3-3 7-3zM3 6v8c0 1.7 3 3 7 3s7-1.3 7-3V6'
	};

	onMount(() => {
		api.get<{ app: Path[]; library_folders: Path[] }>('/app/storage').then((s) => (store = s), () => {});
		api.get<Update>('/app/update').then((u) => (upd = u), () => {});
	});

	function cardLine(g: StatusGroup) {
		const c = groupCounts(g);
		if (c.bad) return `${c.bad} need${c.bad === 1 ? 's' : ''} attention`;
		if (g.rows.length === 1) return g.rows[0].name;
		return c.off ? `${c.off} turned off` : 'All working';
	}
</script>

<div class="secs" data-testid="status-sections">
	<div class="seg" role="tablist" aria-label="Status view">
		<button type="button" role="tab" aria-selected={tab === 'parts'} onclick={() => ontab('parts')}>Status</button>
		<button type="button" role="tab" aria-selected={tab === 'jobs'} onclick={() => ontab('jobs')}>
			Jobs{#if jobs.some((j) => j.tone === 'fail')}<i class="badge" aria-label="has failures"></i>{/if}
		</button>
	</div>

	{#if tab === 'parts'}
		<section aria-labelledby="s-health">
			<h2 id="s-health">Health</h2>
			{#if problems.length}
				<ul class="health" data-testid="status-attention">
					{#each problems as r (r.key)}
						<li class="hrow" data-testid="status-row" data-tone={r.tone} data-key={r.key}>
							<span class="hico {r.tone}" aria-hidden="true">!</span>
							<span class="hmsg">
								<b>{r.name}</b> <span class="hk">· {labelOf(r)} · {r.word}</span>
								{#if r.reason}<span class="hr">{r.reason}</span>{/if}
							</span>
							{#if r.at}<time class="hat" datetime={r.at} title={dateTime(r.at)}>{ago(r.at)}</time>{/if}
							{#if canAct}
								<span class="hacts">
									{#if r.check}<button type="button" class="lnk" disabled={busy === r.key} onclick={() => oncheck(r)}>{r.group === 'jobs' ? 'Run now' : 'Check now'}</button>{/if}
									{#if r.fix}<a class="lnk" href={to(r.fix)}>Fix</a>{/if}
								</span>
							{/if}
						</li>
					{/each}
				</ul>
			{:else}
				<p class="ok-line"><span class="okdot" aria-hidden="true"></span>No problems. Every part and job is working.</p>
			{/if}
		</section>

		<section aria-labelledby="s-parts">
			<h2 id="s-parts">Parts</h2>
			<ul class="deck">
				{#each parts as g (g.id)}
					{@const c = groupCounts(g)}
					<li class="card {c.bad ? 'fail' : c.ok ? 'ok' : 'idle'}">
						<div class="ch">
							<span class="ct">{g.label}</span>
							<span class="ci" aria-hidden="true"><svg viewBox="0 0 20 20"><path d={icon[g.id] ?? icon.lists} fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" /></svg></span>
						</div>
						<p class="cv">{c.ok}<span>/{g.rows.length}</span></p>
						<p class="cs">{cardLine(g)}</p>
						<ul class="cl">
							{#each g.rows.slice(0, 4) as r (r.key)}
								<li data-testid="status-row" data-tone={r.tone} data-key={r.key}><i class="d {r.tone}" aria-hidden="true"></i><span>{r.name}</span></li>
							{/each}
							{#if g.rows.length > 4}<li class="more">+{g.rows.length - 4} more</li>{/if}
						</ul>
					</li>
				{/each}
			</ul>
		</section>

		{#if disks.length || diskChecks.length}
			<section aria-labelledby="s-disk">
				<h2 id="s-disk">Disk space</h2>
				<table class="tbl">
					<thead><tr><th>Location</th><th>Used for</th><th class="num">Free</th></tr></thead>
					<tbody>
						{#if !disks.length}
							{#each diskChecks as r (r.key)}
								<tr>
									<td class="mono">{r.name.replace(/^Free space /, '')}</td>
									<td>Library</td>
									<td class="num"><span class="st {r.tone}"><i class="d {r.tone}" aria-hidden="true"></i>{r.tone === 'ok' ? 'Enough' : r.reason || r.word}</span></td>
								</tr>
							{/each}
						{/if}
						{#each disks as d (d.path)}
							<tr>
								<td class="mono">{d.path}</td>
								<td>{d.name || (d.kind === 'movie' ? 'Movies' : d.kind === 'show' ? 'Shows' : d.kind || 'Library')}</td>
								<td class="num">{d.free_bytes >= 0 ? bytes(d.free_bytes) : 'Unknown'}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</section>
		{/if}

		<section aria-labelledby="s-about">
			<h2 id="s-about">About</h2>
			<dl class="kv">
				<dt>Version</dt>
				<dd>
					<code>{upd?.version ?? '…'}</code>
					{#if upd?.newer}<span class="pill warn">Update available</span>{:else if upd?.on && upd.latest}<span class="pill good">Up to date</span>{/if}
				</dd>
				{#if store}
					<dt>Config folder</dt><dd><code>{store.app.find((p) => p.name === 'Config folder')?.path ?? ''}</code></dd>
					<dt>Databases</dt><dd>{bytes(dbBytes)}</dd>
				{/if}
				<dt>Library folders</dt><dd>{libCount}</dd>
				<dt>Linked parts</dt><dd>{parts.reduce((n, g) => n + g.rows.length, 0)}</dd>
				<dt>Background jobs</dt><dd>{jobs.length}</dd>
			</dl>
		</section>

		{#if upd}
			<section aria-labelledby="s-rel">
				<h2 id="s-rel">Release</h2>
				<div class="rel">
					<div class="rh">
						<span class="rv">{upd.newer && upd.latest ? upd.latest : upd.version}</span>
						{#if upd.newer}<span class="pill warn">Newer than yours</span>{:else}<span class="pill good">Running</span>{/if}
						<span class="rsub">
							{#if !upd.on}Update check is off
							{:else if upd.problem}{upd.problem}
							{:else if upd.no_release}Nothing published yet
							{:else if upd.checked_at}Checked {ago(upd.checked_at)}{/if}
						</span>
						<span class="ract">
							{#if upd.notes}<button type="button" class="btn" aria-expanded={notesOpen} onclick={() => (notesOpen = !notesOpen)}>{notesOpen ? 'Hide notes' : 'Show notes'}</button>{/if}
							{#if upd.link}<a class="btn" href={upd.link} target="_blank" rel="noopener noreferrer">Release page</a>{/if}
						</span>
					</div>
					{#if notesOpen && upd.notes}<pre class="notes">{upd.notes}</pre>{/if}
				</div>
			</section>
		{/if}
	{:else}
		<section aria-labelledby="s-jobs">
			<h2 id="s-jobs">Jobs</h2>
			<table class="tbl jobs">
				<thead><tr><th>Name</th><th>State</th><th>Last run</th><th>Next run</th>{#if canAct}<th><span class="sr">Actions</span></th>{/if}</tr></thead>
				<tbody>
					{#each jobs as r (r.key)}
						<tr data-testid="status-row" data-tone={r.tone} data-key={r.key}>
							<td class="jn">{r.name}{#if r.tone === 'fail' && r.reason}<span class="jr">{r.reason}</span>{/if}</td>
							<td><span class="st {r.tone}"><i class="d {r.tone}" aria-hidden="true"></i>{r.word}</span></td>
							<td>{#if r.lastRun}<time datetime={r.lastRun} title={dateTime(r.lastRun)}>{ago(r.lastRun)}</time>{:else}Never{/if}</td>
							<td>{until(r.nextRun)}</td>
							{#if canAct}<td class="num"><button type="button" class="lnk" disabled={busy === r.key} onclick={() => oncheck(r)}>Run now</button></td>{/if}
						</tr>
					{/each}
				</tbody>
			</table>
		</section>
	{/if}
</div>

<style>
	.secs { display: flex; flex-direction: column; gap: 28px; }
	ul { list-style: none; margin: 0; padding: 0; }
	h2 {
		margin: 0 0 12px; padding-bottom: 10px; border-bottom: 1px solid var(--line);
		font: 600 20px/1.2 var(--font-ui); color: var(--text);
	}
	.seg { display: inline-flex; align-self: flex-start; gap: 2px; padding: 3px; border: 1px solid var(--line); border-radius: var(--r-pill); background: color-mix(in srgb, var(--surface-1) 72%, transparent); }
	.seg button { position: relative; min-height: 40px; padding: 0 22px; border: 0; border-radius: var(--r-pill); background: none; color: var(--text-muted); font: 500 14px/1 var(--font-ui); cursor: pointer; }
	.seg button[aria-selected='true'] { background: var(--seg-thumb); color: var(--text); box-shadow: 0 0 0 1px var(--line-strong); }
	.seg button:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.badge { position: absolute; top: 8px; right: 10px; width: 7px; height: 7px; border-radius: 50%; background: var(--fail); }

	/* Health: slim, one line per problem */
	.health { border: 1px solid var(--line); border-radius: 12px; overflow: hidden; background: var(--surface-1); }
	.hrow { display: grid; grid-template-columns: 22px minmax(0, 1fr) auto auto; align-items: center; gap: 12px; min-height: 48px; padding: 6px 14px; border-top: 1px solid var(--line); }
	.hrow:first-child { border-top: 0; }
	.hico { display: grid; place-items: center; width: 20px; height: 20px; border-radius: 50%; background: var(--fail); color: #fff; font: 700 12px/1 var(--font-ui); }
	.hico.wait { background: var(--wait); }
	.hmsg { min-width: 0; font-size: 14px; color: var(--text); }
	.hk { color: var(--text-muted); }
	.hr { display: block; font-size: 13px; color: var(--text-muted); overflow-wrap: anywhere; }
	.hat { font-size: 12.5px; color: var(--text-muted); white-space: nowrap; }
	.hacts { display: flex; gap: 4px; }
	.lnk {
		display: inline-flex; align-items: center; min-height: 36px; padding: 0 10px; border: 0; border-radius: 8px; background: none;
		color: var(--accent, var(--text)); font: 500 13.5px/1 var(--font-ui); text-decoration: none; cursor: pointer; white-space: nowrap;
	}
	.lnk:hover { background: var(--surface-2, var(--surface-1)); }
	.lnk:focus-visible, .btn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.lnk:disabled { opacity: 0.5; }
	.ok-line { display: flex; align-items: center; gap: 10px; margin: 0; font-size: 14.5px; color: var(--text-muted); }
	.okdot { width: 9px; height: 9px; border-radius: 50%; background: var(--ok); }

	/* Parts deck, after ChannelWatch's metric cards */
	.deck { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(220px, 100%), 1fr)); gap: 14px; }
	.card {
		--tint: var(--ok);
		position: relative; display: flex; flex-direction: column; gap: 4px; padding: 16px 18px; border-radius: 16px;
		border: 1px solid color-mix(in srgb, var(--tint) 28%, var(--line));
		background: linear-gradient(145deg, color-mix(in srgb, var(--tint) 12%, var(--surface-1)), var(--surface-1) 70%);
	}
	.card.fail { --tint: var(--fail); }
	.card.idle { --tint: var(--text-faint); }
	.ch { display: flex; align-items: center; justify-content: space-between; }
	.ct { font-size: 13.5px; font-weight: 500; color: var(--text); }
	.ci { display: grid; place-items: center; width: 28px; height: 28px; border-radius: 50%; background: color-mix(in srgb, var(--tint) 18%, transparent); color: var(--tint); }
	.ci svg { width: 16px; height: 16px; }
	.cv { margin: 6px 0 0; font: 700 32px/1 var(--font-ui); color: var(--text); font-variant-numeric: tabular-nums; }
	.cv span { font-size: 17px; font-weight: 500; color: var(--text-muted); }
	.cs { margin: 0; font-size: 12.5px; color: var(--text-muted); }
	.card.fail .cs { color: var(--fail-text); }
	.cl { display: flex; flex-direction: column; gap: 4px; margin-top: 10px; padding-top: 10px; border-top: 1px solid color-mix(in srgb, var(--tint) 18%, var(--line)); }
	.cl li { display: flex; align-items: center; gap: 8px; font-size: 13px; color: var(--text); min-width: 0; }
	.cl li span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.cl .more { color: var(--text-muted); padding-left: 15px; }
	.d { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--info); }
	.d.ok { background: var(--ok); }
	.d.wait { background: var(--wait); }
	.d.fail { background: var(--fail); }
	.d.idle { background: var(--text-faint); }

	/* Tables */
	.tbl { width: 100%; border-collapse: collapse; font-size: 14px; }
	.tbl th { padding: 8px 12px; text-align: left; font-weight: 600; font-size: 13px; color: var(--text-muted); border-bottom: 1px solid var(--line); }
	.tbl td { padding: 10px 12px; border-bottom: 1px solid var(--line); color: var(--text); vertical-align: middle; }
	.tbl .num { text-align: right; white-space: nowrap; font-variant-numeric: tabular-nums; }
	.mono, code { font-family: var(--font-mono, ui-monospace, monospace); font-size: 13px; overflow-wrap: anywhere; }
	code { padding: 3px 7px; border-radius: 6px; background: var(--surface-2, var(--surface-1)); }
	.jn { font-weight: 500; }
	.jr { display: block; font-weight: 400; font-size: 12.5px; color: var(--fail-text); }
	.st { display: inline-flex; align-items: center; gap: 7px; white-space: nowrap; }
	.st.fail { color: var(--fail-text); font-weight: 500; }
	.st.idle { color: var(--text-muted); }
	.sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); }

	/* About */
	.kv { display: grid; grid-template-columns: 180px minmax(0, 1fr); margin: 0; }
	.kv dt, .kv dd { margin: 0; padding: 11px 0; border-bottom: 1px solid var(--line); font-size: 14px; }
	.kv dt { font-weight: 600; color: var(--text-muted); }
	.kv dd { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; color: var(--text); }
	.pill { padding: 3px 9px; border-radius: var(--r-pill); font-size: 12px; font-weight: 600; }
	.pill.good { background: color-mix(in srgb, var(--ok) 18%, transparent); color: var(--ok-text, var(--text)); }
	.pill.warn { background: color-mix(in srgb, var(--wait) 22%, transparent); color: var(--text); }

	/* Release */
	.rel { border: 1px solid var(--line); border-radius: 12px; background: var(--surface-1); }
	.rh { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; padding: 12px 14px; }
	.rv { font: 700 18px/1 var(--font-ui); color: var(--text); }
	.rsub { font-size: 13px; color: var(--text-muted); }
	.ract { display: flex; gap: 8px; margin-left: auto; }
	.btn {
		display: inline-flex; align-items: center; min-height: 40px; padding: 0 14px; border: 1px solid var(--line); border-radius: var(--r-pill);
		background: var(--surface-2, var(--surface-1)); color: var(--text); font: 500 13.5px/1 var(--font-ui); text-decoration: none; cursor: pointer;
	}
	.notes { margin: 0; padding: 12px 14px; border-top: 1px solid var(--line); white-space: pre-wrap; font: 13.5px/1.55 var(--font-ui); color: var(--text); max-height: 320px; overflow: auto; }

	@media (max-width: 767px) {
		.hrow { grid-template-columns: 22px minmax(0, 1fr); }
		.hat { grid-column: 2; }
		.hacts { grid-column: 2; margin-left: -10px; }
		.deck { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
		.card { padding: 12px; }
		.cl { display: none; }
		.kv { grid-template-columns: 130px minmax(0, 1fr); }
		.tbl.jobs thead { display: none; }
		.tbl.jobs tr { display: grid; grid-template-columns: minmax(0, 1fr) auto; padding: 8px 0; border-bottom: 1px solid var(--line); }
		.tbl.jobs td { border: 0; padding: 2px 4px; font-size: 13px; color: var(--text-muted); }
		.tbl.jobs td.jn { font-size: 14px; color: var(--text); }
		.tbl.jobs td.num { grid-column: 2; grid-row: 1 / span 2; align-self: center; }
		.ract { margin-left: 0; }
	}
</style>
