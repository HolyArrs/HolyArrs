<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { every, listKinds, listLink, listSeasons, titleKinds } from '#lib/requests/index.ts';
	import type { NeverAddEntry, OutsideList } from '#lib/requests/index.ts';
	import { onMount } from 'svelte';
	import { session } from '#lib/core/session.svelte.ts';
	import { api, ApiError } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { live } from '#lib/core/live.svelte.ts';
	import { dateTime } from '#lib/core/format.ts';
	import type { LibraryFolder, PickRulesView } from '#lib/core/types.ts';

	let lists = $state<OutsideList[]>([]);
	let folders = $state<LibraryFolder[]>([]);
	let rules = $state<PickRulesView[]>([]);
	let never = $state<NeverAddEntry[]>([]);
	let loaded = $state(false);
	let hasData = $state(false);
	let error = $state('');
	let busyId = $state<number | null>(null);

	// The list form, for a new list (editId null) or a change.
	let formOpen = $state(false);
	let editId = $state<number | null>(null);
	let name = $state('');
	let kind = $state('tmdb_list');
	let titleKind = $state<'movie' | 'show'>('movie');
	let ref = $state('');
	let folderId = $state(0);
	let rulesId = $state(0);
	let keep = $state(true);
	let searchOnAdd = $state(false);
	let seasons = $state('all');
	let newSeasons = $state('all');
	let enabled = $state(true);
	let hours = $state(12);
	let formError = $state('');
	let saving = $state(false);

	let traktMasked = $state('');
	let traktKey = $state('');
	let traktError = $state('');
	let traktSaving = $state(false);
	// Owner only: a Trakt VIP access token adds the Trakt tab (score, watchers, comments) to title pages.
	let traktVipMasked = $state('');
	let traktVip = $state('');

	let naKind = $state<'movie' | 'show'>('movie');
	let naId = $state('');
	let naName = $state('');
	let naReason = $state('');
	let naError = $state('');

	// The folders and Pick rules that fit what the list adds.
	const kindFolders = $derived(folders.filter((f) => f.kind === titleKind));
	const kindRules = $derived(rules.filter((r) => !r.kinds.length || r.kinds.includes(titleKind)));
	const hasTrakt = $derived(lists.some((l) => l.kind === 'trakt_list'));

	async function loadLists() {
		lists = (await api.get<{ outside_lists: OutsideList[] }>('/outside-lists')).outside_lists;
	}

	async function load() {
		try {
			await loadLists();
			const [f, r, n] = await Promise.all([
				api.get<{ library_folders: LibraryFolder[] }>('/library-folders'),
				api.get<{ pick_rules: PickRulesView[] }>('/pick-rules'),
				api.get<{ never_add: NeverAddEntry[] }>('/never-add')
			]);
			folders = f.library_folders;
			rules = r.pick_rules;
			never = n.never_add;
			// The key read needs a recent password check; without one the lists still show.
			traktMasked = (await api.get<{ key: string }>('/settings/trakt-key').catch(() => ({ key: '' }))).key;
			if (session.s.role === 'owner') traktVipMasked = (await api.get<{ token: string }>('/settings/trakt-token').catch(() => ({ token: '' }))).token;
			hasData = true;
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}

	onMount(() => {
		void load();
		const offs = [live.on('outside_list', () => loadLists().catch(() => {})), live.on('resync', () => load())];
		return () => offs.forEach((off) => off());
	});

	function openForm(l?: OutsideList) {
		formError = '';
		editId = l?.id ?? null;
		name = l?.name ?? '';
		kind = l?.kind ?? 'tmdb_list';
		titleKind = l?.title_kind ?? 'movie';
		ref = l?.ref ?? '';
		folderId = l?.library_folder_id ?? folders.find((f) => f.kind === titleKind)?.id ?? 0;
		rulesId = l?.pick_rules_id ?? 0;
		keep = l?.keep_up_to_date ?? true;
		searchOnAdd = l?.search_on_add ?? false;
		seasons = l?.seasons || 'all';
		newSeasons = l?.new_seasons || 'all';
		enabled = l?.enabled ?? true;
		hours = l?.sync_every_hours ?? 12;
		formOpen = true;
	}

	/** Switches between a movie and a show list; folder and Pick rules start over. */
	function pickTitleKind() {
		folderId = folders.find((f) => f.kind === titleKind)?.id ?? 0;
		rulesId = 0;
	}

	async function save(e: SubmitEvent) {
		e.preventDefault();
		formError = '';
		saving = true;
		const body = {
			name: name.trim(),
			kind,
			title_kind: titleKind,
			ref: ref.trim(),
			library_folder_id: folderId,
			pick_rules_id: rulesId,
			keep_up_to_date: keep,
			search_on_add: searchOnAdd,
			...(titleKind === 'show' ? { seasons, new_seasons: seasons === 'future' ? 'all' : newSeasons } : {}),
			enabled,
			sync_every_hours: Number(hours) || 12
		};
		try {
			if (editId === null) await api.post<OutsideList>('/outside-lists', body);
			else await api.put<OutsideList>(`/outside-lists/${editId}`, body);
			formOpen = false;
			toasts.push(t('settings.outsideLists.saved'), 'ok', { body: enabled ? t('settings.outsideLists.readingNow') : t('settings.outsideLists.listOff') });
			await loadLists();
		} catch (err) {
			formError = message(err);
		} finally {
			saving = false;
		}
	}

	async function syncNow(l: OutsideList) {
		busyId = l.id;
		try {
			await api.post(`/outside-lists/${l.id}/sync`);
			toasts.push(t('settings.outsideLists.reading', { name: l.name }), 'info');
		} catch (err) {
			toasts.push(t('settings.outsideLists.readFailed'), 'fail', { body: message(err) });
		} finally {
			busyId = null;
		}
	}

	async function remove(l: OutsideList) {
		if (!confirm(l.title_kind === 'show' ? t('settings.outsideLists.confirmRemoveShows', { name: l.name }) : t('settings.outsideLists.confirmRemoveMovies', { name: l.name }))) return;
		busyId = l.id;
		try {
			await api.del(`/outside-lists/${l.id}`);
			lists = lists.filter((x) => x.id !== l.id);
			toasts.push(t('settings.outsideLists.removed'), 'ok');
		} catch (err) {
			toasts.push(t('settings.outsideLists.removeFailed'), 'fail', { body: message(err) });
		} finally {
			busyId = null;
		}
	}

	async function saveTrakt(e: SubmitEvent) {
		e.preventDefault();
		traktError = '';
		if (!traktKey.trim() && !traktVip.trim()) {
			traktError = t('settings.outsideLists.pasteId');
			return;
		}
		traktSaving = true;
		try {
			if (traktKey.trim()) traktMasked = (await api.put<{ key: string }>('/settings/trakt-key', { key: traktKey.trim() })).key;
			if (traktVip.trim()) traktVipMasked = (await api.put<{ token: string }>('/settings/trakt-token', { token: traktVip.trim() })).token;
			toasts.push(traktVip.trim() ? t('settings.outsideLists.traktSaved') : t('settings.outsideLists.traktKeySaved'), 'ok');
			traktKey = '';
			traktVip = '';
		} catch (err) {
			traktError = err instanceof ApiError && err.status === 403 ? t('settings.outsideLists.signInToChange') : message(err);
		} finally {
			traktSaving = false;
		}
	}

	async function addNever(e: SubmitEvent) {
		e.preventDefault();
		naError = '';
		const id = naId.trim().replace(/^tmdb:/i, '');
		if (!/^\d+$/.test(id) || !naName.trim()) {
			naError = t('settings.outsideLists.giveNumber');
			return;
		}
		try {
			const n = await api.post<NeverAddEntry>('/never-add', { kind: naKind, tmdb_id: id, name: naName.trim(), reason: naReason.trim() });
			never = [n, ...never.filter((x) => x.id !== n.id)];
			naId = naName = naReason = '';
		} catch (err) {
			naError = message(err);
		}
	}

	async function removeNever(n: NeverAddEntry) {
		try {
			await api.del(`/never-add/${n.id}`);
			never = never.filter((x) => x.id !== n.id);
			toasts.push(t('settings.outsideLists.allowedAgain', { name: n.name }), 'ok');
		} catch (err) {
			toasts.push(t('settings.outsideLists.updateFailed'), 'fail', { body: message(err) });
		}
	}
</script>

<svelte:head><title>{t('settings.outsideLists.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.outsideLists.title')}>
	{#snippet actions()}
		{#if !formOpen && loaded && hasData && lists.length}<Button icon="plus" variant="primary" type="button" onclick={() => openForm()}>{t('settings.outsideLists.add')}</Button>{/if}
	{/snippet}
</PageHeader>

{#if formOpen}
	<form class="sc-form" onsubmit={save} aria-label={editId === null ? t('settings.outsideLists.add') : t('settings.outsideLists.change', { name })}>
		<SettingsCard title={editId === null ? t('settings.outsideLists.add') : t('settings.outsideLists.change', { name })}>
			<Field id="ol-name" label={t('settings.outsideLists.name')}>
				<input id="ol-name" class="input" bind:value={name} maxlength="80" required placeholder={t('settings.outsideLists.nameExample')} />
			</Field>
			<Field id="ol-kind" label={t('settings.outsideLists.kind')}>
				<select id="ol-kind" class="input" bind:value={kind}>
					{#each listKinds.filter(([k]) => k !== 'trakt_list' || !session.held('trakt_lists')) as [k, label] (k)}<option value={k}>{label}</option>{/each}
				</select>
			</Field>
			<Field id="ol-holds" label={t('settings.outsideLists.adds')}>
				<select id="ol-holds" class="input" bind:value={titleKind} onchange={pickTitleKind}>
					{#each titleKinds as [k, label] (k)}<option value={k}>{label}</option>{/each}
				</select>
			</Field>
			<Field id="ol-ref" label={t('settings.outsideLists.which')}>
				<input id="ol-ref" class="input mono" bind:value={ref} required autocomplete="off" spellcheck="false" />
				<span class="hint">{listKinds.find(([k]) => k === kind)?.[2]}</span>
			</Field>
			<Field id="ol-folder" label={t('settings.outsideLists.folder')}>
				<select id="ol-folder" class="input" bind:value={folderId}>
					{#each kindFolders as f (f.id)}<option value={f.id}>{f.label || f.path} ({f.path})</option>{/each}
				</select>
				{#if kindFolders.length === 0}<span class="hint">{titleKind === 'show' ? t('settings.outsideLists.needShowFolder') : t('settings.outsideLists.needMovieFolder')}</span>{/if}
			</Field>
			<Field id="ol-rules" label={t('settings.outsideLists.quality')}>
				<select id="ol-rules" class="input" bind:value={rulesId}>
					<option value={0}>{t('settings.outsideLists.folderOwn')}</option>
					{#each kindRules as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
				</select>
			</Field>
			{#if titleKind === 'show'}
				<Field id="ol-seasons" label={t('settings.outsideLists.seasons')}>
					<select id="ol-seasons" class="input" bind:value={seasons} aria-describedby="ol-seasons-hint">
						{#each listSeasons as [v, l] (v)}<option value={v}>{l}</option>{/each}
					</select>
					<span class="hint" id="ol-seasons-hint">{listSeasons.find(([v]) => v === seasons)?.[2]}</span>
				</Field>
				<Field id="ol-new" label={t('settings.outsideLists.newSeasons')}>
					<select id="ol-new" class="input" bind:value={newSeasons} disabled={seasons === 'future'} aria-describedby="ol-new-hint">
						<option value="all">{t('settings.outsideLists.newAll')}</option>
						<option value="none">{t('settings.outsideLists.newNone')}</option>
					</select>
					<span class="hint" id="ol-new-hint">{seasons === 'future' ? t('settings.outsideLists.futureHint') : t('settings.outsideLists.laterHint')}</span>
				</Field>
			{/if}
			<Field id="ol-hours" label={t('settings.outsideLists.hours')} hint={t('settings.outsideLists.hoursHint')}>
				<input id="ol-hours" class="input" type="number" min="6" max="168" bind:value={hours} />
			</Field>
			<label class="checkline">
				<input type="checkbox" bind:checked={keep} />
				<span><b>{t('settings.outsideLists.keep')}</b><br /><span class="hint">{titleKind === 'show' ? t('settings.outsideLists.keepShows') : t('settings.outsideLists.keepMovies')}</span></span>
			</label>
			<label class="checkline">
				<input type="checkbox" bind:checked={searchOnAdd} />
				<span><b>{t('settings.outsideLists.search')}</b><br /><span class="hint">{titleKind === 'show' ? t('settings.outsideLists.searchShows') : t('settings.outsideLists.searchMovies')}</span></span>
			</label>
			<label class="checkline">
				<input type="checkbox" bind:checked={enabled} />
				<span><b>{t('settings.outsideLists.readThis')}</b></span>
			</label>
			{#if kind === 'trakt_list' && !traktMasked}<p class="notice warn">{t('settings.outsideLists.needKey')}</p>{/if}
			{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
			<div class="row" style="margin-top: var(--s-5)">
				<Button variant="primary" type="submit" disabled={saving || kindFolders.length === 0} aria-busy={saving}>{t('settings.outsideLists.save')}</Button>
				<Button variant="plain" type="button" onclick={() => (formOpen = false)}>{t('settings.outsideLists.cancel')}</Button>
			</div>
		</SettingsCard>
	</form>
{/if}

{#if !loaded}
	<SkSettings rows={2} />
{:else if !hasData}
	<LoadError hasData={false} message={error} onretry={load} />
{:else}
	{#if error}<LoadError hasData={true} message={error} onretry={load} />{/if}
	{#if lists.length === 0 && !formOpen}
		<EmptyState icon="inbox" title={t('settings.outsideLists.none')} text={t('settings.outsideLists.noneText')} action={{ label: t('settings.outsideLists.add'), onclick: () => openForm(), icon: 'plus' }} />
	{:else if lists.length}
		<ul class="card rows" aria-label={t('settings.outsideLists.title')}>
			{#each lists as l (l.id)}
				<li class="list" data-testid="outside-list">
					<div class="grow">
						<div class="row" style="gap: var(--s-2)">
							<b>{l.name}</b>
							<span class="chip">{listKinds.find(([k]) => k === l.kind)?.[1]}</span>
							<span class="chip">{l.title_kind === 'show' ? t('settings.outsideLists.shows') : t('settings.outsideLists.movies')}</span>
							{#if !l.enabled}<span class="chip">{t('settings.outsideLists.off')}</span>{/if}
							{#if l.last_error}<span class="chip fail">{t('settings.outsideLists.problem')}</span>{/if}
						</div>
						<div class="xs muted">
							{#if listLink(l.kind, l.ref)}<a class="ul" href={listLink(l.kind, l.ref)} target="_blank" rel="noreferrer noopener">{l.ref}</a>{:else}{l.ref}{/if}
							· {t('settings.outsideLists.readEvery', { every: every(l.sync_every_hours) })}
						</div>
						{#if l.last_synced_at}
							<p class="small" data-testid="list-result">{t('settings.outsideLists.lastRead', { when: dateTime(l.last_synced_at), result: l.last_result })}</p>
						{:else}
							<p class="small muted">{t('settings.outsideLists.notRead')}</p>
						{/if}
						{#if l.last_error}<p class="small bad-text" role="status">{l.last_error}</p>{/if}
					</div>
					<div class="row acts">
						<Button icon="refresh" type="button" onclick={() => syncNow(l)} disabled={busyId === l.id || !l.enabled} aria-busy={busyId === l.id}>{t('settings.outsideLists.readNow')}</Button>
						<Button variant="plain" type="button" onclick={() => openForm(l)}>{t('settings.outsideLists.changeButton')}</Button>
						<Button variant="plain" type="button" onclick={() => remove(l)} disabled={busyId === l.id}>{t('settings.outsideLists.remove')}</Button>
					</div>
				</li>
			{/each}
		</ul>
	{/if}

	{#if !session.held('trakt_lists')}
	<section class="card" id="trakt-card" aria-labelledby="h-trakt">
		<div class="card-head">
			<h2 id="h-trakt" class="h2">{t('settings.outsideLists.traktKey')}</h2>
			<span class="chip {traktMasked ? 'ok' : ''}">{traktMasked ? t('settings.outsideLists.keySaved') : t('settings.outsideLists.notSet')}</span>
		</div>
		<p class="small muted">
			{t('settings.outsideLists.traktInfo')}{hasTrakt && !traktMasked ? ' ' + t('settings.outsideLists.traktCannotRead') : ''}
		</p>
		<form onsubmit={saveTrakt} style="margin-top: var(--s-4)">
			<Field id="trakt" label={traktMasked ? t('settings.outsideLists.replaceKey') : t('settings.outsideLists.pasteKey')}>
				<input id="trakt" class="input mono" type="password" bind:value={traktKey} autocomplete="off" spellcheck="false" />
			</Field>
			{#if session.s.role === 'owner'}
				<Field id="trakt-token" label={traktVipMasked ? t('settings.outsideLists.replaceVip') : t('settings.outsideLists.vip')} hint={t('settings.outsideLists.vipHint')}>
					<input id="trakt-token" class="input mono" type="password" bind:value={traktVip} autocomplete="off" spellcheck="false" />
				</Field>
			{/if}
			{#if traktError}<p class="form-error" role="alert">{traktError}</p>{/if}
			<Button type="submit" disabled={traktSaving} aria-busy={traktSaving}>{t('settings.outsideLists.save')}</Button>
		</form>
	</section>
	{/if}

	<SettingsCard id="never-card" title={t('settings.outsideLists.never')} description={t('settings.outsideLists.neverDesc')}>
		<form class="never-form" onsubmit={addNever}>
			<Field id="na-kind" label={t('settings.outsideLists.movieOrShow')}>
				<select id="na-kind" class="input" bind:value={naKind}>
					<option value="movie">{t('settings.outsideLists.movie')}</option>
					<option value="show">{t('settings.outsideLists.show')}</option>
				</select>
			</Field>
			<Field id="na-id" label={t('settings.outsideLists.tmdb')}>
				<input id="na-id" class="input mono" bind:value={naId} inputmode="numeric" autocomplete="off" />
			</Field>
			<Field id="na-name" label={t('settings.outsideLists.name')}>
				<input id="na-name" class="input" bind:value={naName} maxlength="200" />
			</Field>
			<div class="field">
				<label for="na-reason">{t('settings.outsideLists.reason')} <span class="muted">{t('settings.outsideLists.optional')}</span></label>
				<input id="na-reason" class="input" bind:value={naReason} maxlength="200" />
			</div>
			<div class="na-act"><Button type="submit">{t('settings.outsideLists.addNever')}</Button></div>
		</form>
		{#if naError}<p class="form-error" role="alert">{naError}</p>{/if}
		{#if never.length}
			<ul class="never" aria-label={t('settings.outsideLists.never')}>
				{#each never as n (n.id)}
					<li data-testid="never-add">
						<span class="grow"><b>{n.name}</b> <span class="xs muted mono">{n.kind === 'show' ? t('settings.outsideLists.showDot') + ' ' : ''}tmdb:{n.ids.tmdb}</span>{#if n.reason}<span class="xs muted"> · {n.reason}</span>{/if}</span>
						<Button variant="plain" type="button" onclick={() => removeNever(n)}>{t('settings.outsideLists.takeOff', { name: n.name })}</Button>
					</li>
				{/each}
			</ul>
		{:else}
			<p class="small muted" style="margin-top: var(--s-3)">{t('settings.outsideLists.empty')}</p>
		{/if}
	</SettingsCard>
{/if}

<style>
	.list {
		display: flex;
		gap: var(--s-4);
		align-items: flex-start;
		padding: var(--s-4);
	}
	.list + .list {
		border-top: 1px solid var(--line);
	}
	.acts {
		flex: none;
		gap: var(--s-2);
	}
	.ul {
		text-decoration: underline;
	}
	.bad-text {
		color: var(--fail-text);
	}
	.never-form {
		margin-top: var(--s-4);
	}
	.na-act {
		margin-top: var(--s-4);
	}
	.never li {
		display: flex;
		gap: var(--s-3);
		align-items: center;
		padding: var(--s-2) 0;
	}
	.never li + li {
		border-top: 1px solid var(--line);
	}
	@media (max-width: 700px) {
		.list {
			flex-direction: column;
		}
		.acts {
			flex-wrap: wrap;
		}
	}
</style>
