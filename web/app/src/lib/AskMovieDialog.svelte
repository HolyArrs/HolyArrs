<script lang="ts" module>
	import type { AskBrief } from './types.ts';

	export interface AskMovie {
		tmdb: string;
		name: string;
		year?: number | string;
		ask?: AskBrief;
	}
</script>

<script lang="ts">
	// A member asks for a movie: one confirm step, so a card or result button
	// never asks in a single click.
	import Sheet from './ui/Sheet.svelte';
	import Button from './ui/Button.svelte';
	import { api, message, ApiError } from './api.ts';
	import type { Ask } from './types.ts';

	let {
		onasked,
		onrefused
	}: {
		onasked: (movie: AskMovie, r: { ask: Ask; joined: boolean; already: boolean }) => void;
		onrefused?: (movie: AskMovie, body: Record<string, unknown>) => void;
	} = $props();

	let sheetOpen = $state(false);
	let picked = $state<AskMovie | null>(null);
	// Follow: put it on my calendar (ticked by default).
	let follow = $state(true);
	// 4K: ask for a 4K copy instead; it is its own ask, approved on its own.
	let fourK = $state(false);
	let busy = $state(false);
	let error = $state('');

	/** open shows the dialog for one movie. */
	export function open(m: AskMovie) {
		follow = true;
		fourK = false;
		picked = m;
		error = '';
		sheetOpen = true;
	}

	async function ask(e: SubmitEvent) {
		e.preventDefault();
		if (!picked || busy) return;
		busy = true;
		error = '';
		try {
			const r = await api.post<{ ask: Ask; joined: boolean; already: boolean }>('/asks', { id: `tmdb:${picked.tmdb}`, follow, ...(fourK ? { four_k: true } : {}) });
			sheetOpen = false;
			onasked(picked, r);
		} catch (err) {
			error = message(err);
			if (err instanceof ApiError && err.status === 409) onrefused?.(picked, err.body);
		} finally {
			busy = false;
		}
	}
</script>

<Sheet bind:open={sheetOpen} size="sm" title={picked ? `Ask for ${picked.name}${picked.year ? ` (${picked.year})` : ''}` : 'Ask for a movie'}>
	{#if picked}
		<form id="ask-movie-title" class="sheet-form" onsubmit={ask}>
			<p class="small muted">
				{fourK
					? 'A 4K copy is its own ask: it goes on the Requests page and is approved apart from the regular copy.'
					: picked.ask
					? `Someone asked for it already (${picked.ask.state_label}). Asking joins their ask, and you hear when it is ready.`
					: 'It goes on the Requests page. You hear when it is approved and when it is ready to watch.'}
			</p>
			<label class="checkline" data-testid="follow-box">
				<input type="checkbox" bind:checked={follow} />
				<span><b>Follow</b><br /><span class="hint">Show it on my calendar.</span></span>
			</label>
			<label class="checkline" data-testid="four-k-box">
				<input type="checkbox" bind:checked={fourK} />
				<span><b>4K copy</b><br /><span class="hint">Ask for the 4K version.</span></span>
			</label>
			{#if error}<p class="form-error" role="alert">{error}</p>{/if}
		</form>
	{/if}
	{#snippet footer()}
		<Button variant="plain" onclick={() => (sheetOpen = false)}>Cancel</Button>
		<Button variant="primary" type="submit" form="ask-movie-title" {busy}>{picked ? `Ask for ${picked.name}${fourK ? ' in 4K' : ''}` : 'Ask'}</Button>
	{/snippet}
</Sheet>

<style>
	.sheet-form { display: flex; flex-direction: column; gap: 4px; }
	.form-error { margin: 8px 0 0; color: var(--fail); font: var(--t-callout); }
</style>
