<script lang="ts">
	import { to } from './core/paths.ts';
	// The Held and Turned down lists: paged, remove one or clear all.
	import { onMount } from 'svelte';
	import { api, message } from '#lib/core/api.ts';
	import { live } from '#lib/core/live.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { bytes, date, dateTime } from '#lib/core/format.ts';
	import { facts, keyReasons, listedName, pager } from '#lib/releases.ts';
	import LibraryToolPage from '#lib/LibraryToolPage.svelte';
	import Poster from '#lib/ui/Poster.svelte';
	import { titleArt } from '#lib/title-art.ts';
	import Button from '#lib/ui/Button.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import { plainReason } from '#lib/core/plain.ts';
	import type { ReleasePage, ReleaseView } from '#lib/core/types.ts';

	// from: the Library's lists (Held and Turned down side by side) or the Downloads page's Turned down list (Undo).
	let { list, from = 'library' }: { list: 'held' | 'turned-down'; from?: 'library' | 'downloads' } = $props();
	const held = $derived(list === 'held');
	const dl = $derived(from === 'downloads');
	const undoWord = $derived(held ? 'Stop holding' : dl ? 'Undo' : 'Take off');

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
			const p = await api.get<ReleasePage>(`/${list}?limit=${limit}&offset=${offset}`);
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

	// Posters for the rows, from one brief read of the Library.
	let art = $state<Map<number, string>>(new Map());
	onMount(() => {
		void load();
		if (from === 'library') void titleArt().then((m) => (art = m));
		const offs = [live.on('releases', load), live.on('slot', load), live.on('resync', load)];
		return () => offs.forEach((off) => off());
	});

	async function removeOne(r: ReleaseView) {
		busy = true;
		try {
			await api.del(`/${list}/${r.id}`);
			await load();
		} catch (e) {
			toasts.push(`Couldn't take off list`, 'fail', { body: message(e) });
		} finally {
			busy = false;
		}
	}

	async function clearAll() {
		const ask = held
			? `Stop holding all ${total} Releases? Their movies and episodes go back to looking.`
			: `Take all ${total} Releases off the Turned down list? HolyArrs may pick them again.`;
		if (!confirm(ask)) return;
		busy = true;
		try {
			await api.del<{ removed: number }>(`/${list}`);
			offset = 0;
			await load();
		} catch (e) {
			toasts.push(`Couldn't take off list`, 'fail', { body: message(e) });
		} finally {
			busy = false;
		}
	}

	function go(off: number | null) {
		if (off === null) return;
		offset = off;
		void load();
	}
</script>

<LibraryToolPage title={held ? 'Held' : 'Turned down'} count={loaded && !error && total ? `${total} ${total === 1 ? 'Release' : 'Releases'}` : ''}>
	{#snippet bar()}
		{#if !dl}
			<nav class="tp-cap switch-list" aria-label="Release lists">
				<a href={to(`/library/held`)} aria-current={held ? 'page' : undefined}>Held</a>
				<a href={to(`/library/turned-down`)} aria-current={held ? undefined : 'page'}>Turned down</a>
			</nav>
		{/if}
		{#if loaded && !error && total > 0}
			<span class="tp-gap"></span>
			<span class="tp-cap"><Button variant="secondary" onclick={clearAll} disabled={busy}>Clear all</Button></span>
		{/if}
	{/snippet}

{#if error}
	<LoadError hasData={false} message={plainReason(error).text} onretry={load} />
{:else if !loaded}
	<ul class="tp-sheet tp-rows" aria-hidden="true">{#each Array(3) as _, i (i)}<li><Skeleton lines={3} /></li>{/each}</ul>
{:else if total === 0}
	<EmptyState
		icon={held ? 'clock' : 'shield'}
		title={held ? 'Nothing held.' : 'Nothing turned down.'}
		text={held
			? 'Waiting out a Pick rule delay.'
			: 'Releases you turn down and wrong downloads land here.'}
	/>
{:else}
	<ul class="tp-sheet tp-rows">
		{#each rows as r (r.id)}
			{@const key = keyReasons(r.reasons)}
			<li class="rel" data-testid="listed-release">
				<div class="rel-top">
					{#if from === 'library' && r.title_id}<span class="tp-art" style="--poster-ratio: 2 / 3"><Poster src={art.get(r.title_id)} name={listedName(r)} /></span>{/if}
					<div class="grow">
						{#if r.title_id}
							<a class="for" href={to(`/library/${r.title_id}`)} data-testid="listed-for">{listedName(r)}{r.version && r.version !== 'Main' ? ` · ${r.version}` : ''}</a>
						{/if}
						<div class="rel-name">{r.name}</div>
					</div>
					<Button variant="secondary" icon="x" onclick={() => removeOne(r)} disabled={busy} aria-label="{undoWord} {r.name}" data-testid="listed-undo">
						{undoWord}
					</Button>
				</div>
				<div class="facts">
					{facts(r, bytes)}
					{#if held && r.hold_until} · sent {dateTime(r.hold_until)}{/if}
					{#if !held} · since {date(r.created_at)}{/if}
				</div>
				{#if key.length}
					<ul class="reasons">
						{#each key as x, i (i)}<li class={x.effect}>{x.text}</li>{/each}
					</ul>
				{/if}
			</li>
		{/each}
	</ul>
	{#if pages.prev !== null || pages.next !== null}
		<nav class="tp-foot" aria-label="Pages">
			<Button variant="secondary" icon="chevronLeft" disabled={pages.prev === null} onclick={() => go(pages.prev)}>Back</Button>
			<span class="tp-note">{pages.from} to {pages.to} of {total}</span>
			<Button variant="secondary" iconAfter="chevronRight" disabled={pages.next === null} onclick={() => go(pages.next)}>More</Button>
		</nav>
	{/if}
{/if}
</LibraryToolPage>

<style>
	.switch-list a { display: inline-flex; align-items: center; height: 44px; padding: 0 14px; border-radius: 10px; color: var(--text-muted); font: 600 13.5px/1 var(--font-ui); white-space: nowrap; transition: background var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out); }
	.switch-list a:hover { color: var(--text); background: color-mix(in srgb, var(--text) 7%, transparent); }
	.switch-list a:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.switch-list a[aria-current='page'] { background: var(--surface-1); color: var(--text); box-shadow: var(--shadow-card); }
	.switch-list a[aria-current='page']:focus-visible { box-shadow: var(--focus-ring); }
	.rel-top { display: flex; justify-content: space-between; gap: 12px; align-items: flex-start; margin-bottom: 4px; }
	.grow { min-width: 0; flex: 1; }
	.for { font: var(--t-headline); color: var(--text); }
	/* On touch phones the title link's box is 44px tall (inline padding moves nothing). */
	@media (max-width: 767px) and (pointer: coarse) { .for { display: inline-block; padding-block: 12px; } }
	.rel-name { margin-top: 2px; color: var(--text-muted); font: var(--t-callout); overflow-wrap: anywhere; }
	.facts { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.reasons { margin: 8px 0 0; padding: 0; list-style: none; font: var(--t-footnote); color: var(--text-muted); }
	.reasons li { padding: 2px 0 2px 10px; border-left: 2px solid var(--line-strong); }
	.reasons li.fail { border-left-color: var(--fail); }
	.reasons li.hold, .reasons li.warn { border-left-color: var(--wait); }
	@media (max-width: 600px) { .rel-top { flex-direction: column; } .switch-list a { flex: 1; justify-content: center; } }
</style>
