<script lang="ts">
	// A member asks for a show: the whole show, or the seasons they pick.
	import { api, message, ApiError } from './api.ts';
	import { seasonName, seasonWords } from './shows.ts';
	import type { Ask, LookupShow } from './types.ts';

	let {
		onasked,
		onrefused
	}: {
		onasked: (show: LookupShow, r: { ask: Ask; joined: boolean; already: boolean }) => void;
		onrefused?: (show: LookupShow, body: Record<string, unknown>) => void;
	} = $props();

	let dialog: HTMLDialogElement | undefined = $state();
	let picked = $state<LookupShow | null>(null);
	let whole = $state(true);
	let chosen = $state<number[]>([]);
	let busy = $state(false);
	let error = $state('');

	const seasons = $derived((picked?.seasons ?? []).map((s) => s.number).sort((a, b) => a - b));

	/** open shows the dialog for one looked-up show. */
	export function open(s: LookupShow) {
		picked = s;
		whole = true;
		chosen = [];
		error = '';
		dialog?.showModal();
	}

	function idOf(s: LookupShow): string {
		for (const k of [s.source, 'tmdb', 'tvdb', 'tvmaze', 'imdb']) if (s.ids[k]) return `${k}:${s.ids[k]}`;
		return '';
	}

	function toggle(n: number, on: boolean) {
		chosen = on ? [...chosen, n].sort((a, b) => a - b) : chosen.filter((x) => x !== n);
	}

	async function ask(e: SubmitEvent) {
		e.preventDefault();
		if (!picked) return;
		busy = true;
		error = '';
		try {
			const r = await api.post<{ ask: Ask; joined: boolean; already: boolean }>('/asks', { kind: 'show', id: idOf(picked), seasons: whole ? [] : chosen });
			dialog?.close();
			onasked(picked, r);
		} catch (err) {
			error = message(err);
			if (err instanceof ApiError && err.status === 409) onrefused?.(picked, err.body);
		} finally {
			busy = false;
		}
	}
</script>

<dialog bind:this={dialog} aria-labelledby="ask-show-title" class="modal">
	{#if picked}
		<form onsubmit={ask}>
			<h2 id="ask-show-title" class="h2">Ask for {picked.name}</h2>
			{#if picked.ask}
				<p class="small muted" style="margin-top: var(--s-2)">Someone asked for {seasonWords(picked.ask.seasons)} already ({picked.ask.state_label}). Asking for the same joins their ask.</p>
			{/if}
			<fieldset class="pick">
				<legend class="small">What would you like?</legend>
				<label class="checkline">
					<input type="radio" name="what" checked={whole} onchange={() => (whole = true)} />
					<span><b>The whole show</b><br /><span class="hint">Every season, and seasons that come later.</span></span>
				</label>
				<label class="checkline">
					<input type="radio" name="what" checked={!whole} onchange={() => (whole = false)} disabled={seasons.length === 0} />
					<span><b>Only some seasons</b><br /><span class="hint">Pick them below.</span></span>
				</label>
			</fieldset>
			{#if !whole}
				<fieldset class="seasons">
					<legend class="sr-only">Seasons</legend>
					{#each seasons as n (n)}
						<label class="season">
							<input type="checkbox" checked={chosen.includes(n)} onchange={(e) => toggle(n, e.currentTarget.checked)} />
							<span>{seasonName(n)}</span>
						</label>
					{/each}
				</fieldset>
			{/if}
			{#if error}<p class="form-error" role="alert">{error}</p>{/if}
			<div class="row modal-actions">
				<button class="btn btn-quiet" type="button" onclick={() => dialog?.close()}>Cancel</button>
				<button class="btn btn-red" type="submit" disabled={busy || (!whole && chosen.length === 0)} aria-busy={busy}>
					{whole ? 'Ask for the whole show' : chosen.length ? `Ask for ${seasonWords(chosen)}` : 'Pick a season'}
				</button>
			</div>
		</form>
	{/if}
</dialog>

<style>
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
		flex-wrap: wrap;
	}
	.pick,
	.seasons {
		border: 0;
		padding: 0;
		margin: var(--s-4) 0 0;
	}
	.seasons {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(130px, 1fr));
		gap: var(--s-2);
	}
	.season {
		display: flex;
		align-items: center;
		gap: var(--s-2);
		min-height: 44px;
		padding: 0 var(--s-3);
		border: 1px solid var(--line-strong);
		border-radius: var(--r-btn);
		cursor: pointer;
	}
	.season:has(input:checked) {
		border-color: var(--ha-red);
		background: var(--ha-red-soft);
	}
</style>
