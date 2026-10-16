<script lang="ts">
	import { to } from './paths.ts';
	// The Held and Turned down lists: paged, remove one or clear all.
	import { onMount } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { bytes, date, dateTime } from '#lib/format.ts';
	import { facts, keyReasons, listedName, pager } from '#lib/releases.ts';
	import EmptyState from '#lib/EmptyState.svelte';
	import Icon from '#lib/Icon.svelte';
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

<header class="page-head">
	<div>
		<h1>{held ? 'Held' : 'Turned down'}</h1>
		<p>
			{held
				? 'Releases HolyArrs picked that are waiting out their wait. A better one found meanwhile takes their place.'
				: 'Releases HolyArrs never picks again for their movie or episode. Take one off the list to let it be picked again.'}
		</p>
	</div>
	<nav class="row switch-list" aria-label="Release lists">
		<a class="btn btn-quiet" href={to(`/library/held`)} aria-current={held ? 'page' : undefined}>Held</a>
		<a class="btn btn-quiet" href={to(`/library/turned-down`)} aria-current={held ? undefined : 'page'}>Turned down</a>
	</nav>
</header>

{#if error}
	<p class="notice bad" role="alert">{error}</p>
{:else if !loaded}
	<div class="sk" style="height: 160px"></div>
{:else if total === 0}
	<EmptyState icon={held ? 'calendar' : 'shield'} title={held ? 'Nothing is held' : 'Nothing is turned down'}>
		<p>
			{held
				? 'When Pick rules ask HolyArrs to wait before sending a Release, it waits here.'
				: 'Turn a Release down from a movie or show page, or HolyArrs does it when a download turns out wrong.'}
		</p>
	</EmptyState>
{:else}
	<div class="list-head">
		<span class="small muted">{total} {total === 1 ? 'Release' : 'Releases'}</span>
		<button class="btn btn-ghost" type="button" onclick={clearAll} disabled={busy}>Clear all</button>
	</div>
	<ul class="card rels">
		{#each rows as r (r.id)}
			{@const key = keyReasons(r.reasons)}
			<li class="rel" data-testid="listed-release">
				<div class="rel-top">
					<div class="grow">
						{#if r.title_id}
							<a class="small" href={to(`/library/${r.title_id}`)} data-testid="listed-for">{listedName(r)}{r.version && r.version !== 'Main' ? ` · ${r.version}` : ''}</a>
						{/if}
						<div class="rel-name mono">{r.name}</div>
					</div>
					<button class="btn btn-quiet" type="button" onclick={() => removeOne(r)} disabled={busy} aria-label="{held ? 'Stop holding' : 'Take off the list'} {r.name}">
						<Icon name="x" size={16} />{held ? 'Stop holding' : 'Take off the list'}
					</button>
				</div>
				<div class="xs muted">
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
		<nav class="row pager" aria-label="Pages">
			<button class="btn btn-quiet" type="button" disabled={pages.prev === null} onclick={() => go(pages.prev)}>Back</button>
			<span class="xs muted">{pages.from} to {pages.to} of {total}</span>
			<button class="btn btn-quiet" type="button" disabled={pages.next === null} onclick={() => go(pages.next)}>More</button>
		</nav>
	{/if}
{/if}

<style>
	.switch-list {
		gap: var(--s-2);
	}
	.switch-list a[aria-current='page'] {
		color: var(--text);
		background: var(--surface-2, var(--surface-1));
	}
	.list-head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: var(--s-3);
	}
	.rel {
		padding: var(--s-3) 0;
	}
	.rel + .rel {
		border-top: 1px solid var(--line);
	}
	.rel-top {
		display: flex;
		justify-content: space-between;
		gap: var(--s-3);
		align-items: flex-start;
		margin-bottom: 4px;
	}
	.grow {
		min-width: 0;
		flex: 1;
	}
	.rel-name {
		font-size: var(--text-sm);
		overflow-wrap: anywhere;
	}
	.reasons {
		margin-top: var(--s-2);
		font-size: var(--text-xs);
		color: var(--text-muted);
	}
	.reasons li {
		padding: 2px 0 2px var(--s-3);
		border-left: 2px solid var(--line-strong);
	}
	.reasons li.fail {
		border-left-color: var(--fail);
	}
	.reasons li.hold,
	.reasons li.warn {
		border-left-color: var(--wait);
	}
	.pager {
		justify-content: space-between;
		margin-top: var(--s-3);
	}
	@media (max-width: 600px) {
		.rel-top {
			flex-direction: column;
		}
	}
</style>
