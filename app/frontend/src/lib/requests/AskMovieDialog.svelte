<script lang="ts" module>
	import type { AskBrief } from '../core/types.ts';

	export interface AskMovie {
		tmdb: string;
		name: string;
		year?: number | string;
		ask?: AskBrief;
		/** The open 4K ask, if someone asked for a 4K copy. */
		ask4k?: AskBrief;
		/** Opened from the 4K button: start on the 4K line. */
		fourKOnly?: boolean;
		/** The movie is in the Library (its main copy is there). */
		inLibrary?: boolean;
		/** The movie has its 4K copy. */
		has4k?: boolean;
	}
</script>

<script lang="ts">
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	// A member asks for a movie in one confirm step, so a card or result button never asks in a single click.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { api, message, ApiError } from '../core/api.ts';
	import type { Ask } from '../core/types.ts';
	import { askQualities } from './four-k-ready.ts';
	import QualityPills from './QualityPills.svelte';
	import { FOUR_K, STANDARD, memberRow } from './quality-row.ts';
	import type { QualityChoice } from './types.ts';

	let {
		onasked,
		onrefused
	}: {
		onasked: (movie: AskMovie, r: { ask: Ask; joined: boolean; already: boolean }) => void;
		onrefused?: (movie: AskMovie, body: Record<string, unknown>) => void;
	} = $props();

	let sheetOpen = $state(false);
	let picked = $state<AskMovie | null>(null);
	// Follow puts it on my calendar (ticked by default).
	let follow = $state(true);
	// The Quality row: the lines of what people can ask for that I may use (one line or none hides it).
	let choices = $state<QualityChoice[]>([]);
	let quality = $state('');
	// Each opening reads the lines again; an answer for an earlier opening is dropped.
	let opened = 0;
	let busy = $state(false);
	let error = $state('');

	const briefs = $derived<Record<string, AskBrief | undefined>>({ [STANDARD]: picked?.ask, [FOUR_K]: picked?.ask4k });
	const row = $derived(
		memberRow(choices, {
			inLibrary: picked?.inLibrary,
			has4k: picked?.has4k,
			mine: Object.keys(briefs).filter((k) => briefs[k]?.mine),
			open: Object.keys(briefs).filter((k) => briefs[k] && !briefs[k]?.mine),
			prefer: picked?.fourKOnly ? FOUR_K : undefined
		})
	);
	// Someone else asked for the picked line already: asking joins their ask.
	const theirs = $derived(quality ? briefs[quality] : undefined);

	/** Shows the dialog for one movie. */
	export function open(m: AskMovie) {
		follow = true;
		choices = [];
		// Until the lines arrive, the 4K button still asks for 4K.
		quality = m.fourKOnly ? FOUR_K : '';
		picked = m;
		error = '';
		sheetOpen = true;
		const asked = ++opened;
		void askQualities().then((q) => {
			if (asked !== opened) return;
			choices = q.movie;
			quality = row.selected || quality;
		});
	}

	async function ask(e: SubmitEvent) {
		e.preventDefault();
		if (!picked || busy) return;
		busy = true;
		error = '';
		try {
			const r = await api.post<{ ask: Ask; joined: boolean; already: boolean }>('/asks', { id: `tmdb:${picked.tmdb}`, follow, ...(quality ? { quality } : {}) });
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

<Sheet bind:open={sheetOpen} size="sm" title={picked ? t('requests.ask.title', { name: `${picked.name}${picked.year ? ` (${picked.year})` : ''}` }) : t('requests.askMovie.titleBare')}>
	{#if picked}
		<form id="ask-movie-title" class="sheet-form" onsubmit={ask}>
			<p class="small muted">{theirs ? t('requests.askMovie.already', { state: serverWords(theirs.state_label) }) : t('requests.askMovie.hear')}</p>
			{#if row.shown}<QualityPills pills={row.pills} bind:selected={quality} stacked />{/if}
			<label class="checkline" data-testid="follow-box">
				<input type="checkbox" bind:checked={follow} />
				<span><b>{t('requests.add.follow')}</b><br /><span class="hint">{t('requests.add.followHint')}</span></span>
			</label>
			{#if error}<p class="form-error" role="alert">{error}</p>{/if}
		</form>
	{/if}
	{#snippet footer()}
		<Button variant="plain" onclick={() => (sheetOpen = false)}>{t('requests.add.cancel')}</Button>
		<Button variant="primary" type="submit" form="ask-movie-title" {busy} disabled={row.shown && !quality}>{picked ? t('requests.ask.title', { name: picked.name }) : t('requests.ask.bare')}</Button>
	{/snippet}
</Sheet>

<style>
	.sheet-form { display: flex; flex-direction: column; gap: 4px; }
	.form-error { margin: 8px 0 0; color: var(--fail); font: var(--t-callout); }
</style>
