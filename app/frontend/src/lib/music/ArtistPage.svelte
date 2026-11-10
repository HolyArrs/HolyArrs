<script lang="ts">
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import '#lib/ui/title-page.css';
	import LoadError from '#lib/ui/LoadError.svelte';
	import TitleSection from '#lib/ui/TitleSection.svelte';
	import { audienceStatus } from '#lib/ui/title-status.ts';
	import MusicArtistPage, { type ArtistLibrary } from './ArtistOverview.svelte';
	import { albumWords, artistProgress } from './labels.ts';
	import type { ArtistSheets, LookupArtist } from './types.ts';
	import { peopleNames } from '../core/plain.ts';
	import { api, message } from '../core/api.ts';
	import { live } from '../core/live.svelte.ts';
	import { date } from '../core/format.ts';
	import { session } from '../core/session.svelte.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import { decided } from '../core/plain.ts';
	import type { Ask, TitleView } from '../core/types.ts';

	// One artist in the Library: the artist page (ArtistOverview) with its Overview and Library tabs. The Library tab holds the albums, their actions and the artist's settings. Below them: who asked.

	let {
		title,
		onchange,
		error = '',
		onretry,
		sheets
	}: { title: TitleView; onchange: (t: TitleView) => void; error?: string; onretry?: () => Promise<void> | void; sheets: ArtistSheets } = $props();

	// A failed refresh keeps what is shown and says so above it.
	let loadErr = $state('');
	let retrying = $state(false);
	async function retry() {
		retrying = true;
		try {
			await onretry?.();
			await reload();
		} finally {
			retrying = false;
		}
	}

	const staff = $derived(session.can('manage_library'));
	const artist = $derived(title.artist);
	// Bumped on every read of the artist, so the Library tab reads again too.
	let stamp = $state(0);
	let asker: ReturnType<ArtistSheets['ask']> | undefined = $state();
	const visibleDisambiguation = $derived(artist?.disambiguation?.trim() ?? '');
	const heroKind = $derived(artist?.missing ? 'missing' : 'in_library');
	// One status vocabulary (title-status.ts); staff also see how many are missing.
	const heroWords = $derived(audienceStatus(heroKind, staff ? 'staff' : 'member', { tracked: true, titleKind: 'artist' }));
	// The artist page shows the hero, the discography, similar artists and About; these are the Library's own parts of it.
	const lib = $derived<ArtistLibrary>({
		titleId: title.id,
		staff,
		progress: staff ? artistProgress(artist) : '',
		status: staff && artist?.missing ? t('music.missing2', { n: artist.missing }) : heroWords.label,
		note: visibleDisambiguation,
		nextRelease: artist?.next_release ? date(artist.next_release) : undefined,
		stamp,
		onAsk: session.can('ask') ? askMore : undefined
	});

	async function reload() {
		try {
			onchange(await api.get<TitleView>(`/titles/${title.id}`));
			loadErr = '';
			stamp++;
		} catch (e) {
			loadErr = message(e);
		}
	}

	// Albums, Pressings and tracks arrive with the refresh after adding. Read again when this artist's Events come in, and every few seconds while included albums have no tracks yet.
	let again: ReturnType<typeof setTimeout> | undefined;
	$effect(() => {
		const soon = (e?: unknown) => {
			const ev = e as { TitleID?: number } | undefined;
			if (ev && typeof ev === 'object' && 'TitleID' in ev && ev.TitleID !== title.id) return;
			clearTimeout(again);
			again = setTimeout(() => void reload(), 400);
		};
		const offs = [live.on('event', soon), live.on('slot', () => soon())];
		return () => {
			clearTimeout(again);
			offs.forEach((off) => off());
		};
	});
	let polls = 0;
	$effect(() => {
		const waiting = !!artist && artist.included > 0 && artist.tracks === 0 && polls < 30;
		if (!waiting) return;
		const t = setTimeout(() => {
			polls++;
			void reload().catch(() => {});
		}, 3000);
		return () => clearTimeout(t);
	});

	function askMore() {
		if (!artist) return;
		const a: LookupArtist = { mbid: artist.mbid, name: title.name, status: artist.status, title_id: title.id, disambiguation: artist.disambiguation };
		void asker?.open(a);
	}

	function asked(_a: LookupArtist, r: { ask: Ask; joined: boolean }) {
		const what = `${title.name} (${albumWords(r.ask.albums)})`;
		if (r.ask.state === 'approved') toasts.push(t('music.approved', { what }), 'ok', { body: t('music.keptUpToDate') });
		else if (r.joined) toasts.push(t('music.alreadyRequested', { what }), 'ok', { body: t('music.youReOnThe') });
		else toasts.push(t('music.requested2', { what }), 'ok', { body: t('music.waitingForApproval') });
		void reload();
	}
</script>

<MusicArtistPage mbid={artist?.mbid ?? ''} library={lib} {sheets}>
{#snippet below()}
<div class="tp lib-below">
{#if loadErr || error}<LoadError hasData message={loadErr || error} onretry={() => void retry()} busy={retrying} />{/if}

{#if title.asks?.length}
	<TitleSection id="h-artist-asks" title={t('music.requests')} testid="title-asks">
		{#each title.asks as a (a.id)}
			<dl class="facts">
				<dt>{t('music.askedBy')}</dt>
				<dd>{peopleNames(a.people.map((p) => p.name))} · {date(a.asked_at)}</dd>
				<dt>{t('music.for')}</dt>
				<dd>{a.album_words ?? albumWords(a.albums)}</dd>
				{#if decided(a)}<dt>{t('music.decided')}</dt>
					<dd>{decided(a)}{a.decided_at ? ` · ${date(a.decided_at)}` : ''}</dd>{/if}
				<dt>{t('music.now2')}</dt>
				<dd>{serverWords(a.state_label)}</dd>
				{#if a.note}<dt>{t('music.note')}</dt>
					<dd>{a.note}</dd>{/if}
			</dl>
		{/each}
	</TitleSection>
{/if}

</div>
{/snippet}
</MusicArtistPage>

{#if !staff && session.can('ask')}
	<sheets.ask bind:this={asker} onasked={asked} />
{/if}

<style>
	.lib-below { display: flex; flex-direction: column; gap: 40px; }
</style>
