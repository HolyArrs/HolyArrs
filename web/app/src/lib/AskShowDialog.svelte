<script lang="ts">
	// A member asks for a show: all seasons, the seasons they pick, or the
	// future seasons only (what airs from now on).
	import Sheet from './ui/Sheet.svelte';
	import Button from './ui/Button.svelte';
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

	let sheetOpen = $state(false);
	let picked = $state<LookupShow | null>(null);
	let mode = $state<'all' | 'choose' | 'future'>('all');
	let chosen = $state<number[]>([]);
	let busy = $state(false);
	let error = $state('');

	const seasons = $derived((picked?.seasons ?? []).map((s) => s.number).sort((a, b) => a - b));

	/** open shows the dialog for one looked-up show. */
	export function open(s: LookupShow) {
		picked = s;
		mode = 'all';
		chosen = [];
		error = '';
		sheetOpen = true;
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
			const r = await api.post<{ ask: Ask; joined: boolean; already: boolean }>('/asks', mode === 'future' ? { kind: 'show', id: idOf(picked), future_only: true } : { kind: 'show', id: idOf(picked), seasons: mode === 'all' ? [] : chosen });
			(sheetOpen = false);
			onasked(picked, r);
		} catch (err) {
			error = message(err);
			if (err instanceof ApiError && err.status === 409) onrefused?.(picked, err.body);
		} finally {
			busy = false;
		}
	}
</script>

<Sheet bind:open={sheetOpen} title={picked ? `Ask for ${picked.name}` : 'Ask for'}>
	{#if picked}
		<form id="ask-show-title" class="sheet-form" onsubmit={ask}>
			{#if picked.ask}
				<p class="small muted" style="margin-top: var(--s-2)">Someone asked for {seasonWords(picked.ask.seasons)} already ({picked.ask.state_label}). Asking for the same joins their ask.</p>
			{/if}
			<fieldset class="pick">
				<legend class="small">What would you like?</legend>
				<label class="checkline">
					<input type="radio" name="what" checked={mode === 'all'} onchange={() => (mode = 'all')} />
					<span><b>All seasons</b><br /><span class="hint">Every season, and seasons that come later.</span></span>
				</label>
				<label class="checkline">
					<input type="radio" name="what" checked={mode === 'choose'} onchange={() => (mode = 'choose')} disabled={seasons.length === 0} />
					<span><b>Choose seasons</b><br /><span class="hint">Pick them below.</span></span>
				</label>
				<label class="checkline">
					<input type="radio" name="what" checked={mode === 'future'} onchange={() => (mode = 'future')} />
					<span><b>Future seasons only</b><br /><span class="hint">Nothing that is out already; new episodes and seasons from today on.</span></span>
				</label>
			</fieldset>
			{#if mode === 'choose'}
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
		</form>
	{/if}
	{#snippet footer()}
				<Button variant="plain" onclick={() => (sheetOpen = false)}>Cancel</Button>
				<Button variant="primary" type="submit" form="ask-show-title" disabled={mode === 'choose' && chosen.length === 0} busy={busy}>
					{mode === 'all' ? 'Ask for all seasons' : mode === 'future' ? 'Ask for future seasons' : chosen.length ? `Ask for ${seasonWords(chosen)}` : 'Pick a season'}
				</Button>
	{/snippet}
</Sheet>

<style>
	.sheet-form { display: flex; flex-direction: column; gap: 4px; }
	.form-error { margin: 8px 0 0; color: var(--fail); font: var(--t-callout); }
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
		border-color: var(--accent);
		background: var(--accent-soft);
	}
</style>
