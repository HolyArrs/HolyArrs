<script lang="ts">
	// Linked apps: a tab per kind with its count and worst colour, and the picked kind's rows beside it.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import GroupRows from './GroupRows.svelte';
	import { partKinds, type StatusGroup, type StatusRow, type Tone } from './rows.ts';
	import { to } from '../core/paths.ts';

	// Icon path for each kind's tab.
	const icon: Record<string, string> = {
		sources: 'M4 6h12M4 10h12M4 14h8',
		downloaders: 'M10 3v10m0 0-4-4m4 4 4-4M4 16h12',
		media: 'M3 4h14v9H3zM7 16h6',
		subtitles: 'M3 5h14v10H3zM6 11h3m2 0h3',
		lists: 'M6 5h10M6 10h10M6 15h10M3 5h.01M3 10h.01M3 15h.01',
		alerts: 'M10 3a5 5 0 0 0-5 5v3l-2 3h14l-2-3V8a5 5 0 0 0-5-5zM8 16a2 2 0 0 0 4 0',
		metadata: 'M10 3c4 0 7 1.3 7 3s-3 3-7 3-7-1.3-7-3 3-3 7-3zM3 6v8c0 1.7 3 3 7 3s7-1.3 7-3V6'
	};

	let { kinds, canAct, busy, oncheck }: { kinds: StatusGroup[]; canAct: boolean; busy: (key: string) => boolean; oncheck: (r: StatusRow) => void } = $props();

	// Every kind keeps its tab, added or not, so the plate holds one shape.
	let picked = $state<string | null>(null);
	let list = $state<HTMLElement>();

	const all = $derived(kinds.reduce((n, g) => n + onOf(g), 0));
	const allOk = $derived(kinds.reduce((n, g) => n + okOf(g), 0));

	// Opens on the first kind with a problem, else the first kind.
	const current = $derived(kinds.find((k) => k.id === picked) ?? kinds.find((k) => worst(k) === 'fail') ?? kinds.find((k) => worst(k) === 'wait') ?? kinds[0]);

	// Arrow keys move in both directions because the list turns sideways on narrow screens.
	function kindKey(e: KeyboardEvent) {
		const i = kinds.findIndex((k) => k.id === current.id);
		const fwd = e.key === 'ArrowDown' || e.key === 'ArrowRight';
		const back = e.key === 'ArrowUp' || e.key === 'ArrowLeft';
		const n = fwd ? (i + 1) % kinds.length : back ? (i - 1 + kinds.length) % kinds.length : e.key === 'Home' ? 0 : e.key === 'End' ? kinds.length - 1 : -1;
		if (n < 0 || n === i) return;
		e.preventDefault();
		picked = kinds[n].id;
		list?.querySelector<HTMLElement>(`#pk-tab-${kinds[n].id}`)?.focus();
	}

	const meta = (g: StatusGroup) => partKinds.find((k) => k.id === g.id)!;
	const okOf = (g: StatusGroup) => g.rows.filter((r) => r.tone === 'ok').length;
	// Rows turned off on purpose are not missing, so they stay out of the count.
	const isOff = (r: StatusRow) => r.word === t('health.run.off');
	const onOf = (g: StatusGroup) => g.rows.filter((r) => !isOff(r)).length;
	const count = (g: StatusGroup) => (!g.rows.length ? '–' : onOf(g) ? `${okOf(g)}/${onOf(g)}` : t('health.run.off'));
	// A failing part wins, then a waiting one, then working; none when empty.
	const worst = (g: StatusGroup): Tone | 'none' =>
		g.rows.some((r) => r.tone === 'fail') ? 'fail' : g.rows.some((r) => r.tone === 'wait') ? 'wait' : g.rows.length ? (g.rows.every((r) => r.tone === 'idle') ? 'idle' : 'ok') : 'none';
	// A kind in a few words, like "3 working" or "Nothing added yet".
	function line(g: StatusGroup): string {
		if (!g.rows.length) return t('health.parts.nothing');
		const bad = g.rows.filter((r) => r.tone === 'fail' || r.tone === 'wait').length;
		if (bad) return t(bad === 1 ? 'health.parts.needsOne' : 'health.parts.needsMany', { count: bad });
		if (g.rows.length === 1) return g.rows[0].word;
		if (g.rows.every(isOff)) return t('health.parts.off', { count: g.rows.length });
		const ok = okOf(g);
		return ok ? t('health.parts.working', { count: ok }) : t('health.parts.waiting', { count: onOf(g) });
	}
</script>

<section class="st-plate" aria-labelledby="p-rail" data-testid="status-parts">
	<h2 id="p-rail">{t('health.parts.title')}<span class="cnt">{allOk}/{all}</span></h2>
	<div class="rl">
		<div class="kinds" role="tablist" aria-label={t('health.parts.kinds')} tabindex="-1" bind:this={list} onkeydown={kindKey}>
			{#each kinds as g (g.id)}
				<button type="button" role="tab" id="pk-tab-{g.id}" aria-selected={current.id === g.id} aria-controls="pk-panel" tabindex={current.id === g.id ? 0 : -1} data-kind={g.id} onclick={() => (picked = g.id)}>
					<span class="gi" aria-hidden="true"><svg viewBox="0 0 20 20"><path d={icon[g.id]} /></svg></span>
					<span class="kl">{g.label}</span>
					<span class="kc">{count(g)}</span>
					{#if worst(g) === 'fail' || worst(g) === 'wait'}
						<span class="ex {worst(g)}" aria-hidden="true">!</span><span class="sr-only">{line(g)}</span>
					{:else}
						<i class="st-dot {worst(g)}" aria-hidden="true"></i>
					{/if}
				</button>
			{/each}
		</div>
		<div class="pane" role="tabpanel" id="pk-panel" aria-labelledby="pk-tab-{current.id}">
			<h3>
				{current.label}<span class="pl">{line(current)}</span>
				{#if canAct && current.rows.length}<span class="end"><Button variant="plain" size="sm" href={to(meta(current).add)}>{t('health.parts.manage')}</Button></span>{/if}
			</h3>
			{#if current.rows.length}
				{#key current.id}<GroupRows rows={current.rows} {canAct} {busy} {oncheck} />{/key}
			{:else}
				<p class="emp">
					<i class="st-dot none" aria-hidden="true"></i>{t('health.parts.noneAdded', { kind: meta(current).one })}
					{#if canAct}<Button variant="plain" size="sm" href={to(meta(current).add)}>{t('health.parts.add')}</Button>{/if}
				</p>
			{/if}
		</div>
	</div>
</section>

<style>
	.gi { display: grid; place-items: center; flex: none; width: 26px; height: 26px; border-radius: 8px; background: var(--surface-2, var(--surface-1)); color: var(--text-muted); }
	.gi svg { width: 15px; height: 15px; fill: none; stroke: currentColor; stroke-width: 1.7; stroke-linecap: round; stroke-linejoin: round; }
	.ex { display: grid; place-items: center; flex: none; width: 16px; height: 16px; border-radius: 50%; background: var(--fail); color: #fff; font: 700 11px/1 var(--font-ui); }
	.ex.wait { background: var(--wait); }
	.emp { display: flex; align-items: center; gap: 10px; margin: 0; min-height: 44px; font-size: 14px; color: var(--text-muted); }
	.end { margin-left: auto; }

	.rl { display: grid; grid-template-columns: 240px minmax(0, 1fr); gap: 0 24px; }
	/* The list takes focus only through its tabs. */
	.kinds:focus { outline: none; }
	.kinds { display: flex; flex-direction: column; gap: 2px; padding-right: 20px; border-right: 1px solid var(--line); }
	.kinds button { display: flex; align-items: center; gap: 10px; min-height: 44px; padding: 0 10px 0 8px; border: 0; border-radius: 10px; background: none; color: var(--text-muted); font: 500 14px/1 var(--font-ui); text-align: left; cursor: pointer; }
	.kinds button:hover { color: var(--text); }
	.kinds button[aria-selected='true'] { background: var(--seg-thumb); color: var(--text); box-shadow: 0 0 0 1px var(--line-strong); }
	.kinds button:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.kl { flex: 1 1 auto; min-width: 0; overflow-wrap: anywhere; line-height: 1.25; }
	.kc { font-size: 12px; font-variant-numeric: tabular-nums; }
	.pane h3 { display: flex; align-items: center; gap: 10px; margin: 0 0 12px; min-height: 34px; font: 600 15px/1.2 var(--font-ui); color: var(--text); }
	.pane .pl { font-size: 13px; font-weight: 400; color: var(--text-muted); }

	/* Tabs turn into a row above the pane. */
	@media (max-width: 900px) {
		.rl { grid-template-columns: minmax(0, 1fr); gap: 14px; }
		.kinds { flex-direction: row; overflow-x: auto; padding: 0 0 10px; border-right: 0; border-bottom: 1px solid var(--line); scrollbar-width: none; }
		.kinds button { flex: none; min-height: 40px; }
		.kinds .kc { display: none; }
		.kl { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	}
	@media (max-width: 767px) and (pointer: coarse) {
		.kinds button { min-height: 44px; }
	}
</style>
