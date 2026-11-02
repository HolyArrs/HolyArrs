<script lang="ts">
	// Every release, at the foot of the Updates tab: the newest one open with its notes, older ones
	// folded to one line each, and only three shown until "Show more". The app
	// knows just the current and newest release today; ?releases=demo fills in
	// a made-up history so the many-release case can be seen.
	import './plate.css';
	import { page } from '$app/state';
	import { ago } from '../format.ts';

	type Update = { on: boolean; version: string; latest?: string; newer: boolean; link?: string; notes?: string; checked_at?: string; problem?: string; no_release?: boolean };
	type Rel = { version: string; date?: string; notes?: string; link?: string };

	let { upd }: { upd: Update | null } = $props();

	const STEP = 3;
	let shown = $state(STEP);
	let open = $state<Record<string, boolean>>({});

	const demo: Rel[] = [
		{ version: '1.4.0', date: '2026-10-30', notes: 'Status page with Health, Parts and Jobs.\nRequests page shows live steps for each ask.\nHistory keeps sign-ins and settings changes for a year.' },
		{ version: '1.3.2', date: '2026-10-21', notes: 'Fixed a stuck download that never moved to import.\nFaster first load of Explore.' },
		{ version: '1.3.1', date: '2026-10-14', notes: 'Fixed search for titles with an apostrophe.' },
		{ version: '1.3.0', date: '2026-10-02', notes: 'Music library and artist pages.\nNew free-space check for every library folder.' },
		{ version: '1.2.4', date: '2026-09-20', notes: 'Smaller backups.\nFixed time zones in the calendar.' },
		{ version: '1.2.0', date: '2026-09-04', notes: 'Media server links for Jellyfin and Plex.' },
		{ version: '1.1.0', date: '2026-08-15', notes: 'Subtitles from outside sites.' },
		{ version: '1.0.0', date: '2026-07-30', notes: 'First release.' }
	];

	const list = $derived.by((): Rel[] => {
		if (page.url.searchParams.get('releases') === 'demo') return demo;
		if (!upd) return [];
		const out: Rel[] = [];
		if (upd.newer && upd.latest) out.push({ version: upd.latest, notes: upd.notes, link: upd.link, date: upd.checked_at });
		out.push({ version: upd.version, notes: upd.newer ? undefined : upd.notes, link: upd.newer ? undefined : upd.link });
		return out;
	});
	const current = $derived(page.url.searchParams.get('releases') === 'demo' ? '1.3.2' : upd?.version);
	const isOpen = (r: Rel, i: number) => open[r.version] ?? i === 0;
</script>

<section class="st-plate" aria-labelledby="s-rel">
	<h2 id="s-rel">All releases{#if list.length > 1}<span class="cnt">{list.length}</span>{/if}</h2>

	<ol class="rels">
		{#each list.slice(0, shown) as r, i (r.version)}
			{@const o = isOpen(r, i)}
			<li class="rel" class:open={o}>
				<button type="button" class="rh" aria-expanded={o} onclick={() => (open[r.version] = !o)} disabled={!r.notes}>
					<span class="chev" aria-hidden="true">{#if r.notes}<svg viewBox="0 0 20 20"><path d="M7 5l5 5-5 5" /></svg>{/if}</span>
					<span class="rv">v{r.version}</span>
					{#if i === 0}<span class="pill new">Latest</span>{/if}
					{#if r.version === current}<span class="pill cur">Yours</span>{/if}
					{#if r.date}<span class="rd">{ago(r.date)}</span>{/if}
				</button>
				{#if o && r.notes}
					<div class="body">
						<ul class="notes">{#each r.notes.split('\n').filter(Boolean) as line}<li>{line}</li>{/each}</ul>
						{#if r.link}<a class="btn" href={r.link} target="_blank" rel="noopener noreferrer">Release page</a>{/if}
					</div>
				{/if}
			</li>
		{/each}
	</ol>
	{#if list.length > shown}
		<button type="button" class="more" onclick={() => (shown += STEP)}>Show {Math.min(STEP, list.length - shown)} more · {list.length - shown} older</button>
	{:else if shown > STEP}
		<button type="button" class="more" onclick={() => (shown = STEP)}>Show fewer</button>
	{/if}
</section>

<style>
	.cnt { padding: 2px 8px; border-radius: var(--r-pill); background: var(--surface-2, var(--surface-1)); font-size: 12px; color: var(--text-muted); }
	.rels { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
	.rel { border: 1px solid var(--line); border-radius: 12px; background: var(--surface-1); overflow: hidden; }
	.rel.open { border-color: var(--line-strong); box-shadow: 0 8px 20px -14px rgb(0 0 0 / 0.35); }
	.rh {
		display: flex; align-items: center; gap: 10px; width: 100%; min-height: 48px; padding: 0 14px; border: 0; background: none;
		color: var(--text); font: inherit; text-align: left; cursor: pointer;
	}
	.rh:disabled { cursor: default; }
	.rh:focus-visible { outline: none; box-shadow: inset var(--focus-ring); }
	.chev { display: grid; place-items: center; width: 18px; color: var(--text-muted); }
	.chev svg { width: 16px; height: 16px; fill: none; stroke: currentColor; stroke-width: 1.8; stroke-linecap: round; stroke-linejoin: round; transition: transform 160ms var(--ease-out); }
	.rel.open .chev svg { transform: rotate(90deg); }
	.rv { font: 700 15.5px/1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.rd { margin-left: auto; font-size: 12.5px; color: var(--text-muted); }
	.pill { padding: 3px 8px; border-radius: var(--r-pill); font-size: 11.5px; font-weight: 600; }
	.pill.new { background: color-mix(in srgb, var(--ok) 20%, transparent); color: var(--text); }
	.pill.cur { background: color-mix(in srgb, var(--accent, var(--info)) 18%, transparent); color: var(--text); }
	.body { display: flex; flex-direction: column; gap: 12px; padding: 4px 14px 14px 42px; border-top: 1px solid var(--line); }
	.notes { margin: 10px 0 0; padding-left: 18px; font-size: 14px; line-height: 1.6; color: var(--text); }
	.btn, .more {
		display: inline-flex; align-items: center; align-self: flex-start; min-height: 40px; padding: 0 14px; border: 1px solid var(--line);
		border-radius: var(--r-pill); background: var(--surface-2, var(--surface-1)); color: var(--text); font: 500 13.5px/1 var(--font-ui);
		text-decoration: none; cursor: pointer;
	}
	.more { margin-top: 12px; width: 100%; justify-content: center; border-style: dashed; color: var(--text-muted); }
	.more:hover, .btn:hover { border-color: var(--line-strong); color: var(--text); }
	.btn:focus-visible, .more:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (max-width: 767px) {
		.body { padding-left: 14px; }
	}
	@media (prefers-reduced-motion: reduce) { .chev svg { transition: none; } }
</style>
