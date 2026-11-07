<script lang="ts">
	import GhostEmpty from './GhostEmpty.svelte';
	// The Downloads app's Turned down list: the same full-bleed shell as the Downloads page
	// (a thin toolbar strip, a dense list that scrolls inside, a status bar). Same data and
	// Undo as the Library's list (ReleaseList); only the look follows Downloads.
	import { onMount } from 'svelte';
	import Icon from '../ui/Icon.svelte';
	import { tip } from './tip.ts';
	import { to } from '../core/paths.ts';
	import { api, message } from '#lib/core/api.ts';
	import { live } from '#lib/core/live.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { bytes, date } from '#lib/core/format.ts';
	import { facts, keyReasons, listedName, pager } from '#lib/mediafiles/index.ts';
	import { plainReason } from '#lib/core/plain.ts';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import type { ReleasePage, ReleaseView } from '#lib/core/types.ts';

	// Embedded: shown as a tab on the History page, which carries the heading and the way back.
	let { embedded = false }: { embedded?: boolean } = $props();
	let rows = $state<ReleaseView[]>([]);
	let total = $state(0);
	let offset = $state(0);
	const limit = 25;
	let loaded = $state(false);
	let error = $state('');
	let busy = $state(false);
	const pages = $derived(pager(total, limit, offset));

	async function load() {
		try {
			const p = await api.get<ReleasePage>(`/turned-down?limit=${limit}&offset=${offset}`);
			rows = p.releases;
			total = p.total;
			if (!rows.length && offset > 0 && total > 0) {
				offset = Math.max(0, offset - limit);
				return load();
			}
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}
	onMount(() => {
		void load();
		const offs = [live.on('releases', load), live.on('slot', load), live.on('resync', load)];
		return () => offs.forEach((off) => off());
	});

	async function undo(r: ReleaseView) {
		busy = true;
		try {
			await api.del(`/turned-down/${r.id}`);
			await load();
		} catch (e) {
			toasts.push("Couldn't take off list", 'fail', { body: message(e) });
		} finally {
			busy = false;
		}
	}
	async function undoAll() {
		if (!confirm(`Take all ${total} Releases off the Turned down list? HolyArrs may pick them again.`)) return;
		busy = true;
		try {
			await api.del(`/turned-down`);
			offset = 0;
			await load();
		} catch (e) {
			toasts.push("Couldn't clear list", 'fail', { body: message(e) });
		} finally {
			busy = false;
		}
	}
	function go(off: number | null) {
		if (off === null) return;
		offset = off;
		void load();
	}

	// Like Downloads: the app fills the column down to the bottom of the screen.
	let appEl = $state<HTMLElement>();
	let appH = $state(0);
	$effect(() => {
		if (!appEl) return;
		const fit = () => {
			const small = innerWidth <= 767;
			const tab = small ? parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--tabbar')) || 56 : 0;
			appH = Math.max(360, Math.floor(innerHeight - appEl!.getBoundingClientRect().top - tab));
		};
		fit();
		addEventListener('resize', fit);
		return () => removeEventListener('resize', fit);
	});
	let open = $state(0);
</script>

<div class="app" bind:this={appEl} style="--app-h: {appH}px" data-testid="td-app">
	{#if !embedded}<h1 class="sr">Turned down</h1>{/if}
	<div class="hdr">
		<div class="tdbar" role="toolbar" aria-label="Turned down actions">
			{#if !embedded}<a class="it stk" href={to('/downloads')} aria-label="Back to Downloads" use:tip={'Back to Downloads'} data-testid="td-back"><span class="ic"><Icon name="chevronLeft" size={20} /></span><span class="lb">Downloads</span></a>
			<span class="sep" aria-hidden="true"></span>
			<span class="it stk here" aria-current="page"><span class="ic"><Icon name="shield" size={20} /></span><span class="lb">Turned down</span></span>{/if}
			<span class="grow"></span>
			<button type="button" class="it stk" disabled={busy || total === 0} onclick={undoAll} use:tip={'Undo all: every copy may be picked again'} data-testid="td-undo-all"><span class="ic"><Icon name="x" size={20} /></span><span class="lb">Undo all</span></button>
		</div>
	</div>
	<div class="listbox">
		{#if error}
			<div class="msg"><LoadError hasData={false} message={plainReason(error).text} onretry={load} /></div>
		{:else if !loaded}
			<ul class="rows" aria-hidden="true">{#each Array(4) as _, i (i)}<li class="row sk"><Skeleton lines={2} /></li>{/each}</ul>
		{:else if total === 0}
			<GhostEmpty testid="td-empty" title="Nothing turned down" text="Releases HolyArrs skips or you reject show up here." />
		{:else}
			<div class="chead" aria-hidden="true"><span>Name</span><span class="c-for">For</span><span class="c-since">Since</span><span></span></div>
			<ul class="rows">
				{#each rows as r (r.id)}
					{@const key = keyReasons(r.reasons)}
					<li class="row" class:open={open === r.id} data-testid="listed-release">
						<div class="line">
							<button type="button" class="tg" aria-expanded={open === r.id} aria-label="Why {r.name} was turned down" onclick={() => (open = open === r.id ? 0 : r.id)} disabled={!key.length}><Icon name="chevronDown" size={14} stroke={2.2} /></button>
							<span class="nm"><b>{r.name}</b><span class="ft">{facts(r, bytes)}</span></span>
							<span class="c-for">{#if r.title_id}<a href={to(`/library/${r.title_id}`)} data-testid="listed-for">{listedName(r)}{r.version && r.version !== 'Main' ? ` · ${r.version}` : ''}</a>{/if}</span>
							<span class="c-since">{date(r.created_at)}</span>
							<button type="button" class="undo" onclick={() => undo(r)} disabled={busy} aria-label="Undo {r.name}" data-testid="listed-undo"><Icon name="x" size={14} /><span>Undo</span></button>
						</div>
						{#if open === r.id && key.length}
							<ul class="reasons">{#each key as x, i (i)}<li class={x.effect}>{x.text}</li>{/each}</ul>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</div>
	<div class="sbar"><div class="sbi">
		<span><Icon name="shield" size={14} /><b>{total}</b> {total === 1 ? 'copy' : 'copies'} turned down</span>
		<span class="grow"></span>
		{#if pages.prev !== null || pages.next !== null}
			<span class="pg">
				<button type="button" class="pgb" disabled={pages.prev === null} onclick={() => go(pages.prev)} aria-label="Previous page"><Icon name="chevronLeft" size={14} /></button>
				<span>{pages.from}–{pages.to} of {total}</span>
				<button type="button" class="pgb" disabled={pages.next === null} onclick={() => go(pages.next)} aria-label="Next page"><Icon name="chevronRight" size={14} /></button>
			</span>
		{/if}
	</div></div>
</div>

<style>
	.app { --g: 10px; --ez: cubic-bezier(0.32, 0.72, 0, 1); display: flex; flex-direction: column; height: var(--app-h, auto); min-height: 360px; min-width: 0; overflow: clip; }
	.sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
	.hdr { flex: none; padding: 3px var(--g); border-bottom: 1px solid var(--line); }
	.tdbar { display: flex; align-items: center; gap: 4px; min-width: 0; }
	.sep { flex: none; width: 1px; height: 30px; margin: 0 6px; background: var(--line); }
	.grow { flex: 1 1 0; }
	.it { display: inline-flex; flex: none; flex-direction: column; align-items: center; justify-content: center; gap: 2px; min-width: 52px; height: 44px; padding: 3px 8px; border: 0; border-radius: var(--r-btn); background: none; color: var(--text); text-decoration: none; cursor: pointer; transition: background var(--dur-fast) var(--ease-out), transform 180ms var(--ez); }
	.it .ic { display: grid; place-items: center; transition: transform 180ms var(--ez); }
	.it .lb { font: 500 11px/1.1 var(--font-ui); color: var(--text-muted); white-space: nowrap; }
	.it:hover:not(:disabled):not(.here) { background: var(--surface-3); }
	.it:hover:not(:disabled):not(.here) .ic { transform: translateY(-1px); }
	.it:active:not(:disabled):not(.here) { transform: scale(0.96); }
	.it:disabled { opacity: 0.45; cursor: not-allowed; }
	.it:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.it.here { cursor: default; background: var(--surface-3); }
	.it.here .ic { color: var(--accent-text, var(--text)); }
	.it.here .lb { color: var(--text); font-weight: 600; }

	.listbox { flex: 1 1 auto; min-height: 0; overflow: auto; overscroll-behavior: contain; }
	.chead, .line { display: grid; grid-template-columns: 24px minmax(0, 1fr) minmax(0, 220px) 96px auto; align-items: center; gap: 10px; padding: 0 var(--g); }
	.chead { position: sticky; top: 0; z-index: 2; height: 28px; border-bottom: 1px solid var(--line); background: var(--bg); color: var(--text-muted); font: 600 11px/1 var(--font-ui); }
	.chead span:first-child { grid-column: 2; }
	.rows { margin: 0; padding: 0; list-style: none; }
	.row { border-bottom: 1px solid var(--line); }
	.row.sk { padding: 8px var(--g); }
	.line { min-height: 44px; font: var(--t-footnote); }
	.row:hover > .line { background: var(--surface-2); }
	.nm { display: grid; min-width: 0; }
	.nm b { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: 600; }
	.ft { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); font: var(--t-caption); font-variant-numeric: tabular-nums; }
	.c-for { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.c-for a { color: var(--text); }
	.c-for a:hover { color: var(--accent-text, var(--text)); }
	.c-since { color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.tg { display: grid; place-items: center; width: 24px; height: 24px; padding: 0; border: 0; border-radius: 6px; background: none; color: var(--text-muted); cursor: pointer; }
	.tg :global(svg) { transform: rotate(-90deg); transition: transform 220ms var(--ez); }
	.tg[aria-expanded='true'] :global(svg) { transform: none; }
	.tg:disabled { opacity: 0.3; cursor: default; }
	.tg:hover:not(:disabled) { background: var(--surface-3); color: var(--text); }
	.undo { display: inline-flex; align-items: center; gap: 4px; height: 28px; padding: 0 10px; border: 0; border-radius: var(--r-btn); background: none; color: var(--text); font: var(--t-footnote); cursor: pointer; transition: background var(--dur-fast) var(--ease-out), transform 180ms var(--ez); }
	.undo:hover:not(:disabled) { background: var(--surface-3); }
	.undo:active:not(:disabled) { transform: scale(0.96); }
	.undo:disabled { opacity: 0.45; cursor: not-allowed; }
	.tg:focus-visible, .undo:focus-visible, .pgb:focus-visible, .c-for a:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.reasons { margin: 0; padding: 2px var(--g) 10px calc(var(--g) + 34px); list-style: none; font: var(--t-footnote); color: var(--text-muted); }
	.reasons li { padding: 2px 0 2px 10px; border-left: 2px solid var(--line-strong); }
	.reasons li.fail { border-left-color: var(--fail); }
	.reasons li.hold, .reasons li.warn { border-left-color: var(--wait); }
	.msg { padding: 16px var(--g); }

	.sbar { flex: none; border-top: 1px solid var(--line); font-variant-numeric: tabular-nums; }
	.sbi { display: flex; align-items: center; gap: 14px; padding: 3px var(--g); min-width: 0; overflow: hidden; white-space: nowrap; font-size: 11px; line-height: 1.3; }
	.sbi > span { display: inline-flex; align-items: center; gap: 6px; }
	.pgb { display: grid; place-items: center; width: 22px; height: 22px; padding: 0; border: 0; border-radius: 6px; background: none; color: var(--text); cursor: pointer; }
	.pgb:hover:not(:disabled) { background: var(--surface-3); }
	.pgb:disabled { opacity: 0.4; cursor: not-allowed; }

	@media (max-width: 767px) {
		.chead { display: none; }
		.line { grid-template-columns: 24px minmax(0, 1fr) auto; grid-template-areas: 'tg nm undo' '. for for'; row-gap: 0; padding-top: 6px; padding-bottom: 6px; }
		.tg { grid-area: tg; }
		.nm { grid-area: nm; }
		.undo { grid-area: undo; }
		.c-for { grid-area: for; color: var(--text-muted); font: var(--t-caption); }
		.c-since { display: none; }
		.undo span { display: none; }
		.undo { width: 32px; padding: 0; justify-content: center; }
		.reasons { padding-left: calc(var(--g) + 10px); }
	}
	@media (prefers-reduced-motion: reduce) {
		.it, .it .ic, .undo, .tg :global(svg) { transition: none; }
		.it:active:not(:disabled), .undo:active:not(:disabled) { transform: none; }
	}
	@media (max-width: 640px) {
		.tg, .pgb { width: 32px; height: 32px; }
		.undo { height: 32px; }
	}
</style>
