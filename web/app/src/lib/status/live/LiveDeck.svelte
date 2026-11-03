<script lang="ts">
	// View "Deck": a small stack of cards, like a wallet. The top card is the
	// job running now (name, note, progress, time so far) or, when idle, the
	// next run counting down. Two cards peek out behind it: other running or
	// waiting jobs first, then the next runs. Clicking a peeking card lifts it
	// to the top for six seconds, then the stack settles back.
	import type { LiveJobs } from '../livejobs.svelte.ts';
	import { clock, wait, t } from '../livejobs.svelte.ts';
	import type { StatusRow } from '../status.ts';
	import type { ActiveJob } from '../../types.ts';

	let { lj, upcoming, recent }: { lj: LiveJobs; upcoming: StatusRow[]; recent: StatusRow[] } = $props();

	type Item = { key: string; kind: 'run'; job: ActiveJob } | { key: string; kind: 'wait'; job: ActiveJob } | { key: string; kind: 'next'; row: StatusRow };

	const tone: Record<string, string> = { ok: 'var(--ok)', wait: 'var(--wait)', fail: 'var(--fail)', info: 'var(--info)', idle: 'var(--text-faint)' };
	const hm = (ms: number) => new Date(ms).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false });

	// Every card the stack could show, in its natural order.
	const items = $derived.by((): Item[] => {
		const out: Item[] = [];
		for (const j of lj.running) out.push({ key: `r${j.id}`, kind: 'run', job: j });
		for (const j of lj.queued) out.push({ key: `q${j.id}`, kind: 'wait', job: j });
		for (const r of lj.coming(upcoming, 3)) out.push({ key: `n${r.key}`, kind: 'next', row: r });
		// Idle, the next run leads and waiting jobs peek behind it.
		if (!lj.running.length) {
			const first = out.findIndex((x) => x.kind === 'next');
			if (first > 0) out.unshift(...out.splice(first, 1));
		}
		return out;
	});

	// A card lifted by a click stays on top for a while, then eases back.
	let pick = $state<string | null>(null);
	let timer: ReturnType<typeof setTimeout> | undefined;
	function lift(key: string) {
		pick = key;
		clearTimeout(timer);
		timer = setTimeout(() => (pick = null), 6000);
	}
	$effect(() => () => clearTimeout(timer));

	const stack = $derived.by(() => {
		const list = [...items];
		const i = pick ? list.findIndex((x) => x.key === pick) : -1;
		if (i > 0) list.unshift(...list.splice(i, 1));
		return list.slice(0, 3);
	});

	const nameOf = (x: Item) => (x.kind === 'next' ? x.row.name : x.job.label);
	const since = (j: ActiveJob) => lj.tick - t(j.started_at ?? j.queued_at);
	// The short time a peeking card shows on its title line.
	const peekTime = (x: Item) => {
		if (x.kind === 'run') return j_pct(x.job) > 0 ? `${j_pct(x.job)}%` : 'running';
		if (x.kind === 'wait') return 'waiting';
		const d = lj.due(x.row);
		return d <= 0 ? 'due now' : `in ${wait(d)}`;
	};
	const j_pct = (j: ActiveJob) => Math.round(Math.min(1, Math.max(0, j.progress)) * 100);
	const busy = $derived(lj.running.length > 0);
	const count = $derived(lj.running.length + lj.queued.length + lj.coming(upcoming, 99).length);
</script>

<div class="deck">
	<h3>
		{busy ? 'Working on' : 'Up next'}
		<span class="st" class:on={busy}>{busy ? `${lj.running.length} running` : count ? `${count} coming` : 'Idle'}</span>
	</h3>

	<div class="stage" style:--n={stack.length}>
		{#if !stack.length}
			<div class="empty">
				<b>Nothing planned</b>
				<span>No job is running and none is scheduled.</span>
			</div>
		{/if}
		{#each stack as x, k (x.key)}
			<div class="cd" class:top={k === 0} class:live={x.kind === 'run'} style:--k={k} style:z-index={3 - k}>
				<!-- Only the top card's face is read out; a card behind shows just its title line, which stays a button. -->
				<div class="face" aria-hidden={k === 0 ? undefined : 'true'} inert={k !== 0}>
					{#if x.kind === 'run'}
						{@const pct = j_pct(x.job)}
						<div class="hd"><span class="kick"><i class="dot" aria-hidden="true"></i>Running</span><span class="tm">{clock(since(x.job))}</span></div>
						<p class="nm">{x.job.label}</p>
						<p class="nt">{x.job.note || 'Working'}{x.job.by_person ? ' · started by hand' : ''}</p>
						<div class="pg">
							<div class="tr" class:indet={pct <= 0} role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={pct > 0 ? pct : undefined} aria-label="{x.job.label} progress">
								<i style:transform="scaleX({pct > 0 ? pct / 100 : 1})"></i>
							</div>
							<span class="pc">{pct > 0 ? `${pct}%` : '…'}</span>
						</div>
					{:else if x.kind === 'wait'}
						<div class="hd"><span class="kick">Waiting in line</span><span class="tm">{clock(since(x.job))}</span></div>
						<p class="nm">{x.job.label}</p>
						<p class="nt">Starts as soon as a slot is free{x.job.by_person ? ' · asked for by hand' : ''}</p>
						<div class="big"><span>queued at</span><b>{hm(t(x.job.queued_at))}</b></div>
					{:else}
						{@const d = lj.due(x.row)}
						<div class="hd"><span class="kick"><i class="dot" style:--c={tone[x.row.tone] ?? 'var(--info)'} aria-hidden="true"></i>Next run</span><span class="tm">at {hm(t(x.row.nextRun))}</span></div>
						<p class="nm">{x.row.name}</p>
						<div class="big" class:due={d <= 0}>
							{#if d <= 0}<b>due now</b>{:else}<span>in</span><b>{wait(d)}</b>{/if}
						</div>
						<div class="fl" role="meter" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(lj.fill(x.row) * 100)} aria-label="Wait for {x.row.name}">
							<i style:transform="scaleX({lj.fill(x.row)})"></i>
						</div>
					{/if}
				</div>
				<button class="pk" type="button" inert={k === 0} onclick={() => lift(x.key)} aria-label="Show {nameOf(x)} on top">
					<span class="pn">{nameOf(x)}</span><span class="pt">{peekTime(x)}</span>
				</button>
			</div>
		{/each}
	</div>
</div>

<style>
	.deck { display: flex; flex-direction: column; min-width: 0; height: 100%; container-type: inline-size; }
	h3 { display: flex; align-items: center; gap: 10px; margin: 0 0 10px; font: 600 13px/1.2 var(--font-ui); color: var(--text-muted); }
	.st { margin-left: auto; padding: 2px 9px; border-radius: var(--r-pill); background: var(--surface-2, var(--surface-1)); font-size: 11.5px; font-weight: 600; color: var(--text-muted); }
	.st.on { background: color-mix(in srgb, var(--accent, var(--text)) 16%, transparent); color: var(--text); }

	/* The stack: the top card is 150px tall; each card behind sticks out 30px below the one in front. */
	.stage {
		--h: 150px;
		--peek: 30px;
		position: relative;
		width: 100%;
		max-width: 400px;
		height: calc(var(--h) + (var(--n, 1) - 1) * var(--peek));
		min-height: var(--h);
		margin: auto;
		padding-bottom: 6px;
		transition: height 0.4s ease;
	}
	.cd {
		position: absolute;
		top: 0;
		left: calc(var(--k) * 8px);
		right: calc(var(--k) * 8px);
		height: var(--h);
		border-radius: 14px;
		background: color-mix(in srgb, var(--surface-2, var(--surface-1)) calc(100% - var(--k) * 12%), var(--surface-1));
		border: 1px solid var(--line);
		box-shadow: 0 10px 22px -16px var(--line-strong);
		transform: translateY(calc(var(--k) * var(--peek)));
		overflow: hidden;
		transition: transform 0.45s cubic-bezier(0.2, 0.8, 0.2, 1), left 0.45s cubic-bezier(0.2, 0.8, 0.2, 1), right 0.45s cubic-bezier(0.2, 0.8, 0.2, 1), background-color 0.45s ease;
	}
	.cd.top {
		background: color-mix(in srgb, var(--surface-2, var(--surface-1)) 92%, var(--text));
		border-color: var(--line-strong);
		box-shadow: inset 0 1px 0 color-mix(in srgb, var(--text) 9%, transparent), inset 0 0 0 1px color-mix(in srgb, var(--text) 3%, transparent), 0 14px 28px -18px var(--line-strong);
	}
	.cd.top.live { border-color: color-mix(in srgb, var(--accent) 40%, var(--line-strong)); }

	.face { position: absolute; inset: 0; display: flex; flex-direction: column; padding: 14px 16px 14px; opacity: 0; transition: opacity 0.3s ease; }
	.cd.top .face { opacity: 1; }
	.hd { display: flex; align-items: center; justify-content: space-between; gap: 10px; font-size: 11.5px; color: var(--text-muted); }
	.kick { display: inline-flex; align-items: center; gap: 7px; font-weight: 600; }
	.dot { width: 7px; height: 7px; border-radius: 50%; background: var(--c, var(--accent)); }
	.tm { font-variant-numeric: tabular-nums; white-space: nowrap; }
	.nm { margin: 8px 0 0; font: 600 20px/1.2 var(--font-display); color: var(--text); letter-spacing: -0.01em; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.nt { margin: 4px 0 0; font-size: 12.5px; color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

	.pg { display: flex; align-items: center; gap: 10px; margin-top: auto; }
	.tr { position: relative; flex: 1 1 auto; height: 8px; border-radius: var(--r-pill); overflow: hidden; background: color-mix(in srgb, var(--text) 9%, transparent); }
	.tr i { position: absolute; inset: 0; border-radius: inherit; background: var(--accent); transform-origin: left center; transition: transform 0.8s linear; }
	.tr.indet i { opacity: 0.45; }
	.pc { flex: none; min-width: 3ch; text-align: right; font: 600 13px/1 var(--font-ui); color: var(--text); font-variant-numeric: tabular-nums; }

	.big { display: flex; align-items: baseline; gap: 8px; margin-top: auto; color: var(--text-muted); font-size: 13px; }
	.big b { font: 600 28px/1 var(--font-display); color: var(--text); font-variant-numeric: tabular-nums; letter-spacing: -0.01em; }
	.big.due b { color: var(--accent-text); }
	.fl { position: relative; height: 3px; margin-top: 10px; border-radius: var(--r-pill); overflow: hidden; background: color-mix(in srgb, var(--text) 9%, transparent); }
	.fl i { position: absolute; inset: 0; background: color-mix(in srgb, var(--text) 45%, transparent); transform-origin: left center; transition: transform 0.9s linear; }

	/* The title line of a card behind: it fills the strip that sticks out. */
	.pk {
		position: absolute;
		left: 0;
		right: 0;
		bottom: 0;
		height: var(--peek);
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 0 14px;
		border: 0;
		background: none;
		font: 500 12.5px/1 var(--font-ui);
		color: var(--text-muted);
		text-align: left;
		cursor: pointer;
		opacity: 1;
		transition: opacity 0.3s ease, color 0.2s ease;
	}
	.pk:hover { color: var(--text); }
	.pk:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 0 0 14px 14px; }
	.cd.top .pk { opacity: 0; pointer-events: none; }
	.pn { flex: 1 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.pt { flex: none; font-variant-numeric: tabular-nums; }

	.empty { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 6px; border: 1px dashed var(--line-strong); border-radius: 14px; color: var(--text-muted); font-size: 12.5px; text-align: center; padding: 0 16px; }
	.empty b { font: 600 16px/1.2 var(--font-display); color: var(--text); }

	@container (max-width: 380px) {
		.nm { font-size: 18px; }
		.big b { font-size: 24px; }
	}

	@media (prefers-reduced-motion: no-preference) {
		@starting-style { .cd { opacity: 0; transform: translateY(calc(var(--k) * var(--peek) + 14px)); } }
		.cd { transition-property: transform, left, right, background-color, opacity; }
		.cd.top.live .dot { animation: beat 2.4s ease-in-out infinite; }
		.tr.indet i { inset: 0 auto 0 0; width: 40%; opacity: 1; transform: none; animation: slide 1.6s ease-in-out infinite; }
		@keyframes beat { 0%, 100% { opacity: 1; transform: scale(1); } 50% { opacity: 0.55; transform: scale(0.8); } }
		@keyframes slide { 0% { transform: translateX(-100%); } 100% { transform: translateX(250%); } }
	}
	@media (prefers-reduced-motion: reduce) {
		.stage, .cd, .face, .pk, .tr i, .fl i { transition: none; }
	}
</style>
