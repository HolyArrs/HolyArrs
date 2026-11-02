<script lang="ts">
	// The Parts deck in trial styles (?cards=, default b). b Instrument and
	// c Ledger were kept from the first round; d to m are new.
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { ago, dateTime } from '../format.ts';
	import { groupCounts, type StatusGroup } from './status.ts';

	let { parts, icon, cardLine }: { parts: StatusGroup[]; icon: Record<string, string>; cardLine: (g: StatusGroup) => string } = $props();

	const styles = [
		{ key: 'b', label: 'Instrument' },
		{ key: 'c', label: 'Ledger' },
		{ key: 'd', label: 'Gauge' },
		{ key: 'e', label: 'Terminal' },
		{ key: 'f', label: 'Neon edge' },
		{ key: 'g', label: 'Watermark' },
		{ key: 'h', label: 'Rail' },
		{ key: 'i', label: 'Soft relief' },
		{ key: 'j', label: 'Ticket' },
		{ key: 'k', label: 'Bars' },
		{ key: 'l', label: 'Seal' },
		{ key: 'm', label: 'Blueprint' }
	];
	const v = $derived(styles.find((s) => s.key === page.url.searchParams.get('cards'))?.key ?? 'b');

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
	const pad = (n: number) => String(n).padStart(2, '0');
	// Ring for the Gauge: circumference of r=26.
	const C = 2 * Math.PI * 26;
</script>

{#snippet ico(id: string)}
	<span class="ico" aria-hidden="true"><svg viewBox="0 0 20 20"><path d={icon[id] ?? icon.lists} /></svg></span>
{/snippet}

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
			{#if v === 'b'}
				<div class="top"><span class="ttl">{g.label}</span>{@render ico(g.id)}</div>
				<div class="well">
					<p class="val">{pad(c.ok)}<span>/{pad(g.rows.length)}</span></p>
					<div class="segs" aria-hidden="true">{#each g.rows as r (r.key)}<i class="s {r.tone}"></i>{/each}</div>
				</div>
				<p class="sub">{cardLine(g)}</p>
			{:else if v === 'c'}
				<header class="band">{@render ico(g.id)}<span class="ttl">{g.label}</span><span class="chip">{c.ok}/{g.rows.length}</span></header>
				<ul class="rows">
					{#each g.rows.slice(0, 5) as r (r.key)}
						<li><i class="d {r.tone}" aria-hidden="true"></i><span class="nm">{r.name}</span><span class="wd {r.tone}">{r.word}</span></li>
					{/each}
					{#if g.rows.length > 5}<li class="more">+{g.rows.length - 5} more</li>{/if}
				</ul>
				<footer class="foot">{#if at}Checked <time datetime={at} title={dateTime(at)}>{ago(at)}</time>{:else}Not checked yet{/if}</footer>
			{:else if v === 'd'}
				<div class="gauge">
					<svg viewBox="0 0 64 64" aria-hidden="true">
						<circle class="trk" cx="32" cy="32" r="26" />
						<circle class="arc" cx="32" cy="32" r="26" stroke-dasharray="{(c.ok / g.rows.length) * C} {C}" />
					</svg>
					<span class="gv">{c.ok}<small>/{g.rows.length}</small></span>
				</div>
				<div class="gt"><span class="ttl">{g.label}</span><span class="sub">{cardLine(g)}</span></div>
			{:else if v === 'e'}
				<div class="bar"><i></i><i></i><i></i><span>{g.label.toLowerCase().replace(/ /g, '-')}</span></div>
				<pre class="term">{#each g.rows.slice(0, 5) as r (r.key)}<span class={r.tone}>[{r.tone === 'ok' ? ' OK ' : r.tone === 'fail' ? 'FAIL' : r.tone === 'wait' ? 'WARN' : ' -- '}]</span> {r.name}
{/each}{#if g.rows.length > 5}<span class="idle">… {g.rows.length - 5} more</span>
{/if}<span class="cur">$ {c.ok}/{g.rows.length} up</span></pre>
			{:else if v === 'g'}
				<span class="wm" aria-hidden="true">{c.ok}</span>
				<div class="top"><span class="ttl">{g.label}</span>{@render ico(g.id)}</div>
				<p class="val">{c.ok}<span> of {g.rows.length}</span></p>
				<p class="sub">{cardLine(g)}</p>
			{:else if v === 'j'}
				<div class="stub"><span class="sv">{c.ok}</span><span class="sl">of {g.rows.length}</span></div>
				<div class="tk">
					<span class="ttl">{g.label}</span>
					<span class="sub">{cardLine(g)}</span>
					<span class="dots">{#each g.rows as r (r.key)}<i class="d {r.tone}"></i>{/each}</span>
				</div>
			{:else if v === 'k'}
				<div class="top"><span class="ttl">{g.label}</span><span class="chip">{c.ok}/{g.rows.length}</span></div>
				<div class="bars" aria-hidden="true">
					{#each g.rows.slice(0, 12) as r (r.key)}<i class="b {r.tone}" title={r.name}></i>{/each}
				</div>
				<p class="sub">{cardLine(g)}</p>
			{:else if v === 'l'}
				<div class="seal"><span>{c.ok}/{g.rows.length}</span></div>
				<span class="ttl">{g.label}</span>
				<span class="sub">{cardLine(g)}</span>
			{:else}
				<!-- f Neon edge · h Rail · i Soft relief · m Blueprint share one body -->
				<div class="top"><span class="ttl">{g.label}</span>{@render ico(g.id)}</div>
				<p class="val">{c.ok}<span>/{g.rows.length}</span></p>
				<p class="sub">{cardLine(g)}</p>
				<ul class="rows">
					{#each g.rows.slice(0, 3) as r (r.key)}<li><i class="d {r.tone}" aria-hidden="true"></i><span class="nm">{r.name}</span></li>{/each}
					{#if g.rows.length > 3}<li class="more">+{g.rows.length - 3} more</li>{/if}
				</ul>
			{/if}
		</li>
	{/each}
</ul>

<style>
	ul { list-style: none; margin: 0; padding: 0; }
	.pick { display: flex; flex-wrap: wrap; gap: 6px; margin: -4px 0 14px; }
	.pick button { min-height: 32px; padding: 0 12px; border: 1px solid var(--line); border-radius: var(--r-pill); background: none; color: var(--text-muted); font: 500 12.5px/1 var(--font-ui); cursor: pointer; }
	.pick button[aria-pressed='true'] { color: var(--text); border-color: var(--line-strong); background: var(--surface-2, var(--surface-1)); }
	.pick button:focus-visible { outline: none; box-shadow: var(--focus-ring); }

	.deck { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(240px, 100%), 1fr)); gap: 16px; align-items: stretch; }
	.card { --tint: var(--ok); position: relative; display: flex; flex-direction: column; min-width: 0; isolation: isolate; overflow: hidden; }
	.card.fail { --tint: var(--fail); }
	.card.idle { --tint: var(--text-faint); }
	.ttl { font: 600 13.5px/1.2 var(--font-ui); color: var(--text); }
	.top { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
	.ico { display: grid; place-items: center; flex: none; width: 30px; height: 30px; border-radius: 9px; color: var(--tint); }
	.ico svg { width: 16px; height: 16px; fill: none; stroke: currentColor; stroke-width: 1.7; stroke-linecap: round; stroke-linejoin: round; }
	.val { margin: 10px 0 0; font: 700 34px/1 var(--font-ui); color: var(--text); font-variant-numeric: tabular-nums; }
	.val span { font-size: 16px; font-weight: 500; color: var(--text-muted); }
	.sub { margin: 4px 0 0; font-size: 12.5px; color: var(--text-muted); }
	.card.fail .sub { color: var(--fail-text); }
	.rows li { display: flex; align-items: center; gap: 9px; min-width: 0; font-size: 13px; color: var(--text); }
	.nm { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.more { color: var(--text-muted); }
	.chip { padding: 3px 9px; border-radius: var(--r-pill); background: var(--surface-1); font: 600 12px/1.2 var(--font-ui); color: var(--text); font-variant-numeric: tabular-nums; box-shadow: inset 0 0 0 1px var(--line); }
	.d { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--info); }
	.d.ok { background: var(--ok); }
	.d.wait { background: var(--wait); }
	.d.fail { background: var(--fail); }
	.d.idle { background: var(--text-faint); }

	/* B · Instrument */
	.v-b .card { padding: 14px; border-radius: 14px; border: 1px solid var(--line-strong); background: linear-gradient(180deg, var(--surface-2, var(--surface-1)), var(--surface-1)); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.07), 0 2px 0 rgb(0 0 0 / 0.08), 0 14px 30px -18px rgb(0 0 0 / 0.45); }
	.v-b .ttl { font-size: 11.5px; letter-spacing: 0.12em; text-transform: uppercase; color: var(--text-muted); }
	.v-b .ico { width: 26px; height: 26px; border-radius: 50%; border: 1px solid var(--line); }
	.v-b .well { margin-top: 12px; padding: 12px 12px 10px; border-radius: 10px; background: linear-gradient(180deg, #0b0d10, #15181d); box-shadow: inset 0 2px 6px rgb(0 0 0 / 0.5), inset 0 0 0 1px rgb(0 0 0 / 0.3), 0 1px 0 rgb(255 255 255 / 0.06); }
	.v-b .val { margin: 0; font-family: var(--font-mono, ui-monospace, monospace); font-size: 30px; color: color-mix(in srgb, var(--tint) 70%, #fff); text-shadow: 0 0 14px color-mix(in srgb, var(--tint) 45%, transparent); }
	.v-b .val span { color: rgb(255 255 255 / 0.45); }
	.v-b .segs { display: flex; gap: 3px; margin-top: 10px; }
	.v-b .s { flex: 1; height: 6px; border-radius: 2px; background: rgb(255 255 255 / 0.12); }
	.v-b .s.ok { background: var(--ok); box-shadow: 0 0 6px color-mix(in srgb, var(--ok) 60%, transparent); }
	.v-b .s.wait { background: var(--wait); }
	.v-b .s.fail { background: var(--fail); box-shadow: 0 0 6px color-mix(in srgb, var(--fail) 70%, transparent); }
	.v-b .sub { margin-top: 10px; }

	/* C · Ledger */
	.v-c .card { border-radius: 14px; border: 1px solid var(--line); background: var(--surface-1); box-shadow: 0 10px 24px -16px rgb(0 0 0 / 0.30); }
	.v-c .band { display: flex; align-items: center; gap: 10px; padding: 12px 14px; background: linear-gradient(90deg, color-mix(in srgb, var(--tint) 20%, var(--surface-1)), color-mix(in srgb, var(--tint) 6%, var(--surface-1))); border-bottom: 1px solid color-mix(in srgb, var(--tint) 30%, var(--line)); }
	.v-c .band .ttl { flex: 1; }
	.v-c .ico { width: 28px; height: 28px; background: var(--surface-1); box-shadow: 0 1px 2px rgb(0 0 0 / 0.12); }
	.v-c .rows li { min-height: 38px; padding: 0 14px; border-top: 1px solid var(--line); }
	.v-c .rows li:first-child { border-top: 0; }
	.v-c .rows li:nth-child(even) { background: color-mix(in srgb, var(--surface-2, var(--surface-1)) 50%, transparent); }
	.v-c .wd { font-size: 12px; color: var(--text-muted); }
	.v-c .wd.fail { color: var(--fail-text); font-weight: 600; }
	.v-c .foot { margin-top: auto; padding: 9px 14px; border-top: 1px solid var(--line); background: var(--surface-2, var(--surface-1)); font-size: 12px; color: var(--text-muted); }

	/* D · Gauge: ring meter beside the title */
	.v-d .card { flex-direction: row; align-items: center; gap: 16px; padding: 16px; border-radius: 18px; border: 1px solid var(--line); background: radial-gradient(90% 120% at 0% 50%, color-mix(in srgb, var(--tint) 10%, transparent), transparent 60%), var(--surface-1); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.08), 0 12px 26px -18px rgb(0 0 0 / 0.4); }
	.v-d .gauge { position: relative; flex: none; width: 72px; height: 72px; }
	.v-d svg { width: 100%; height: 100%; transform: rotate(-90deg); }
	.v-d circle { fill: none; stroke-width: 7; }
	.v-d .trk { stroke: color-mix(in srgb, var(--text) 10%, transparent); }
	.v-d .arc { stroke: var(--tint); stroke-linecap: round; filter: drop-shadow(0 0 4px color-mix(in srgb, var(--tint) 50%, transparent)); }
	.v-d .gv { position: absolute; inset: 0; display: grid; place-items: center; font: 700 18px/1 var(--font-ui); color: var(--text); }
	.v-d .gv small { font-size: 11px; color: var(--text-muted); font-weight: 500; }
	.v-d .gt { display: flex; flex-direction: column; min-width: 0; }

	/* E · Terminal: console window */
	.v-e .card { border-radius: 12px; border: 1px solid #262a31; background: #0c0e12; box-shadow: 0 16px 30px -18px rgb(0 0 0 / 0.6); }
	.v-e .bar { display: flex; align-items: center; gap: 6px; padding: 9px 12px; background: #171a20; border-bottom: 1px solid #262a31; }
	.v-e .bar i { width: 9px; height: 9px; border-radius: 50%; background: #3a3f48; }
	.v-e .bar i:first-child { background: #ff5f57; }
	.v-e .bar i:nth-child(2) { background: #febc2e; }
	.v-e .bar i:nth-child(3) { background: #28c840; }
	.v-e .bar span { margin-left: 8px; font: 500 11.5px/1 var(--font-mono, ui-monospace, monospace); color: #8a919c; }
	.v-e .term { margin: 0; padding: 12px; font: 12.5px/1.7 var(--font-mono, ui-monospace, monospace); color: #d6dae0; white-space: pre-wrap; overflow-wrap: anywhere; }
	.v-e .term .ok { color: #4ade80; }
	.v-e .term .fail { color: #f87171; }
	.v-e .term .wait { color: #fbbf24; }
	.v-e .term .idle { color: #6b7280; }
	.v-e .term .cur { color: #8a919c; }

	/* F · Neon edge: dark glass with a glowing tone border */
	.v-f .card { padding: 16px; border-radius: 16px; background: linear-gradient(160deg, #13161c, #0d0f13); border: 1px solid color-mix(in srgb, var(--tint) 55%, transparent); box-shadow: 0 0 0 1px rgb(0 0 0 / 0.4), 0 0 22px -4px color-mix(in srgb, var(--tint) 55%, transparent), inset 0 0 18px -8px color-mix(in srgb, var(--tint) 60%, transparent); }
	.v-f .ttl, .v-f .val, .v-f .rows li { color: #eef1f5; }
	.v-f .val span, .v-f .sub, .v-f .more { color: #8f97a3; }
	.v-f .ico { border: 1px solid color-mix(in srgb, var(--tint) 50%, transparent); }
	.v-f .rows { display: flex; flex-direction: column; gap: 6px; margin-top: 12px; padding-top: 10px; border-top: 1px solid rgb(255 255 255 / 0.08); }

	/* G · Watermark: giant faded count behind */
	.v-g .card { padding: 18px; border-radius: 18px; border: 1px solid var(--line); background: linear-gradient(135deg, color-mix(in srgb, var(--tint) 8%, var(--surface-1)), var(--surface-1)); box-shadow: 0 14px 30px -20px rgb(0 0 0 / 0.4); min-height: 140px; }
	.v-g .wm { position: absolute; right: -6px; bottom: -34px; z-index: -1; font: 800 150px/1 var(--font-ui); color: color-mix(in srgb, var(--tint) 14%, transparent); letter-spacing: -0.05em; }
	.v-g .ico { background: var(--surface-1); box-shadow: 0 1px 3px rgb(0 0 0 / 0.12); }
	.v-g .val { margin-top: auto; padding-top: 18px; }

	/* H · Rail: coloured side rail */
	.v-h .card { padding: 16px 16px 16px 22px; border-radius: 14px; border: 1px solid var(--line); background: var(--surface-1); box-shadow: 0 10px 24px -18px rgb(0 0 0 / 0.4); }
	.v-h .card::before { content: ''; position: absolute; left: 0; top: 0; bottom: 0; width: 6px; background: linear-gradient(var(--tint), color-mix(in srgb, var(--tint) 40%, transparent)); }
	.v-h .rows { display: flex; flex-direction: column; gap: 6px; margin-top: 12px; }
	.v-h .ico { background: color-mix(in srgb, var(--tint) 12%, transparent); }

	/* I · Soft relief: embossed, pillowy */
	.v-i .card { padding: 18px; border-radius: 22px; background: var(--surface-1); border: 1px solid color-mix(in srgb, var(--line) 50%, transparent); box-shadow: 8px 8px 18px rgb(0 0 0 / 0.12), -6px -6px 14px rgb(255 255 255 / 0.06), inset 0 1px 0 rgb(255 255 255 / 0.08); }
	.v-i .ico { border-radius: 50%; background: var(--surface-1); box-shadow: inset 3px 3px 6px rgb(0 0 0 / 0.14), inset -3px -3px 6px rgb(255 255 255 / 0.06); }
	.v-i .rows { display: flex; flex-direction: column; gap: 6px; margin-top: 12px; padding: 10px 12px; border-radius: 14px; box-shadow: inset 3px 3px 8px rgb(0 0 0 / 0.12), inset -3px -3px 8px rgb(255 255 255 / 0.05); }

	/* J · Ticket: stub with the count, perforated edge */
	.v-j .card { flex-direction: row; border-radius: 14px; border: 1px solid var(--line); background: var(--surface-1); box-shadow: 0 12px 26px -18px rgb(0 0 0 / 0.4); }
	.v-j .stub { display: flex; flex-direction: column; align-items: center; justify-content: center; flex: none; width: 84px; padding: 14px 0; background: linear-gradient(180deg, color-mix(in srgb, var(--tint) 24%, var(--surface-1)), color-mix(in srgb, var(--tint) 10%, var(--surface-1))); border-right: 2px dashed var(--line-strong); }
	.v-j .sv { font: 800 30px/1 var(--font-ui); color: var(--text); }
	.v-j .sl { font-size: 11.5px; color: var(--text-muted); }
	.v-j .tk { display: flex; flex-direction: column; gap: 3px; min-width: 0; padding: 14px; }
	.v-j .dots { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 8px; }

	/* K · Bars: one bar per part */
	.v-k .card { padding: 16px; border-radius: 16px; border: 1px solid var(--line); background: linear-gradient(180deg, var(--surface-1), color-mix(in srgb, var(--surface-2, var(--surface-1)) 60%, var(--surface-1))); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.07), 0 12px 26px -18px rgb(0 0 0 / 0.4); }
	.v-k .bars { display: flex; align-items: flex-end; gap: 4px; height: 56px; margin-top: 14px; padding-bottom: 6px; border-bottom: 1px solid var(--line); }
	.v-k .b { flex: 1; max-width: 22px; height: 100%; border-radius: 4px 4px 1px 1px; background: var(--ok); opacity: 0.85; }
	.v-k .b.fail { background: var(--fail); }
	.v-k .b.wait { background: var(--wait); height: 70%; }
	.v-k .b.idle { background: var(--text-faint); height: 30%; }
	.v-k .sub { margin-top: 8px; }

	/* L · Seal: round badge with the count */
	.v-l .card { align-items: center; text-align: center; gap: 4px; padding: 20px 16px; border-radius: 18px; border: 1px solid var(--line); background: radial-gradient(70% 60% at 50% 20%, color-mix(in srgb, var(--tint) 12%, transparent), transparent 70%), var(--surface-1); box-shadow: 0 12px 26px -18px rgb(0 0 0 / 0.4); }
	.v-l .seal { display: grid; place-items: center; width: 76px; height: 76px; margin-bottom: 8px; border-radius: 50%; background: radial-gradient(circle at 35% 30%, color-mix(in srgb, var(--tint) 60%, #fff), var(--tint)); box-shadow: 0 0 0 5px color-mix(in srgb, var(--tint) 18%, transparent), 0 0 0 6px color-mix(in srgb, var(--tint) 40%, transparent), 0 8px 18px -6px color-mix(in srgb, var(--tint) 60%, transparent); }
	.v-l .seal span { font: 800 18px/1 var(--font-ui); color: #fff; text-shadow: 0 1px 2px rgb(0 0 0 / 0.3); }

	/* M · Blueprint: grid paper, drafting lines */
	.v-m .card { padding: 16px; border-radius: 6px; border: 1px solid color-mix(in srgb, #4c8dff 45%, transparent); background: linear-gradient(color-mix(in srgb, #4c8dff 10%, transparent) 1px, transparent 1px) 0 0 / 14px 14px, linear-gradient(90deg, color-mix(in srgb, #4c8dff 10%, transparent) 1px, transparent 1px) 0 0 / 14px 14px, color-mix(in srgb, #0d2a55 30%, var(--surface-1)); }
	.v-m .ttl { font-family: var(--font-mono, ui-monospace, monospace); letter-spacing: 0.08em; text-transform: uppercase; font-size: 12px; }
	.v-m .val { font-family: var(--font-mono, ui-monospace, monospace); }
	.v-m .ico { border: 1px dashed color-mix(in srgb, #4c8dff 60%, transparent); border-radius: 4px; color: #6aa2ff; }
	.v-m .rows { display: flex; flex-direction: column; gap: 6px; margin-top: 12px; padding-top: 10px; border-top: 1px dashed color-mix(in srgb, #4c8dff 45%, transparent); }

	@media (max-width: 767px) {
		.deck { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
		.v-c.deck, .v-d.deck, .v-e.deck, .v-j.deck { grid-template-columns: minmax(0, 1fr); }
		.v-b .card, .v-f .card, .v-g .card, .v-h .card, .v-i .card, .v-k .card, .v-l .card, .v-m .card { padding: 12px; }
		.v-f .rows, .v-h .rows, .v-i .rows, .v-m .rows { display: none; }
		.val { font-size: 28px; }
	}
</style>
