<script lang="ts">
	import { to } from './paths.ts';
	// The Held and Turned down lists: paged, remove one or clear all.
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { bytes, date, dateTime } from '#lib/format.ts';
	import { facts, keyReasons, listedName, pager } from '#lib/releases.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import { plainReason } from '#lib/plain.ts';
	import type { ReleasePage, ReleaseView } from '#lib/types.ts';

	let { list }: { list: 'held' | 'turned-down' } = $props();
	const held = $derived(list === 'held');

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

	onMount(() => {
		void load();
		const offs = [live.on('releases', load), live.on('slot', load), live.on('resync', load)];
		return () => offs.forEach((off) => off());
	});

	async function removeOne(r: ReleaseView) {
		busy = true;
		try {
			await api.del(`/${list}/${r.id}`);
			toasts.push(
				held ? `${r.name} is no longer held; ${listedName(r) || 'it'} goes back to looking.` : `${r.name} may be picked again.`,
				'ok'
			);
			await load();
		} catch (e) {
			toasts.push(message(e), 'fail');
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
			const r = await api.del<{ removed: number }>(`/${list}`);
			toasts.push(`${r.removed} ${r.removed === 1 ? 'Release' : 'Releases'} taken off the list.`, 'ok');
			offset = 0;
			await load();
		} catch (e) {
			toasts.push(message(e), 'fail');
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

<div class="page">
<PageHeader
	title={held ? 'Held' : 'Turned down'}
	subtitle={held
		? 'Releases HolyArrs picked that are waiting out their wait. A better one found meanwhile takes their place.'
		: 'Releases HolyArrs never picks again for their movie or episode. Take one off the list to let it be picked again.'}
	back={{ href: to('/library'), label: 'Library' }}
>
	{#snippet actions()}
		<nav class="switch-list" aria-label="Release lists">
			<a href={to(`/library/held`)} aria-current={held ? 'page' : undefined}>Held</a>
			<a href={to(`/library/turned-down`)} aria-current={held ? undefined : 'page'}>Turned down</a>
		</nav>
	{/snippet}
</PageHeader>

{#if error}
	<LoadError hasData={false} message={plainReason(error).text} onretry={load} />
{:else if !loaded}
	<ul class="rels" aria-hidden="true">{#each Array(3) as _, i (i)}<li class="rel"><Skeleton lines={3} /></li>{/each}</ul>
{:else if total === 0}
	<EmptyState
		icon={held ? 'clock' : 'shield'}
		title={held ? 'Nothing is held' : 'Nothing is turned down'}
		text={held
			? 'When Pick rules ask HolyArrs to wait before sending a Release, it waits here.'
			: 'Turn a Release down from a movie or show page, or HolyArrs does it when a download turns out wrong.'}
	/>
{:else}
	<div class="list-head">
		<span class="count">{total} {total === 1 ? 'Release' : 'Releases'}</span>
		<Button variant="plain" onclick={clearAll} disabled={busy}>Clear all</Button>
	</div>
	<ul class="rels">
		{#each rows as r (r.id)}
			{@const key = keyReasons(r.reasons)}
			<li class="rel" data-testid="listed-release">
				<div class="rel-top">
					<div class="grow">
						{#if r.title_id}
							<a class="for" href={to(`/library/${r.title_id}`)} data-testid="listed-for">{listedName(r)}{r.version && r.version !== 'Main' ? ` · ${r.version}` : ''}</a>
						{/if}
						<div class="rel-name">{r.name}</div>
					</div>
					<Button variant="secondary" icon="x" onclick={() => removeOne(r)} disabled={busy} aria-label="{held ? 'Stop holding' : 'Take off the list'} {r.name}">
						{held ? 'Stop holding' : 'Take off the list'}
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
		<nav class="pager" aria-label="Pages">
			<Button variant="secondary" icon="chevronLeft" disabled={pages.prev === null} onclick={() => go(pages.prev)}>Back</Button>
			<span class="count">{pages.from} to {pages.to} of {total}</span>
			<Button variant="secondary" iconAfter="chevronRight" disabled={pages.next === null} onclick={() => go(pages.next)}>More</Button>
		</nav>
	{/if}
{/if}
</div>

<style>
	.page { display: flex; flex-direction: column; gap: 16px; padding-bottom: 48px; min-width: 0; }
	.switch-list { display: flex; gap: 2px; padding: 3px; border-radius: var(--r-btn); background: var(--surface-2); }
	.switch-list a { display: inline-flex; align-items: center; min-height: 38px; padding: 0 14px; border-radius: 8px; color: var(--text-muted); font: var(--t-subhead); font-size: 14px; transition: background var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out); }
	.switch-list a:hover { color: var(--text); }
	.switch-list a:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.switch-list a[aria-current='page'] { background: var(--surface-1); color: var(--text); font-weight: 600; box-shadow: var(--shadow-card); }
	@media (pointer: coarse) { .switch-list a { min-height: 44px; } }
	.list-head { display: flex; justify-content: space-between; align-items: center; gap: 12px; }
	.count { color: var(--text-muted); font: var(--t-subhead); font-variant-numeric: tabular-nums; }
	.rels { margin: 0; padding: 4px; list-style: none; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: var(--shadow-card); }
	.rel { padding: 12px 14px; }
	.rel + .rel { border-top: 1px solid var(--line); }
	.rel-top { display: flex; justify-content: space-between; gap: 12px; align-items: flex-start; margin-bottom: 4px; }
	.grow { min-width: 0; flex: 1; }
	.for { font: var(--t-headline); color: var(--text); }
	.rel-name { margin-top: 2px; color: var(--text-muted); font: var(--t-callout); overflow-wrap: anywhere; }
	.facts { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.reasons { margin: 8px 0 0; padding: 0; list-style: none; font: var(--t-footnote); color: var(--text-muted); }
	.reasons li { padding: 2px 0 2px 10px; border-left: 2px solid var(--line-strong); }
	.reasons li.fail { border-left-color: var(--fail); }
	.reasons li.hold, .reasons li.warn { border-left-color: var(--wait); }
	.pager { display: flex; justify-content: space-between; align-items: center; gap: 12px; }
	@media (max-width: 600px) { .rel-top { flex-direction: column; } }
</style>
