<script lang="ts">
	// Releases found for an episode, season or album, with pick and turn down actions.
	import Button from '#lib/ui/Button.svelte';
	import { chooseWarnings, facts, keyReasons, pager, verdict, verdictTone } from './releases.ts';
	import type { ReleaseScope as Scope } from './releases.ts';
	import { api, ApiError, message } from '../core/api.ts';
	import { bytes } from '../core/format.ts';
	import { live } from '../core/live.svelte.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import type { Choice, ReleasePage, ReleaseView } from '../core/types.ts';

	let {
		titleId,
		scope,
		jobs = [],
		music = false,
		bare = false,
		onclose,
		onchange
	}: { titleId: number; scope: Scope; jobs?: number[]; music?: boolean; bare?: boolean; onclose: () => void; onchange: () => void } = $props();

	// Rows per page.
	const limit = 25;

	let releases = $state<ReleaseView[]>([]);
	let total = $state(0);
	let offset = $state(0);
	let running = $state<number[]>([]);
	let loaded = $state(false);
	let chooseDialog: HTMLDialogElement | undefined = $state();
	let turnDialog: HTMLDialogElement | undefined = $state();
	let target = $state<ReleaseView | null>(null);
	let forceReasons = $state<string[]>([]);
	let chooseError = $state('');
	let choosing = $state(false);
	let turnReason = $state('');

	const noun = $derived(music ? 'tracks' : 'episodes');
	const pages = $derived(pager(total, limit, offset));

	// Picking from a season or album view fills several slots at once.
	const fillsMany = $derived(!!target && !scope.slotId && (target.slot_ids?.length ?? 0) > 1);
	const warnings = $derived(target ? chooseWarnings(target, scope.slotState, true, music) : []);
	const needsForce = $derived(!!target && (target.needs_force || forceReasons.length > 0));
	// Reading scope.query makes a new scope restart from page one.
	$effect(() => {
		void scope.query;
		offset = 0;
		running = [...jobs];
		void load();
	});

	$effect(() => {
		const offs = [
			live.on('releases', () => load()),
			live.on('job', (d) => {
				// Reload once a watched job leaves the running and queued states.
				const j = d as { id?: number; state?: string };
				if (j?.id && running.includes(j.id) && j.state !== 'running' && j.state !== 'queued') {
					running = running.filter((x) => x !== j.id);
					void load();
				}
			}),
			live.on('resync', () => load())
		];
		return () => offs.forEach((off) => off());
	});

	// Running ids are trimmed to the ones the server still lists.
	async function load() {
		try {
			const p = await api.get<ReleasePage>(`/titles/${titleId}/releases?${scope.query}&limit=${limit}&offset=${offset}`);
			releases = p.releases;
			total = p.total;
			const still = p.running_job_ids ?? [];
			running = running.filter((j) => still.includes(j));
		} catch (e) {
			toasts.push(`Couldn't load Releases`, 'fail', { body: message(e) });
		} finally {
			loaded = true;
		}
	}

	function go(off: number | null) {
		if (off === null) return;
		offset = off;
		void load();
	}


	function askChoose(r: ReleaseView) {
		target = r;
		forceReasons = [];
		chooseError = '';
		chooseDialog?.showModal();
	}

	async function choose() {
		if (!target) return;
		choosing = true;
		chooseError = '';
		try {
			const body: Record<string, unknown> = { force: needsForce };
			if (scope.slotId) body.slot_id = scope.slotId;
			const ch = await api.post<Choice>(`/releases/${target.id}/choose`, body);
			chooseDialog?.close();
			const n = ch.slot_ids?.length ?? 1;
			toasts.push('Sent to Downloader', 'ok', ch.warnings.length ? { body: ch.warnings.length === 1 ? ch.warnings[0] : `${ch.warnings.length} warnings` } : n > 1 ? { body: `For ${n} ${noun}` } : undefined);
			onchange();
			await load();
		} catch (e) {
			// The server asks for a forced pick when the release did not pass.
			if (e instanceof ApiError && e.status === 409 && e.body.needs_force) {
				forceReasons = (e.body.reasons as string[] | undefined) ?? ['it did not pass'];
			} else {
				chooseError = message(e);
			}
		} finally {
			choosing = false;
		}
	}

	function askTurnDown(r: ReleaseView) {
		target = r;
		turnReason = '';
		chooseError = '';
		turnDialog?.showModal();
	}

	async function turnDown(e: SubmitEvent) {
		e.preventDefault();
		if (!target) return;
		choosing = true;
		try {
			const body: Record<string, unknown> = {};
			if (turnReason.trim()) body.reason = turnReason.trim();
			if (scope.slotId) body.slot_id = scope.slotId;
			await api.post(`/releases/${target.id}/turn-down`, body);
			turnDialog?.close();
			toasts.push('Release turned down', 'ok', { body: `Never picked for ${scope.slotId || music ? scope.label : 'these episodes'}` });
			await load();
		} catch (err) {
			chooseError = message(err);
		} finally {
			choosing = false;
		}
	}
</script>

<section class="card rel-card" class:bare aria-labelledby="h-show-rel" data-testid="show-releases">
	<div class="card-head" class:sr-only={bare}>
		<h2 id="h-show-rel" class="h2">Choose a release: {scope.label}</h2>
		{#if !bare}<Button variant="plain" onclick={onclose}>Close</Button>{/if}
	</div>
	{#if total}<p class="xs muted">{total} found in the last search</p>{/if}
	<p class="small muted" aria-live="polite" data-testid="search-running">
		{running.length ? 'Searching your Sources. The list fills in once they answer.' : ''}
	</p>
	{#if loaded && releases.length === 0 && !running.length}
		<p class="small muted">Nothing found yet.</p>
	{:else if releases.length}
		<ul class="rels">
			{#each releases as r (r.id)}
				{@const key = keyReasons(r.reasons)}
				<li class="rel" data-testid="release">
					<div class="rel-top">
						<span class="rel-name mono">{r.name}</span>
						<span class="chip {verdictTone(r)}" data-testid="verdict">{verdict(r)}</span>
					</div>
					<div class="xs muted">{facts(r, bytes)}{r.episode ? ` · ${r.episode}` : ''}{music && r.slot_ids?.length ? ` · ${r.slot_ids.length} tracks` : ''}{r.version && r.version !== 'Main' ? ` · ${r.version}` : ''}</div>
					{#if key.length}
						<ul class="reasons">
							{#each key as x, i (i)}<li class={x.effect}>{x.text}</li>{/each}
						</ul>
					{/if}
					{#if r.reasons.length}
						<details>
							<summary class="xs muted">All {r.reasons.length} reasons</summary>
							<ul class="reasons">
								{#each r.reasons as x, i (i)}<li class={x.effect}>{x.text}</li>{/each}
							</ul>
						</details>
					{/if}
					{#if r.state !== 'chosen'}
						<div class="row rel-actions">
							<Button onclick={() => askChoose(r)} aria-label="Choose {r.name}">Choose</Button>
							{#if r.state !== 'turned_down'}
								<Button variant="plain" onclick={() => askTurnDown(r)} aria-label="Turn down {r.name}">Turn down</Button>
							{/if}
						</div>
					{/if}
				</li>
			{/each}
		</ul>
		{#if pages.prev !== null || pages.next !== null}
			<nav class="row pager" aria-label="Release pages">
				<Button variant="plain" disabled={pages.prev === null} onclick={() => go(pages.prev)}>Back</Button>
				<span class="xs muted">{pages.from} to {pages.to} of {total}</span>
				<Button variant="plain" disabled={pages.next === null} onclick={() => go(pages.next)}>More</Button>
			</nav>
		{/if}
	{/if}
</section>

<dialog bind:this={chooseDialog} aria-labelledby="show-choose-title" class="modal">
	{#if target}
		<h2 id="show-choose-title" class="h2">{needsForce ? 'Choose it anyway?' : 'Choose this Release?'}</h2>
		<p class="mono small path" style="margin-top: var(--s-3)">{target.name}</p>
		<p class="small muted">{facts(target, bytes)}</p>
		<p class="small">{music ? `For ${scope.label}.` : fillsMany ? `For ${target.episode}.` : `For ${scope.slotId ? scope.label : (target.episode ?? scope.label)}.`}</p>
		{#if warnings.length || forceReasons.length}
			<ul class="warns" data-testid="choose-warnings">
				{#each warnings as w (w)}<li>{w}</li>{/each}
				{#each forceReasons as w (w)}<li>It did not pass: {w}.</li>{/each}
			</ul>
		{/if}
		{#if chooseError}<p class="form-error" role="alert">{chooseError}</p>{/if}
		<div class="row modal-actions">
			<Button variant="plain" onclick={() => chooseDialog?.close()}>Cancel</Button>
			<Button variant="primary" onclick={choose} disabled={choosing} busy={choosing}>{needsForce ? 'Choose anyway' : 'Choose'}</Button>
		</div>
	{/if}
</dialog>

<dialog bind:this={turnDialog} aria-labelledby="show-turn-title" class="modal">
	{#if target}
		<form onsubmit={turnDown}>
			<h2 id="show-turn-title" class="h2">Turn down this Release?</h2>
			<p class="mono small path" style="margin-top: var(--s-3)">{target.name}</p>
			<p class="small muted">
				{scope.slotId
					? `HolyArrs never picks it again for ${scope.label}; for other episodes it is still judged.`
					: `HolyArrs never picks it again for these ${noun}.`} You can take it off the Turned down list later.
			</p>
			<div class="field" style="margin-top: var(--s-4)">
				<label for="show-turn-reason">Why (optional)</label>
				<input id="show-turn-reason" class="input" bind:value={turnReason} maxlength="500" />
			</div>
			{#if chooseError}<p class="form-error" role="alert">{chooseError}</p>{/if}
			<div class="row modal-actions">
				<Button variant="plain" onclick={() => turnDialog?.close()}>Cancel</Button>
				<Button variant="primary" type="submit" disabled={choosing} busy={choosing}>Turn down</Button>
			</div>
		</form>
	{/if}
</dialog>

<style>
	.rel-card.bare { padding: 0; background: none; box-shadow: none; border: 0; }
	.card-head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: var(--s-3);
		flex-wrap: wrap;
	}
	.rels {
		margin-top: var(--s-3);
	}
	.rel {
		padding: var(--s-3);
		border: 1px solid var(--line);
		border-radius: 6px;
		background: var(--surface-1);
	}
	.rel + .rel {
		margin-top: var(--s-2);
	}
	.rel-top {
		display: flex;
		justify-content: space-between;
		gap: var(--s-3);
		align-items: flex-start;
		margin-bottom: 4px;
	}
	.rel-name {
		font-size: var(--text-sm);
		overflow-wrap: anywhere;
		min-width: 0;
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
	.rel-actions {
		margin-top: var(--s-2);
		gap: var(--s-2);
	}
	.rel-actions :global(.ui-btn) { min-height: 36px; }
	.pager {
		justify-content: space-between;
		margin-top: var(--s-3);
	}
	.path {
		overflow-wrap: anywhere;
	}
	.warns {
		margin-top: var(--s-3);
		padding-left: var(--s-4);
		list-style: disc;
		font-size: var(--text-sm);
		color: var(--wait);
	}
	.modal {
		width: min(480px, calc(100vw - 32px));
		max-height: calc(100dvh - 32px);
		overflow: auto;
		padding: var(--s-6);
		border: 0;
		border-radius: var(--r-modal);
		background: var(--surface-1);
		color: var(--text);
		box-shadow: var(--shadow-modal);
	}
	.modal::backdrop {
		background: var(--scrim);
	}
	.modal-actions {
		justify-content: flex-end;
		margin-top: var(--s-6);
	}
	@media (max-width: 600px) {
		.rel-top {
			flex-direction: column;
		}
	}
</style>
