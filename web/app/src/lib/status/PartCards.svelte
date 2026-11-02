<script lang="ts">
	// The Parts deck in three trial styles (?cards=a|b|c, default a):
	// a Slab (raised glass, bevel, grain), b Instrument (recessed well, segment
	// lights), c Ledger (tinted header band, divided rows, footer).
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { ago, dateTime } from '../format.ts';
	import { groupCounts, type StatusGroup } from './status.ts';

	let { parts, icon, cardLine }: { parts: StatusGroup[]; icon: Record<string, string>; cardLine: (g: StatusGroup) => string } = $props();

	const styles = [
		{ key: 'a', label: 'Slab' },
		{ key: 'b', label: 'Instrument' },
		{ key: 'c', label: 'Ledger' }
	];
	const v = $derived(['b', 'c'].includes(page.url.searchParams.get('cards') ?? '') ? page.url.searchParams.get('cards')! : 'a');

	function pick(k: string) {
		const u = new URL(page.url.href);
		u.searchParams.set('cards', k);
		void goto(u.pathname + u.search);
	}
	const toneOf = (g: StatusGroup) => {
		const c = groupCounts(g);
		return c.bad ? 'fail' : c.ok ? 'ok' : 'idle';
	};
	const newest = (g: StatusGroup) => g.rows.map((r) => r.at).filter(Boolean).sort().pop();
</script>

<div class="pick" role="group" aria-label="Card style">
	{#each styles as s (s.key)}
		<button type="button" aria-pressed={v === s.key} onclick={() => pick(s.key)}>{s.key.toUpperCase()} · {s.label}</button>
	{/each}
</div>

<ul class="deck v-{v}" data-cards={v}>
	{#each parts as g (g.id)}
		{@const c = groupCounts(g)}
		{@const t = toneOf(g)}
		{@const at = newest(g)}
		<li class="card {t}">
			{#if v === 'c'}
				<header class="band">
					<span class="ico" aria-hidden="true"><svg viewBox="0 0 20 20"><path d={icon[g.id] ?? icon.lists} /></svg></span>
					<span class="ttl">{g.label}</span>
					<span class="chip">{c.ok}/{g.rows.length}</span>
				</header>
				<ul class="rows">
					{#each g.rows.slice(0, 5) as r (r.key)}
						<li data-testid="status-row" data-tone={r.tone} data-key={r.key}>
							<i class="d {r.tone}" aria-hidden="true"></i><span class="nm">{r.name}</span><span class="wd {r.tone}">{r.word}</span>
						</li>
					{/each}
					{#if g.rows.length > 5}<li class="more">+{g.rows.length - 5} more</li>{/if}
				</ul>
				<footer class="foot">{#if at}Checked <time datetime={at} title={dateTime(at)}>{ago(at)}</time>{:else}Not checked yet{/if}</footer>
			{:else}
				<div class="top">
					<span class="ttl">{g.label}</span>
					<span class="ico" aria-hidden="true"><svg viewBox="0 0 20 20"><path d={icon[g.id] ?? icon.lists} /></svg></span>
				</div>
				{#if v === 'b'}
					<div class="well">
						<p class="val">{String(c.ok).padStart(2, '0')}<span>/{String(g.rows.length).padStart(2, '0')}</span></p>
						<div class="segs" aria-hidden="true">
							{#each g.rows as r (r.key)}<i class="s {r.tone}"></i>{/each}
						</div>
					</div>
					<p class="sub">{cardLine(g)}</p>
				{:else}
					<p class="val">{c.ok}<span>/{g.rows.length}</span></p>
					<p class="sub">{cardLine(g)}</p>
					<ul class="rows">
						{#each g.rows.slice(0, 4) as r (r.key)}
							<li data-testid="status-row" data-tone={r.tone} data-key={r.key}><i class="d {r.tone}" aria-hidden="true"></i><span class="nm">{r.name}</span></li>
						{/each}
						{#if g.rows.length > 4}<li class="more">+{g.rows.length - 4} more</li>{/if}
					</ul>
				{/if}
			{/if}
		</li>
	{/each}
</ul>

<style>
	ul { list-style: none; margin: 0; padding: 0; }
	.pick { display: flex; flex-wrap: wrap; gap: 6px; margin: -4px 0 14px; }
	.pick button {
		min-height: 32px; padding: 0 12px; border: 1px solid var(--line); border-radius: var(--r-pill); background: none;
		color: var(--text-muted); font: 500 12.5px/1 var(--font-ui); cursor: pointer;
	}
	.pick button[aria-pressed='true'] { color: var(--text); border-color: var(--line-strong); background: var(--surface-2, var(--surface-1)); }
	.pick button:focus-visible { outline: none; box-shadow: var(--focus-ring); }

	.deck { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(240px, 100%), 1fr)); gap: 16px; }
	.card { --tint: var(--ok); position: relative; display: flex; flex-direction: column; min-width: 0; isolation: isolate; }
	.card.fail { --tint: var(--fail); }
	.card.idle { --tint: var(--text-faint); }
	.ttl { font: 600 13.5px/1.2 var(--font-ui); color: var(--text); }
	.ico { display: grid; place-items: center; flex: none; width: 30px; height: 30px; border-radius: 9px; color: var(--tint); }
	.ico svg { width: 16px; height: 16px; fill: none; stroke: currentColor; stroke-width: 1.7; stroke-linecap: round; stroke-linejoin: round; }
	.top { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
	.val { margin: 10px 0 0; font: 700 34px/1 var(--font-ui); color: var(--text); font-variant-numeric: tabular-nums; letter-spacing: -0.01em; }
	.val span { font-size: 17px; font-weight: 500; color: var(--text-muted); }
	.sub { margin: 4px 0 0; font-size: 12.5px; color: var(--text-muted); }
	.card.fail .sub { color: var(--fail-text); }
	.rows li { display: flex; align-items: center; gap: 9px; min-width: 0; font-size: 13px; color: var(--text); }
	.nm { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.more { color: var(--text-muted); }
	.d { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--info); }
	.d.ok { background: var(--ok); box-shadow: 0 0 0 3px color-mix(in srgb, var(--ok) 18%, transparent); }
	.d.wait { background: var(--wait); }
	.d.fail { background: var(--fail); box-shadow: 0 0 0 3px color-mix(in srgb, var(--fail) 22%, transparent); }
	.d.idle { background: var(--text-faint); }

	/* A · Slab: raised glass with a lit top edge, grain and a hairline divider */
	.v-a .card {
		padding: 18px 18px 16px; border-radius: 18px;
		border: 1px solid color-mix(in srgb, var(--tint) 22%, var(--line));
		background:
			radial-gradient(120% 80% at 100% 0%, color-mix(in srgb, var(--tint) 16%, transparent), transparent 60%),
			linear-gradient(180deg, color-mix(in srgb, var(--surface-1) 92%, #fff 8%), var(--surface-1));
		box-shadow:
			inset 0 1px 0 rgb(255 255 255 / 0.10), inset 0 -1px 0 rgb(0 0 0 / 0.08),
			0 1px 2px rgb(0 0 0 / 0.06), 0 12px 28px -14px rgb(0 0 0 / 0.30);
	}
	.v-a .card::after {
		content: ''; position: absolute; inset: 0; z-index: -1; border-radius: inherit; opacity: 0.35; pointer-events: none;
		background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='120' height='120'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='.9' numOctaves='2' stitchTiles='stitch'/%3E%3CfeColorMatrix values='0 0 0 0 .5 0 0 0 0 .5 0 0 0 0 .5 0 0 0 .18 0'/%3E%3C/filter%3E%3Crect width='120' height='120' filter='url(%23n)'/%3E%3C/svg%3E");
	}
	.v-a .ico { background: color-mix(in srgb, var(--tint) 16%, transparent); box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--tint) 30%, transparent); }
	.v-a .rows { display: flex; flex-direction: column; gap: 6px; margin-top: 14px; padding-top: 12px; border-top: 1px dashed color-mix(in srgb, var(--tint) 30%, var(--line)); }

	/* B · Instrument: dark panel, recessed readout, one segment light per part */
	.v-b .card {
		padding: 14px; border-radius: 14px; border: 1px solid var(--line-strong);
		background: linear-gradient(180deg, var(--surface-2, var(--surface-1)), var(--surface-1));
		box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.07), 0 2px 0 rgb(0 0 0 / 0.08), 0 14px 30px -18px rgb(0 0 0 / 0.45);
	}
	.v-b .ttl { font-size: 11.5px; letter-spacing: 0.12em; text-transform: uppercase; color: var(--text-muted); }
	.v-b .ico { width: 26px; height: 26px; border-radius: 50%; border: 1px solid var(--line); }
	.v-b .well {
		margin-top: 12px; padding: 12px 12px 10px; border-radius: 10px;
		background: linear-gradient(180deg, #0b0d10, #15181d);
		box-shadow: inset 0 2px 6px rgb(0 0 0 / 0.35), inset 0 0 0 1px rgb(0 0 0 / 0.25), 0 1px 0 rgb(255 255 255 / 0.06);
	}
	.v-b .val { margin: 0; font-family: var(--font-mono, ui-monospace, monospace); font-size: 30px; color: color-mix(in srgb, var(--tint) 70%, #fff); text-shadow: 0 0 14px color-mix(in srgb, var(--tint) 45%, transparent); }
	.v-b .val span { color: color-mix(in srgb, #fff 45%, transparent); }
	.v-b .segs { display: flex; gap: 3px; margin-top: 10px; }
	.v-b .s { flex: 1; height: 6px; border-radius: 2px; background: rgb(255 255 255 / 0.12); }
	.v-b .s.ok { background: var(--ok); box-shadow: 0 0 6px color-mix(in srgb, var(--ok) 60%, transparent); }
	.v-b .s.wait { background: var(--wait); }
	.v-b .s.fail { background: var(--fail); box-shadow: 0 0 6px color-mix(in srgb, var(--fail) 70%, transparent); }
	.v-b .sub { margin-top: 10px; }

	/* C · Ledger: tinted header band, ruled rows, footer strip */
	.v-c .card {
		overflow: hidden; border-radius: 14px; border: 1px solid var(--line); background: var(--surface-1);
		box-shadow: 0 1px 0 rgb(0 0 0 / 0.04), 0 10px 24px -16px rgb(0 0 0 / 0.30);
	}
	.v-c .band {
		display: flex; align-items: center; gap: 10px; padding: 12px 14px;
		background: linear-gradient(90deg, color-mix(in srgb, var(--tint) 20%, var(--surface-1)), color-mix(in srgb, var(--tint) 6%, var(--surface-1)));
		border-bottom: 1px solid color-mix(in srgb, var(--tint) 30%, var(--line));
		box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.08);
	}
	.v-c .band .ttl { flex: 1; }
	.v-c .ico { width: 28px; height: 28px; background: var(--surface-1); box-shadow: 0 1px 2px rgb(0 0 0 / 0.12); }
	.v-c .chip { padding: 3px 9px; border-radius: var(--r-pill); background: var(--surface-1); font: 600 12px/1.2 var(--font-ui); color: var(--text); font-variant-numeric: tabular-nums; box-shadow: inset 0 0 0 1px var(--line); }
	.v-c .rows li { min-height: 38px; padding: 0 14px; border-top: 1px solid var(--line); }
	.v-c .rows li:first-child { border-top: 0; }
	.v-c .rows li:nth-child(even) { background: color-mix(in srgb, var(--surface-2, var(--surface-1)) 50%, transparent); }
	.v-c .wd { font-size: 12px; color: var(--text-muted); }
	.v-c .wd.fail { color: var(--fail-text); font-weight: 600; }
	.v-c .foot { margin-top: auto; padding: 9px 14px; border-top: 1px solid var(--line); background: var(--surface-2, var(--surface-1)); font-size: 12px; color: var(--text-muted); }

	@media (max-width: 767px) {
		.deck { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
		.v-c.deck { grid-template-columns: minmax(0, 1fr); }
		.v-a .card, .v-b .card { padding: 12px; }
		.v-a .rows { display: none; }
		.val { font-size: 28px; }
	}
</style>
