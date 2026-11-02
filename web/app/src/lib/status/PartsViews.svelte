<script lang="ts">
	// The parts on the Status tab in five preview shapes (?parts=). Every kind
	// of part always has its seat, empty or not, so the layout keeps one shape
	// however many parts are linked:
	//   1 Lattice      two columns, pairs share a height; the metadata server is a strip above
	//   2 Tiles        eight equal tiles, one per kind plus Storage, counts and a dot per part
	//   3 Ledger       one plate, a band per kind
	//   4 Rail         kinds down the left, the picked kind's parts on the right
	//   5 Three across three columns, flat compact rows
	import { ago, dateTime } from '../format.ts';
	import { to } from '../paths.ts';
	import GroupRows from './GroupRows.svelte';
	import { partKinds, type StatusGroup, type StatusRow, type Tone } from './status.ts';

	let {
		kinds,
		view,
		canAct,
		busy,
		oncheck,
		storageLine,
		storagePct
	}: {
		kinds: StatusGroup[];
		view: 1 | 2 | 3 | 4 | 5;
		canAct: boolean;
		busy: string | null;
		oncheck: (r: StatusRow) => void;
		/** One line about the disks, for the Storage tile: "4 folders · 84% used". */
		storageLine: string;
		/** How full the disks are, 0..100, or null when unknown. */
		storagePct: number | null;
	} = $props();

	const icon: Record<string, string> = {
		sources: 'M4 6h12M4 10h12M4 14h8',
		downloaders: 'M10 3v10m0 0-4-4m4 4 4-4M4 16h12',
		media: 'M3 4h14v9H3zM7 16h6',
		subtitles: 'M3 5h14v10H3zM6 11h3m2 0h3',
		lists: 'M6 5h10M6 10h10M6 15h10M3 5h.01M3 10h.01M3 15h.01',
		alerts: 'M10 3a5 5 0 0 0-5 5v3l-2 3h14l-2-3V8a5 5 0 0 0-5-5zM8 16a2 2 0 0 0 4 0',
		metadata: 'M10 3c4 0 7 1.3 7 3s-3 3-7 3-7-1.3-7-3 3-3 7-3zM3 6v8c0 1.7 3 3 7 3s7-1.3 7-3V6',
		storage: 'M3 6h14v4H3zM3 10h14v4H3zM6 8h.01M6 12h.01'
	};
	const meta = (g: StatusGroup) => partKinds.find((k) => k.id === g.id)!;
	const okOf = (g: StatusGroup) => g.rows.filter((r) => r.tone === 'ok').length;
	/** worst is the kind's colour: a failing part wins, then a waiting one, then working; none when empty. */
	const worst = (g: StatusGroup): Tone | 'none' =>
		g.rows.some((r) => r.tone === 'fail') ? 'fail' : g.rows.some((r) => r.tone === 'wait') ? 'wait' : g.rows.length ? (g.rows.every((r) => r.tone === 'idle') ? 'idle' : 'ok') : 'none';
	/** line says a kind in a few words: "3 working", "1 needs attention", "Off", "Nothing linked yet". */
	function line(g: StatusGroup): string {
		if (!g.rows.length) return 'Nothing linked yet';
		const bad = g.rows.filter((r) => r.tone === 'fail' || r.tone === 'wait').length;
		if (bad) return `${bad} need${bad === 1 ? 's' : ''} attention`;
		const ok = okOf(g);
		if (!ok) return g.rows.length === 1 ? g.rows[0].word : `${g.rows.length} off`;
		return g.rows.length === 1 ? g.rows[0].word : `${ok} working`;
	}
	const six = $derived(kinds.filter((k) => k.id !== 'metadata'));
	const server = $derived(kinds.find((k) => k.id === 'metadata'));
	const all = $derived(kinds.reduce((n, g) => n + g.rows.length, 0));
	const allOk = $derived(kinds.reduce((n, g) => n + okOf(g), 0));

	// Rail: the picked kind, else the first with a problem, else Sources.
	let picked = $state<string | null>(null);
	const current = $derived(kinds.find((k) => k.id === picked) ?? kinds.find((k) => worst(k) === 'fail') ?? kinds.find((k) => worst(k) === 'wait') ?? kinds[0]);
</script>

{#snippet title(g: StatusGroup)}
	<span class="gi" aria-hidden="true"><svg viewBox="0 0 20 20"><path d={icon[g.id]} /></svg></span>
	{g.label}
	<span class="cnt" class:fail={worst(g) === 'fail'} class:wait={worst(g) === 'wait'}>{g.rows.length ? `${okOf(g)}/${g.rows.length}` : '0'}</span>
{/snippet}

{#snippet empty(g: StatusGroup)}
	<p class="emp">
		<i class="d none" aria-hidden="true"></i>No {meta(g).one} linked yet.
		{#if canAct}<a class="lnk" href={to(meta(g).add)}>Add</a>{/if}
	</p>
{/snippet}

{#snippet strip(g: StatusGroup)}
	{@const r = g.rows[0]}
	<section class="st-plate strip" aria-label={g.label}>
		<span class="gi" aria-hidden="true"><svg viewBox="0 0 20 20"><path d={icon.metadata} /></svg></span>
		<b>{g.label}</b>
		{#if r}
			<i class="d {r.tone}" aria-hidden="true"></i><span class="sw {r.tone}">{r.word}</span>
			{#if r.at}<time class="at" datetime={r.at} title={dateTime(r.at)}>checked {ago(r.at)}</time>{/if}
			{#if r.reason && r.tone !== 'ok'}<span class="why">{r.reason}</span>{/if}
			{#if canAct && r.check}<button type="button" class="lnk end" disabled={busy === r.key} onclick={() => oncheck(r)}>Check now</button>{/if}
		{:else}
			<span class="sw">Not checked yet</span>
		{/if}
	</section>
{/snippet}

{#if view === 1}
	{#if server}{@render strip(server)}{/if}
	<div class="lattice">
		{#each six as g (g.id)}
			<section class="st-plate" aria-labelledby="p-{g.id}">
				<h2 id="p-{g.id}">{@render title(g)}</h2>
				{#if g.rows.length}<GroupRows rows={g.rows} {canAct} {busy} {oncheck} />{:else}{@render empty(g)}{/if}
			</section>
		{/each}
	</div>
{:else if view === 2}
	<div class="tiles">
		{#each kinds as g (g.id)}
			{@const w = worst(g)}
			<a class="tile st-plate" href={to(meta(g).add)} data-tone={w}>
				<span class="th"><span class="gi" aria-hidden="true"><svg viewBox="0 0 20 20"><path d={icon[g.id]} /></svg></span>{g.label}</span>
				<span class="big">{#if g.rows.length}{okOf(g)}<small>/{g.rows.length}</small>{:else}<small class="dash">none</small>{/if}</span>
				<span class="sub {w}">{line(g)}</span>
				<span class="dots" aria-hidden="true">{#each g.rows as r (r.key)}<i class="d {r.tone}" title={r.name}></i>{/each}</span>
			</a>
		{/each}
		<a class="tile st-plate" href="#s-disk" data-tone="ok">
			<span class="th"><span class="gi" aria-hidden="true"><svg viewBox="0 0 20 20"><path d={icon.storage} /></svg></span>Storage</span>
			<span class="big">{#if storagePct === null}<small class="dash">—</small>{:else}{storagePct}<small>% used</small>{/if}</span>
			<span class="sub" class:wait={storagePct !== null && storagePct >= 85} class:fail={storagePct !== null && storagePct >= 95}>{storageLine}</span>
		</a>
	</div>
{:else if view === 3}
	<section class="st-plate" aria-labelledby="p-all">
		<h2 id="p-all">Parts<span class="cnt">{allOk}/{all}</span></h2>
		<div class="ledger">
			{#each kinds as g (g.id)}
				<div class="band" data-tone={worst(g)}>
					<div class="bh">
						{@render title(g)}
						{#if canAct && g.rows.length}<a class="lnk end" href={to(meta(g).add)}>Manage</a>{/if}
					</div>
					{#if g.rows.length}<GroupRows rows={g.rows} {canAct} {busy} {oncheck} flat />{:else}{@render empty(g)}{/if}
				</div>
			{/each}
		</div>
	</section>
{:else if view === 4}
	<section class="st-plate rail" aria-labelledby="p-rail">
		<h2 id="p-rail">Parts<span class="cnt">{allOk}/{all}</span></h2>
		<div class="rl">
			<div class="kinds" role="tablist" aria-label="Kind of part" aria-orientation="vertical">
				{#each kinds as g (g.id)}
					{@const w = worst(g)}
					<button type="button" role="tab" aria-selected={current.id === g.id} onclick={() => (picked = g.id)}>
						<span class="gi" aria-hidden="true"><svg viewBox="0 0 20 20"><path d={icon[g.id]} /></svg></span>
						<span class="kl">{g.label}</span>
						<span class="kc">{g.rows.length ? `${okOf(g)}/${g.rows.length}` : '–'}</span>
						<i class="d {w}" aria-hidden="true"></i>
					</button>
				{/each}
			</div>
			<div class="pane" role="tabpanel">
				<h3>{current.label}<span class="pl">{line(current)}</span>{#if canAct && current.rows.length}<a class="lnk end" href={to(meta(current).add)}>Manage</a>{/if}</h3>
				{#if current.rows.length}<GroupRows rows={current.rows} {canAct} {busy} {oncheck} />{:else}{@render empty(current)}{/if}
			</div>
		</div>
	</section>
{:else}
	{#if server}{@render strip(server)}{/if}
	<div class="three">
		{#each six as g (g.id)}
			<section class="st-plate" aria-labelledby="p-{g.id}">
				<h2 id="p-{g.id}">{@render title(g)}</h2>
				{#if g.rows.length}<GroupRows rows={g.rows} {canAct} {busy} {oncheck} flat compact />{:else}{@render empty(g)}{/if}
			</section>
		{/each}
	</div>
{/if}

<style>
	/* shared */
	.gi { display: grid; place-items: center; flex: none; width: 26px; height: 26px; border-radius: 8px; background: var(--surface-2, var(--surface-1)); color: var(--text-muted); }
	.gi svg { width: 15px; height: 15px; fill: none; stroke: currentColor; stroke-width: 1.7; stroke-linecap: round; stroke-linejoin: round; }
	.cnt { margin-left: auto; padding: 2px 9px; border-radius: var(--r-pill); background: var(--surface-2, var(--surface-1)); font-size: 12px; font-weight: 600; color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.cnt.fail { background: color-mix(in srgb, var(--fail) 18%, transparent); color: var(--fail-text); }
	.cnt.wait { background: color-mix(in srgb, var(--wait) 20%, transparent); color: var(--text); }
	.d { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--info); }
	.d.ok { background: var(--ok); }
	.d.wait { background: var(--wait); }
	.d.fail { background: var(--fail); box-shadow: 0 0 0 3px color-mix(in srgb, var(--fail) 22%, transparent); }
	.d.idle { background: var(--text-faint); }
	.d.none { background: none; box-shadow: inset 0 0 0 1.5px var(--text-faint); }
	.emp { display: flex; align-items: center; gap: 10px; margin: 0; min-height: 44px; font-size: 14px; color: var(--text-muted); }
	.lnk { display: inline-flex; align-items: center; min-height: 34px; padding: 0 10px; border: 0; border-radius: 8px; background: none; color: var(--accent, var(--text)); font: 600 13px/1 var(--font-ui); text-decoration: none; cursor: pointer; white-space: nowrap; }
	.lnk:hover { background: var(--surface-2, var(--surface-1)); }
	.lnk:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.lnk:disabled { opacity: 0.5; }
	.end { margin-left: auto; }

	/* strip: the metadata server in one line */
	.strip { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; padding: 12px 20px; font-size: 14px; color: var(--text-muted); }
	.strip b { color: var(--text); font-weight: 600; }
	.strip .sw { color: var(--text); }
	.strip .sw.fail { color: var(--fail-text); font-weight: 600; }
	.strip .at { font-size: 12.5px; }
	.strip .why { flex-basis: 100%; font-size: 13px; }

	/* 1 lattice, 5 three across */
	.lattice, .three { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 22px; align-items: stretch; }
	.three { grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 18px; }

	/* 2 tiles */
	.tiles { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 18px; }
	.tile { display: flex; flex-direction: column; gap: 6px; min-height: 148px; padding: 16px 18px; color: inherit; text-decoration: none; transition: border-color 0.15s, transform 0.15s; }
	.tile:hover { border-color: var(--line-strong); }
	.tile:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.tile[data-tone='fail'] { background: color-mix(in srgb, var(--fail) 8%, color-mix(in srgb, var(--surface-1) 55%, transparent)); }
	.tile[data-tone='wait'] { background: color-mix(in srgb, var(--wait) 8%, color-mix(in srgb, var(--surface-1) 55%, transparent)); }
	.th { display: flex; align-items: center; gap: 9px; font-size: 13.5px; font-weight: 600; color: var(--text-muted); }
	.big { margin-top: auto; font: 600 30px/1 var(--font-ui); color: var(--text); font-variant-numeric: tabular-nums; letter-spacing: -0.01em; }
	.big small { font-size: 16px; font-weight: 500; color: var(--text-muted); }
	.big .dash { font-size: 15px; }
	.sub { font-size: 13px; color: var(--text-muted); }
	.sub.fail { color: var(--fail-text); font-weight: 600; }
	.sub.wait { color: var(--text); font-weight: 600; }
	.dots { display: flex; gap: 5px; min-height: 7px; margin-top: 2px; }

	/* 3 ledger */
	.ledger { display: flex; flex-direction: column; }
	.band { padding: 6px 0 10px; border-top: 1px solid var(--line); }
	.band:first-child { border-top: 0; padding-top: 0; }
	.band[data-tone='fail'] { margin: 0 -20px; padding-left: 20px; padding-right: 20px; background: color-mix(in srgb, var(--fail) 5%, transparent); }
	.bh { display: flex; align-items: center; gap: 10px; min-height: 40px; font-size: 14.5px; font-weight: 600; color: var(--text); }
	.bh .cnt { margin-left: 0; }

	/* 4 rail */
	.rl { display: grid; grid-template-columns: 240px minmax(0, 1fr); gap: 0 24px; }
	.kinds { display: flex; flex-direction: column; gap: 2px; padding-right: 20px; border-right: 1px solid var(--line); }
	.kinds button { display: flex; align-items: center; gap: 10px; min-height: 44px; padding: 0 10px 0 8px; border: 0; border-radius: 10px; background: none; color: var(--text-muted); font: 500 14px/1 var(--font-ui); text-align: left; cursor: pointer; }
	.kinds button:hover { color: var(--text); }
	.kinds button[aria-selected='true'] { background: var(--seg-thumb); color: var(--text); box-shadow: 0 0 0 1px var(--line-strong); }
	.kinds button:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.kl { flex: 1 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.kc { font-size: 12px; font-variant-numeric: tabular-nums; }
	.pane h3 { display: flex; align-items: center; gap: 10px; margin: 0 0 12px; min-height: 34px; font: 600 15px/1.2 var(--font-ui); color: var(--text); }
	.pane .pl { font-size: 13px; font-weight: 400; color: var(--text-muted); }

	@media (max-width: 1100px) { .three { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
	@media (max-width: 900px) {
		.lattice, .three { grid-template-columns: minmax(0, 1fr); }
		.tiles { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
		.rl { grid-template-columns: minmax(0, 1fr); gap: 14px; }
		.kinds { flex-direction: row; overflow-x: auto; padding: 0 0 10px; border-right: 0; border-bottom: 1px solid var(--line); scrollbar-width: none; }
		.kinds button { flex: none; min-height: 40px; }
		.kinds .kc { display: none; }
	}
	@media (max-width: 767px) {
		.tile { min-height: 128px; padding: 14px; }
		.big { font-size: 26px; }
		.band[data-tone='fail'] { margin: 0 -14px; padding-left: 14px; padding-right: 14px; }
		.strip { padding: 12px 14px; }
	}
</style>
