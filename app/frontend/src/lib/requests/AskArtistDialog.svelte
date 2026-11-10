<script lang="ts">
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	// A member asks for an artist: the whole artist, or the albums they pick.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { albumKind, albumWords, albumYear } from '../music/index.ts';
	import type { AskableAlbum, LookupArtist } from '../music/index.ts';
	import { api, message, ApiError } from '../core/api.ts';
	import type { Ask } from '../core/types.ts';

	let {
		onasked,
		onrefused
	}: {
		onasked: (artist: LookupArtist, r: { ask: Ask; joined: boolean; already: boolean }) => void;
		onrefused?: (artist: LookupArtist, body: Record<string, unknown>) => void;
	} = $props();

	let sheetOpen = $state(false);
	let picked = $state<LookupArtist | null>(null);
	// Follow puts it on my calendar (ticked by default).
	let follow = $state(true);
	let albums = $state<AskableAlbum[] | null>(null);
	let whole = $state(true);
	let chosen = $state<string[]>([]);
	let busy = $state(false);
	let error = $state('');

	/** Shows the dialog for one artist and reads its albums; `album` (a release group) starts picked when given. */
	export async function open(a: LookupArtist, album?: string) {
		follow = true;
		picked = a;
		albums = null;
		whole = true;
		chosen = [];
		error = '';
		sheetOpen = true;
		try {
			const r = await api.get<{ artists: LookupArtist[] }>(`/artists/lookup?term=${encodeURIComponent(`mbid:${a.mbid}`)}&albums=true`);
			albums = r.artists[0]?.ask_albums ?? [];
			if (album && albums.some((x) => x.mbid === album)) {
				whole = false;
				chosen = [album];
			}
			if (r.artists[0]) picked = { ...a, ...r.artists[0] };
		} catch (err) {
			albums = [];
			error = message(err);
		}
	}

	function pick(mbid: string, on: boolean) {
		chosen = on ? [...chosen, mbid] : chosen.filter((x) => x !== mbid);
	}

	const chosenNames = $derived((albums ?? []).filter((a) => chosen.includes(a.mbid)).map((a) => ({ mbid: a.mbid, name: a.name })));

	async function ask(e: SubmitEvent) {
		e.preventDefault();
		if (!picked) return;
		busy = true;
		error = '';
		try {
			const r = await api.post<{ ask: Ask; joined: boolean; already: boolean }>('/asks', { kind: 'artist', id: `mbid:${picked.mbid}`, albums: whole ? [] : chosen, follow });
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

<Sheet bind:open={sheetOpen} title={picked ? t('requests.ask.title', { name: picked.name }) : t('requests.ask.titleBare')}>
	{#if picked}
		<form id="ask-artist-title" class="sheet-form" onsubmit={ask}>
			{#if picked.ask}
				<p class="small muted" style="margin-top: var(--s-2)">{t('requests.askArtist.already', { what: albumWords(picked.ask.albums), state: serverWords(picked.ask.state_label) })}</p>
			{/if}
			<fieldset class="pick">
				<legend class="small">{t('requests.ask.what')}</legend>
				<label class="checkline">
					<input type="radio" name="what" checked={whole} onchange={() => (whole = true)} />
					<span><b>{t('requests.askArtist.every')}</b><br /><span class="hint">{t('requests.askArtist.everyHint')}</span></span>
				</label>
				<label class="checkline">
					<input type="radio" name="what" checked={!whole} onchange={() => (whole = false)} disabled={!albums?.length} />
					<span><b>{t('requests.askArtist.some')}</b><br /><span class="hint">{t('requests.askArtist.someHint')}</span></span>
				</label>
			</fieldset>
			{#if !whole}
				<fieldset class="albums">
					<legend class="sr-only">{t('requests.askArtist.albums')}</legend>
					{#each albums ?? [] as al (al.mbid)}
						<label class="album" class:off={al.kept || al.left_out} data-testid="ask-album">
							<input type="checkbox" checked={chosen.includes(al.mbid)} disabled={al.kept || al.left_out} onchange={(e) => pick(al.mbid, e.currentTarget.checked)} />
							<span class="grow">
								<b>{al.name}</b>
								<span class="xs muted block">{[albumYear(al.release_date), albumKind(al)].filter(Boolean).join(' · ')}{al.kept ? ` · ${t('requests.askArtist.inLibrary')}` : al.left_out ? ` · ${t('requests.askArtist.notIncluded')}` : ''}</span>
							</span>
						</label>
					{/each}
				</fieldset>
			{:else if albums === null}
				<div class="sk" style="height: 60px; margin-top: var(--s-3)"></div>
			{/if}
			<label class="checkline" data-testid="follow-box">
				<input type="checkbox" bind:checked={follow} />
				<span><b>{t('requests.add.follow')}</b><br /><span class="hint">{t('requests.add.followHint')}</span></span>
			</label>
			{#if error}<p class="form-error" role="alert">{error}</p>{/if}
		</form>
	{/if}
	{#snippet footer()}
				<Button variant="plain" onclick={() => (sheetOpen = false)}>{t('requests.add.cancel')}</Button>
				<Button variant="primary" type="submit" form="ask-artist-title" disabled={(!whole && chosen.length === 0)} busy={busy}>
					{whole ? t('requests.askArtist.submitAll') : chosen.length ? t('requests.ask.title', { name: albumWords(chosenNames) }) : t('requests.askArtist.pick')}
				</Button>
	{/snippet}
</Sheet>

<style>
	.sheet-form { display: flex; flex-direction: column; gap: 4px; }
	.form-error { margin: 8px 0 0; color: var(--fail); font: var(--t-callout); }
	.pick,
	.albums {
		border: 0;
		padding: 0;
		margin: var(--s-4) 0 0;
	}
	.albums {
		display: flex;
		flex-direction: column;
		gap: var(--s-2);
	}
	.album {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		min-height: 48px;
		padding: var(--s-2) var(--s-3);
		border: 1px solid var(--line-strong);
		border-radius: var(--r-btn);
		cursor: pointer;
	}
	.album:has(input:checked) {
		border-color: var(--accent);
		background: var(--accent-soft);
	}
	/* Muted grey on the tint, or faded further, falls below the contrast floor. */
	.album:has(input:checked) .muted {
		color: var(--text);
	}
	.album.off {
		cursor: default;
		color: var(--text-muted);
	}
	.block {
		display: block;
	}
	.grow {
		min-width: 0;
		overflow-wrap: anywhere;
	}
</style>
