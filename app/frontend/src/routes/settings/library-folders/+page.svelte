<script lang="ts">
	import { ask } from '#lib/ui/ask.svelte.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import Field from '#lib/ui/Field.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import Menu from '#lib/ui/Menu.svelte';
	import TagInput from '#lib/ui/TagInput.svelte';
	import { forgetTags, switchDropped } from '#lib/ui/tags.ts';
	import { onMount } from 'svelte';
	import { api } from '#lib/core/api.ts';
	import { recheckSettingsStatus } from '#lib/alerts/index.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { folderMissing, qualityFor, qualityBody, qualityValue, qualityChoices, defaultChoice, savedDefaults } from '#lib/mediafiles/index.ts';
	import type { TitleDefaults } from '#lib/mediafiles/index.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { bytes, dateTime, folderHealth, tone } from '#lib/core/format.ts';
	import type { LibraryFolder, PickRulesView } from '#lib/core/types.ts';

	// Grouped list with a plain line icon per folder.
	const kindWord = (k: string) => (k === 'show' ? t('settings.libraryFolders.shows') : k === 'music' ? t('settings.libraryFolders.music') : t('settings.libraryFolders.movies'));
	const kindIcon = (k: string) => (k === 'show' ? 'tv' : k === 'music' ? 'music' : 'film') as 'tv' | 'music' | 'film';
	// The last two parts of a path: enough to tell folders apart; the full path is in the tooltip.
	const shortPath = (p: string) => { const parts = p.split(/[\\/]/).filter(Boolean); return parts.length > 2 ? '…/' + parts.slice(-2).join('/') : p; };
	const freeNum = (n: number) => bytes(n).split(' ');
	const byKind = $derived(['movie', 'show', 'music'].map((k) => ({ k, items: folders.filter((f) => (f.kind || 'movie') === k) })).filter((g) => g.items.length));

	let folders = $state<LibraryFolder[]>([]);
	let loaded = $state(false);
	// Quality sets, for each folder's own quality. Left empty (and the choice hidden) when they cannot be read.
	let rules = $state<PickRulesView[]>([]);
	// The saved Movies and TV defaults name the set a folder with no quality of its own hands on.
	let titleDefaults = $state<TitleDefaults | null>(null);
	const canEdit = $derived(session.can('settings'));
	let error = $state('');

	let adding = $state(false);
	let path = $state('');
	let label = $state('');
	let kind = $state<'movie' | 'show' | 'music'>('movie');
	let quality = $state('');
	let newTags = $state<string[]>([]);
	let onlyTagged = $state(false);
	// Taking the last tag away turns "only these tags" off; the form and the row say so until a tag is there again or the form is done.
	let addDropped = $state(false);
	let rowDropped = $state<number[]>([]);
	function setNewTags(v: string[]) {
		if (switchDropped({ tags: newTags, only: onlyTagged }, v)) (addDropped = true), (onlyTagged = false);
		else if (v.length) addDropped = false;
		newTags = v;
	}
	let formError = $state('');
	let saving = $state(false);

	let editing = $state<number | null>(null);
	let editLabel = $state('');
	let busyId = $state<number | null>(null);

	// Change path: the files were already moved; the server checks each one and moves nothing.
	let moving = $state<number | null>(null);
	let movePath = $state('');
	let moveError = $state('');
	let moveBusy = $state(false);
	function openMove(f: LibraryFolder) {
		moving = moving === f.id ? null : f.id;
		movePath = f.path;
		moveError = '';
	}
	async function changePath(f: LibraryFolder) {
		moveError = '';
		const p = movePath.trim();
		if (!p) {
			moveError = t('settings.libraryFolders.pathNeeded');
			return;
		}
		if (p === f.path) {
			moving = null;
			return;
		}
		moveBusy = true;
		busyId = f.id;
		try {
			const n = await api.put<LibraryFolder>(`/library-folders/${f.id}/path`, { path: p });
			took(n);
			moving = null;
			toasts.push(t('settings.libraryFolders.pathChanged'), n.health === 'ok' ? 'ok' : 'info', { body: n.health === 'ok' ? undefined : folderHealth(n.health) });
		} catch (err) {
			moveError = folderMissing(err, p, f.kind) ?? message(err);
		} finally {
			moveBusy = false;
			busyId = null;
		}
	}

	// Extra import folders: where Import files by hand may also read.
	let extraText = $state('');
	let extraLoaded = $state(false);
	let extraError = $state('');
	let extraSaving = $state(false);
	const canSaveExtra = $derived(session.can('users'));
	// Subtitle files brought along on import: off with srt until turned on.
	let side = $state<{ on: boolean; types: string[] } | null>(null);
	let sideTypes = $state('srt');
	let sideError = $state('');
	let sideSaving = $state(false);
	const canSaveSide = $derived(session.can('settings'));
	async function loadSide() {
		try {
			side = await api.get<{ on: boolean; types: string[] }>('/settings/side-files');
			sideTypes = side.types.join(', ');
		} catch {
			side = null; // No settings permission: the card stays hidden.
		}
	}
	async function saveSide(e: SubmitEvent) {
		e.preventDefault();
		if (!side) return;
		sideError = '';
		sideSaving = true;
		try {
			const types = sideTypes.split(/[\s,]+/).map((x) => x.trim()).filter(Boolean);
			side = await api.put<{ on: boolean; types: string[] }>('/settings/side-files', { on: side.on, types });
			sideTypes = side.types.join(', ');
			toasts.push(t('settings.libraryFolders.importSaved'), 'ok');
		} catch (err) {
			sideError = message(err);
		} finally {
			sideSaving = false;
		}
	}

	async function load() {
		try {
			folders = (await api.get<{ library_folders: LibraryFolder[] }>('/library-folders')).library_folders;
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}
	async function loadRules() {
		try {
			rules = (await api.get<{ pick_rules: PickRulesView[] }>('/pick-rules')).pick_rules;
		} catch {
			rules = [];
		}
	}
	async function loadExtra() {
		try {
			extraText = (await api.get<{ extra_folders: string[] }>('/settings/import-folders')).extra_folders.join('\n');
			extraLoaded = true;
		} catch {
			extraLoaded = false; // No settings permission: the card stays hidden.
		}
	}
	onMount(() => {
		void load();
		void loadRules();
		void savedDefaults.load().then((d) => (titleDefaults = d));
		void loadExtra();
		void loadSide();
	});

	async function saveExtra(e: SubmitEvent) {
		e.preventDefault();
		extraError = '';
		extraSaving = true;
		try {
			const lines = extraText.split('\n').map((l) => l.trim()).filter(Boolean);
			extraText = (await api.put<{ extra_folders: string[] }>('/settings/import-folders', { extra_folders: lines })).extra_folders.join('\n');
			toasts.push(t('settings.libraryFolders.importSaved'), 'ok');
		} catch (err) {
			extraError = message(err);
		} finally {
			extraSaving = false;
		}
	}

	async function add(e: SubmitEvent) {
		e.preventDefault();
		formError = '';
		if (!path.trim()) {
			formError = t('settings.libraryFolders.pathNeeded');
			return;
		}
		saving = true;
		try {
			const f = await api.post<LibraryFolder>('/library-folders', { path: path.trim(), kind, label: label.trim(), new_title_tags: newTags, only_tagged_sources: newTags.length > 0 && onlyTagged, ...(quality ? qualityBody(quality) : {}) });
			folders = [...folders, f];
			path = '';
			label = '';
			kind = 'movie';
			quality = '';
			newTags = [];
			onlyTagged = false;
			addDropped = false;
			forgetTags();
			adding = false;
			void recheckSettingsStatus('library_folder');
			toasts.push(t('settings.libraryFolders.added'), f.health === 'ok' ? 'ok' : 'info', { body: f.health === 'ok' ? undefined : folderHealth(f.health) });
		} catch (err) {
			formError = folderMissing(err, path, kind) ?? message(err);
		} finally {
			saving = false;
		}
	}

	// Takes a row the server sent back after another kind of save. The tag choices on screen stay: their own saves look after them, and one may still be on its way.
	const took = (n: LibraryFolder) => (folders = folders.map((x) => (x.id === n.id ? { ...n, new_title_tags: x.new_title_tags, only_tagged_sources: x.only_tagged_sources } : x)));

	async function recheck(f: LibraryFolder) {
		busyId = f.id;
		try {
			const n = await api.post<LibraryFolder>(`/library-folders/${f.id}/check`);
			took(n);
			toasts.push(t('settings.libraryFolders.checked'), n.health === 'ok' ? 'ok' : 'info', { body: folderHealth(n.health) });
			void recheckSettingsStatus('library_folder');
		} catch (err) {
			toasts.push(t('settings.libraryFolders.checkFailed'), 'fail', { body: message(err) });
		} finally {
			busyId = null;
		}
	}

	async function saveLabel(f: LibraryFolder) {
		busyId = f.id;
		try {
			const n = await api.put<LibraryFolder>(`/library-folders/${f.id}`, { label: editLabel.trim() });
			took(n);
			editing = null;
		} catch (err) {
			toasts.push(t('settings.libraryFolders.couldntSave'), 'fail', { body: message(err) });
		} finally {
			busyId = null;
		}
	}

	// Shows the choice at once and puts the old one back when the save fails.
	async function setQuality(f: LibraryFolder, id: number) {
		const was = f.default_pick_rules_id;
		if ((was ?? 0) === id) return;
		const put = (v: number | null) => (folders = folders.map((x) => (x.id === f.id ? { ...x, default_pick_rules_id: v } : x)));
		put(id || null);
		busyId = f.id;
		try {
			const n = await api.put<LibraryFolder>(`/library-folders/${f.id}`, qualityBody(id));
			took(n);
		} catch (err) {
			put(was);
			toasts.push(t('settings.libraryFolders.couldntSave'), 'fail', { body: message(err) });
		} finally {
			busyId = null;
		}
	}

	// Each change is saved at once. A folder's saves go one after another and each sends the newest choices, so the last one made is the one kept; when the newest one fails, the row shows what the server has.
	const tagSeq = new Map<number, number>();
	const tagQueue = new Map<number, Promise<void>>();
	function saveFolderTags(f: LibraryFolder, patch: { new_title_tags?: string[]; only_tagged_sources?: boolean }) {
		const id = f.id;
		const mine = (tagSeq.get(id) ?? 0) + 1;
		tagSeq.set(id, mine);
		const typed = patch.new_title_tags;
		if (typed) {
			const dropped = switchDropped({ tags: f.new_title_tags ?? [], only: !!f.only_tagged_sources }, typed);
			if (dropped) patch = { ...patch, only_tagged_sources: false };
			if (dropped || typed.length) rowDropped = dropped ? [...rowDropped, id] : rowDropped.filter((x) => x !== id);
		}
		folders = folders.map((x) => (x.id === id ? { ...x, ...patch } : x));
		const run = async () => {
			const cur = folders.find((x) => x.id === id);
			if (!cur) return;
			try {
				const n = await api.put<LibraryFolder>(`/library-folders/${id}`, { new_title_tags: cur.new_title_tags ?? [], only_tagged_sources: !!cur.only_tagged_sources });
				if (mine === tagSeq.get(id)) folders = folders.map((x) => (x.id === id ? { ...x, new_title_tags: n.new_title_tags ?? [], only_tagged_sources: !!n.only_tagged_sources } : x));
				forgetTags();
			} catch (err) {
				if (mine === tagSeq.get(id)) {
					try {
						const all = (await api.get<{ library_folders: LibraryFolder[] }>('/library-folders')).library_folders;
						const real = all.find((x) => x.id === id);
						if (real && mine === tagSeq.get(id)) {
							folders = folders.map((x) => (x.id === id ? { ...x, new_title_tags: real.new_title_tags ?? [], only_tagged_sources: !!real.only_tagged_sources } : x));
							// The row shows what the server holds again, so the line about the switch no longer applies.
							rowDropped = rowDropped.filter((x) => x !== id);
						}
					} catch {
						// The error below already tells the person the save failed.
					}
				}
				toasts.push(t('settings.libraryFolders.couldntSave'), 'fail', { body: message(err) });
			}
		};
		tagQueue.set(id, (tagQueue.get(id) ?? Promise.resolve()).then(run));
	}

	// Copying on purpose (downloads and media on separate disks): the folder then counts as fine.
	const copiesOk = (f: LibraryFolder) => f.health === 'no_hardlink' && !!f.copies_expected;
	const rowTone = (f: LibraryFolder) => (copiesOk(f) ? 'ok' : tone(f.health));
	const rowHealth = (f: LibraryFolder) => (copiesOk(f) ? t('settings.libraryFolders.copiesExpectedState') : f.health === 'ok' ? t('settings.libraryFolders.stateInstant') : folderHealth(f.health));
	// The one /data folder layout that makes imports instant.
	const copiesDoc = 'https://docs.holyarrs.com/start/folders/';
	async function setCopies(f: LibraryFolder, on: boolean) {
		busyId = f.id;
		try {
			const n = await api.put<LibraryFolder>(`/library-folders/${f.id}`, { copies_expected: on });
			took(n);
			// The folder checks run again, so the warning and the menu dot follow at once.
			void recheckSettingsStatus('library_folder');
		} catch (err) {
			toasts.push(t('settings.libraryFolders.couldntSave'), 'fail', { body: message(err) });
		} finally {
			busyId = null;
		}
	}

	async function remove(f: LibraryFolder) {
		if (!(await ask(t('settings.libraryFolders.confirmRemove', { path: f.path }), { yes: t('ui.ask.remove'), danger: true }))) return;
		busyId = f.id;
		try {
			await api.del(`/library-folders/${f.id}`);
			folders = folders.filter((x) => x.id !== f.id);
			toasts.push(t('settings.libraryFolders.removed'), 'ok');
			void recheckSettingsStatus('library_folder');
		} catch (err) {
			toasts.push(t('settings.libraryFolders.removeFailed'), 'fail', { body: message(err) });
		} finally {
			busyId = null;
		}
	}
</script>

<svelte:head><title>{t('settings.libraryFolders.title')} · HolyArrs</title></svelte:head>

<PageHeader title={t('settings.libraryFolders.title')}>
	{#snippet actions()}
		{#if !adding && folders.length}<Button icon="plus" variant="primary" type="button" onclick={() => (adding = true)}>{t('settings.libraryFolders.add')}</Button>{/if}
	{/snippet}
</PageHeader>

{#if adding}
	<form class="sc-form" onsubmit={add} aria-label={t('settings.libraryFolders.addFolder')}>
		<SettingsCard title={t('settings.libraryFolders.addFolder')}>
			<Field id="lf-kind" label={t('settings.libraryFolders.holds')} hint={t('settings.libraryFolders.holdsHint')}>
				<select id="lf-kind" class="input" bind:value={kind} onchange={() => (quality = '')}>
					<option value="movie">{t('settings.libraryFolders.movies')}</option>
					<option value="show">{t('settings.libraryFolders.shows')}</option>
					<option value="music">{t('settings.libraryFolders.music')}</option>
				</select>
			</Field>
			<Field id="lf-path" label={t('settings.libraryFolders.path')}>
				<input id="lf-path" class="input mono" bind:value={path} placeholder={kind === 'show' ? '/shows' : kind === 'music' ? '/music' : '/movies'} autocomplete="off" spellcheck="false" />
			</Field>
			<div class="field">
				<label for="lf-label">{t('settings.libraryFolders.name')} <span class="muted">{t('settings.libraryFolders.optional')}</span></label>
				<input id="lf-label" class="input" bind:value={label} placeholder={kindWord(kind)} maxlength="64" />
			</div>
			{#if rules.length}
				<Field id="lf-quality" label={t('settings.libraryFolders.quality')} hint={t('settings.libraryFolders.qualityHint')}>
					<select id="lf-quality" class="input" bind:value={quality} disabled={!canEdit}>
						<option value="">{defaultChoice(rules, kind, titleDefaults)}</option>
						{#each qualityFor(rules, kind) as r (r.id)}<option value={String(r.id)}>{r.name}</option>{/each}
					</select>
				</Field>
			{/if}
			<Field id="lf-tags" label={t('settings.libraryFolders.newTags')} hint={t('settings.libraryFolders.newTagsHint')}>
				<TagInput id="lf-tags" bind:value={() => newTags, setNewTags} describedby="lf-tags-hint" testid="lf-tags" />
			</Field>
			{#if newTags.length}
				<label class="only"><input type="checkbox" bind:checked={onlyTagged} aria-describedby="lf-only-hint" data-testid="lf-only" /><span>{t('settings.libraryFolders.onlyTagged')}</span></label>
				<p class="hint only-hint" id="lf-only-hint">{t('settings.libraryFolders.onlyTaggedHint')}</p>
			{:else if addDropped}
				<p class="hint only-off" role="status" data-testid="lf-only-off">{t('settings.libraryFolders.onlyTaggedOff', { label: t('settings.libraryFolders.onlyTagged') })}</p>
			{/if}
			{#if formError}<p class="form-error" role="alert">{formError}</p>{/if}
			<div class="row" style="margin-top: var(--s-5)">
				<Button variant="primary" type="submit" disabled={saving} aria-busy={saving}>{t('settings.libraryFolders.addButton')}</Button>
				<Button type="button" onclick={() => ((adding = false), (formError = ''), (addDropped = false))}>{t('settings.libraryFolders.cancel')}</Button>
			</div>
		</SettingsCard>
	</form>
{/if}

{#if !loaded}
	<SkSettings rows={2} />
{:else if error}
	<LoadError hasData={false} message={error} onretry={load} />
{:else if folders.length === 0 && !adding}
	<EmptyState icon="folder" title={t('settings.libraryFolders.none')} text={t('settings.libraryFolders.noneText')} action={{ label: t('settings.libraryFolders.add'), icon: 'plus', onclick: () => (adding = true) }} />
{:else if folders.length}
	{#snippet row(f: LibraryFolder)}
		<li class="folder k-{f.kind || 'movie'}" title="{f.path}{f.health_checked_at ? ` · ${t('settings.libraryFolders.checkedAt', { when: dateTime(f.health_checked_at) })}` : ''}">
			<span class="ic"><Icon name="folder" size={20} /></span>
			<div class="grow">
				{#if editing === f.id}
					<form class="ed" onsubmit={(e) => (e.preventDefault(), saveLabel(f))}>
						<label class="sr-only" for="ed-{f.id}">{t('settings.libraryFolders.name')}</label>
						<input id="ed-{f.id}" class="input" bind:value={editLabel} maxlength="64" />
						<Button variant="primary" type="submit" disabled={busyId === f.id}>{t('settings.libraryFolders.save')}</Button>
						<Button type="button" onclick={() => (editing = null)}>{t('settings.libraryFolders.cancel')}</Button>
					</form>
				{:else}
					<div class="top"><span class="name">{f.label || f.path}</span><span class="kind" data-testid="folder-kind">{f.kind === 'show' ? t('settings.libraryFolders.forShows') : f.kind === 'music' ? t('settings.libraryFolders.forMusic') : t('settings.libraryFolders.forMovies')}</span></div>
				{/if}
				<div class="where"><bdi dir="ltr">{shortPath(f.path)}</bdi></div>
				{#if f.health_reason && rowTone(f) !== 'ok'}<div class="why {rowTone(f)}">{f.health_reason}</div>{/if}
			</div>
			<div class="free"><b>{freeNum(f.free_space)[0]}</b><small>{t('settings.libraryFolders.free', { unit: freeNum(f.free_space)[1] ?? '' })}</small></div>
			{#if rules.length}
				{@const q = qualityValue(f, rules, titleDefaults)}
				<!-- A settings row: the name, then the value; the menu opens only on a tap. -->
				<div class="qpick" class:own={q.own} data-testid="folder-quality">
					{#if canEdit}
						<Menu label={t('settings.libraryFolders.qualityFor', { name: f.label || f.path, value: q.text })} variant="row" align="end" width={240} portal items={qualityChoices(rules, f.kind, f.default_pick_rules_id, titleDefaults).map((c) => ({ label: c.label, checked: c.checked, onselect: () => void setQuality(f, c.id) }))}>
							{#snippet trigger()}<span class="ql">{t('settings.libraryFolders.quality')}</span><span class="qv" title={q.text}>{q.text}</span><span class="qc" aria-hidden="true"><Icon name="chevronRight" size={16} /></span>{/snippet}
						</Menu>
					{:else}
						<div class="qro"><span class="ql">{t('settings.libraryFolders.quality')}</span><span class="qv">{q.text}</span></div>
					{/if}
				</div>
			{/if}
			<span class="st {rowTone(f)}"><i></i><span>{rowHealth(f)}</span>{#if f.health === 'no_hardlink'}<a class="why-link" href={copiesDoc} target="_blank" rel="noopener noreferrer">{t('settings.libraryFolders.whyCopies')}</a>{/if}</span>
			<div class="acts">
				<IconButton icon="refresh" label={t('settings.libraryFolders.checkAgain', { path: f.path })} type="button" tooltip={false} disabled={busyId === f.id} onclick={() => recheck(f)} />
				{#if f.health === 'no_hardlink'}<Button type="button" disabled={busyId === f.id} onclick={() => setCopies(f, !f.copies_expected)}>{f.copies_expected ? t('settings.libraryFolders.warnAgain') : t('settings.libraryFolders.copiesExpected')}</Button>{/if}
				<IconButton icon="pencil" label={t('settings.libraryFolders.renameOf', { path: f.path })} type="button" tooltip={false} disabled={busyId === f.id} onclick={() => ((editing = f.id), (editLabel = f.label))} />
				<IconButton icon="folderMove" label={t('settings.libraryFolders.changePathOf', { path: f.path })} type="button" tooltip={false} disabled={busyId === f.id} aria-expanded={moving === f.id} onclick={() => openMove(f)} />
				<IconButton icon="trash" label={t('settings.libraryFolders.removePath', { path: f.path })} type="button" tooltip={false} disabled={busyId === f.id} onclick={() => remove(f)} />
			</div>
			{#if canEdit || f.new_title_tags?.length}
				<div class="nt" data-testid="folder-tags">
					<label class="ql" for="nt-{f.id}">{t('settings.libraryFolders.newTags')}</label>
					<div class="nv">
						<TagInput id="nt-{f.id}" bind:value={() => f.new_title_tags ?? [], (v) => saveFolderTags(f, { new_title_tags: v })} disabled={!canEdit} describedby="nt-{f.id}-hint" testid="nt-{f.id}" />
						<p class="hint" id="nt-{f.id}-hint">{t('settings.libraryFolders.newTagsHint')}</p>
						{#if f.new_title_tags?.length}
							<label class="only"><input type="checkbox" checked={!!f.only_tagged_sources} disabled={!canEdit} onchange={(e) => saveFolderTags(f, { only_tagged_sources: e.currentTarget.checked })} aria-describedby="nt-only-{f.id}-hint" data-testid="nt-only-{f.id}" /><span>{t('settings.libraryFolders.onlyTagged')}</span></label>
							<p class="hint only-hint" id="nt-only-{f.id}-hint">{t('settings.libraryFolders.onlyTaggedHint')}</p>
						{:else if rowDropped.includes(f.id)}
							<p class="hint only-off" role="status" data-testid="nt-only-off-{f.id}">{t('settings.libraryFolders.onlyTaggedOff', { label: t('settings.libraryFolders.onlyTagged') })}</p>
						{/if}
					</div>
				</div>
			{/if}
			{#if moving === f.id}
				<form class="mv" onsubmit={(e) => (e.preventDefault(), changePath(f))} aria-label={t('settings.libraryFolders.changePath')}>
					<label for="mv-{f.id}">{t('settings.libraryFolders.newPath')}</label>
					<div class="mv-row">
						<input id="mv-{f.id}" class="input mono" dir="ltr" bind:value={movePath} autocomplete="off" spellcheck="false" disabled={moveBusy} aria-describedby="mv-{f.id}-hint" />
						<Button variant="primary" type="submit" busy={moveBusy} disabled={moveBusy}>{moveBusy ? t('settings.libraryFolders.checkingFiles') : t('settings.libraryFolders.changePath')}</Button>
						<Button type="button" disabled={moveBusy} onclick={() => (moving = null)}>{t('settings.libraryFolders.cancel')}</Button>
					</div>
					<p class="hint" id="mv-{f.id}-hint">{t('settings.libraryFolders.changePathHint')}</p>
					{#if moveError}<p class="form-error" role="alert">{moveError}</p>{/if}
				</form>
			{/if}
		</li>
	{/snippet}
	<div class="fl-groups">
		{#each byKind as g (g.k)}
			<section class="fl-group">
				<h3><Icon name={kindIcon(g.k)} size={15} />{kindWord(g.k)}<span class="cnt">{g.items.length}</span></h3>
				<ul class="fl" data-view="3">{#each g.items as f (f.id)}{@render row(f)}{/each}</ul>
			</section>
		{/each}
	</div>
{/if}

{#if extraLoaded}
	<form class="sc-form extra" onsubmit={saveExtra} aria-label={t('settings.libraryFolders.extra')}>
		<SettingsCard id="import-folders" title={t('settings.libraryFolders.extra')} description={t('settings.libraryFolders.extraDesc')}>
			<Field id="if-paths" label={t('settings.libraryFolders.folders')} hint={t('settings.libraryFolders.foldersHint')} error={extraError}>
				<textarea id="if-paths" class="input mono" rows="3" bind:value={extraText} placeholder="/data/imports" autocomplete="off" spellcheck="false" readonly={!canSaveExtra}></textarea>
			</Field>
			{#if canSaveExtra}
				<div class="row" style="margin-top: var(--s-5)">
					<Button variant="primary" type="submit" disabled={extraSaving} aria-busy={extraSaving}>{t('settings.libraryFolders.save')}</Button>
				</div>
			{:else}
				<p class="hint">{t('settings.libraryFolders.ownerOnly')}</p>
			{/if}
		</SettingsCard>
	</form>
{/if}

{#if side}
	<form class="sc-form extra" onsubmit={saveSide} aria-label={t('settings.libraryFolders.sideFiles')} data-testid="side-files">
		<SettingsCard id="side-files" title={t('settings.libraryFolders.sideFiles')} description={t('settings.libraryFolders.sideFilesDesc')}>
			<label class="checkline">
				<input type="checkbox" bind:checked={side.on} disabled={!canSaveSide} data-testid="side-files-on" />
				<span><b>{t('settings.libraryFolders.sideOn')}</b><br /><span class="hint">{t('settings.libraryFolders.sideOnHint')}</span></span>
			</label>
			<Field id="side-types" label={t('settings.libraryFolders.sideTypes')} hint={t('settings.libraryFolders.sideTypesHint')} error={sideError}>
				<input id="side-types" class="input mono" bind:value={sideTypes} placeholder="srt" autocomplete="off" spellcheck="false" readonly={!canSaveSide} />
			</Field>
			{#if canSaveSide}
				<div class="row" style="margin-top: var(--s-5)">
					<Button variant="primary" type="submit" disabled={sideSaving} aria-busy={sideSaving}>{t('settings.libraryFolders.save')}</Button>
				</div>
			{:else}
				<p class="hint">{t('settings.libraryFolders.ownerOnly')}</p>
			{/if}
		</SettingsCard>
	</form>
{/if}

<style>
	.extra {
		margin-top: var(--s-5);
	}
	textarea.input {
		height: auto;
		padding-top: 10px;
		width: 100%;
		min-width: 0;
	}
	/* The folder list: shared pieces. */
	.fl { list-style: none; margin: 0; padding: 0; }
	.folder { position: relative; display: grid; grid-template-columns: 40px minmax(0, 1fr) auto auto auto; align-items: center; column-gap: 16px; min-width: 0; }
	/* Kind tiles: glossy, the same family as the Settings icons. */
	.ic { width: 40px; height: 40px; display: grid; place-items: center; border-radius: 11px; color: #fff; background: linear-gradient(160deg, #5aa4ff, #2563eb); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.35), 0 4px 10px -4px rgb(37 99 235 / 0.55); }
	.k-show .ic { background: linear-gradient(160deg, #b18cff, #7c3aed); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.35), 0 4px 10px -4px rgb(124 58 237 / 0.55); }
	.k-music .ic { background: linear-gradient(160deg, #ff8fb1, #e11d74); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.35), 0 4px 10px -4px rgb(225 29 116 / 0.55); }
	.grow { min-width: 0; }
	.top { display: flex; align-items: baseline; gap: 8px; min-width: 0; }
	.name { font: 600 15px/1.3 var(--font-ui); letter-spacing: -0.01em; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.kind { flex: none; color: var(--text-muted); font: 500 12px/1 var(--font-ui); }
	.where { margin-top: 3px; color: var(--text-muted); font: 400 13px/1.3 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	/* Quality: a compact name-and-value button in the facts on wide screens, a full settings row on phones. */
	/* A fixed width keeps the label lined up from row to row, whatever the path or set name. */
	.qpick { width: 232px; min-width: 0; }
	.qpick :global(.ui-pop) { display: flex; min-width: 0; }
	.qpick :global(.trigger.row), .qro { display: flex; align-items: center; gap: 6px; width: auto; max-width: 100%; min-width: 0; min-height: 36px; padding: 4px 8px; }
	.ql { flex: none; color: var(--text-muted); font: 500 12px/1.3 var(--font-ui); }
	.qv { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); font: 500 13px/1.3 var(--font-ui); }
	.own .qv { color: var(--text); }
	.qc { flex: none; display: inline-flex; color: var(--text-muted); transform: rotate(90deg); }
	@media (pointer: coarse) { .qpick :global(.trigger.row) { min-height: 44px; } }
	.why { margin-top: 3px; font: 500 12px/1.3 var(--font-ui); color: var(--fail-text, var(--fail)); }
	.why.wait { color: var(--wait-text, var(--wait)); }
	.free { display: flex; align-items: baseline; gap: 4px; justify-content: flex-end; white-space: nowrap; font-variant-numeric: tabular-nums; }
	.free b { font: 600 15px/1 var(--font-ui); }
	.free small { color: var(--text-muted); font: 400 12px/1 var(--font-ui); }
	.st { display: inline-flex; align-items: center; gap: 6px; color: var(--text-muted); font: 500 12px/1 var(--font-ui); white-space: nowrap; }
	.st i { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--ok); box-shadow: 0 0 0 3px color-mix(in srgb, var(--ok) 18%, transparent); }
	.st.wait i { background: var(--wait); box-shadow: 0 0 0 3px color-mix(in srgb, var(--wait) 22%, transparent); }
	.st.fail i { background: var(--fail); box-shadow: 0 0 0 3px color-mix(in srgb, var(--fail) 22%, transparent); }
	.st .why-link { flex: none; color: var(--accent, var(--text)); text-decoration: underline; text-underline-offset: 2px; }
	.acts { display: flex; align-items: center; gap: 2px; }
	.ed { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
	.ed .input { max-width: 240px; min-height: 36px; }
	/* New-title tags: a full-width line under the row, label left of the field. */
	.folder .nt { grid-column: 2 / -1; display: grid; grid-template-columns: 232px minmax(0, 1fr); align-items: start; column-gap: 16px; row-gap: 4px; min-width: 0; padding: 6px 0 0 0; }
	.nt .ql { padding: 10px 8px 0 8px; }
	.nt .nv { min-width: 0; }
	.only { display: flex; align-items: flex-start; gap: 8px; margin-top: 8px; font: 400 14px/1.4 var(--font-ui); color: var(--text); }
	.only input { margin-top: 3px; flex: none; }
	.only-hint { margin: 2px 0 0 24px; color: var(--text-muted); font: 400 12px/1.4 var(--font-ui); }
	.only-off { margin: 6px 0 0; color: var(--text-muted); font: 400 12px/1.4 var(--font-ui); }
	/* On a touch screen the checkbox row and the tag box are as tall as the Quality row. */
	@media (pointer: coarse) {
		.only { align-items: center; min-height: 44px; margin-top: 4px; }
		.only input { margin-top: 0; }
		.sc-form :global(.ti .input), .nt .nv :global(.input) { min-height: 44px; height: 44px; }
	}
	.nt .hint { margin: 2px 0 0; color: var(--text-muted); font: 400 12px/1.4 var(--font-ui); }
	/* The chips and the box share one wrapping line. */
	.nt .nv :global(.ti) { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
	.nt .nv :global(.chips) { display: contents; }
	.nt .nv :global(.input) { flex: 1 1 140px; min-width: 0; min-height: 36px; height: 36px; }
	/* Change path: a full-width line under the row. */
	.folder .mv { grid-column: 2 / -1; display: flex; flex-direction: column; gap: 6px; min-width: 0; padding-top: 6px; }
	.mv label { color: var(--text-muted); font: 500 12px/1.3 var(--font-ui); }
	.mv-row { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
	.mv-row .input { flex: 1 1 220px; min-width: 0; min-height: 36px; }
	.mv .hint { margin: 0; color: var(--text-muted); font: 400 12px/1.4 var(--font-ui); }
	.mv .form-error { margin: 0; overflow-wrap: anywhere; }
	/* Glass sheet shared by the folder rows. */
	.fl {
		border: var(--sheet-edge); border-radius: var(--sheet-r);
		background: var(--sheet-bg);
		
		box-shadow: var(--sheet-shadow);
		overflow: hidden;
	}

	/* Grouped: Movies, Shows and Music each get their own sheet, one line per folder. */
	.fl .folder { padding: 14px 16px 14px 18px; min-height: 72px; }
	.fl .folder + .folder { box-shadow: inset 0 1px 0 var(--line); }
	.fl .folder:hover { background: color-mix(in srgb, var(--text) 3%, transparent); }
	.fl-groups { display: flex; flex-direction: column; gap: 18px; }
	.fl-group h3 { display: flex; align-items: center; gap: 8px; margin: 0 0 8px 6px; color: var(--text-muted); font: 600 13px/1 var(--font-ui); letter-spacing: 0.02em; }
	.fl-group h3 .cnt { padding: 3px 8px; border-radius: var(--r-pill); background: var(--surface-2); font-size: 11px; font-variant-numeric: tabular-nums; }
	.kind { display: none; }

	@container settings (max-width: 640px) {
		.folder { grid-template-columns: 40px minmax(0, 1fr) auto; grid-template-areas: 'ic grow grow' 'ic st st' 'ic q q' 'ic free acts' 'ic nt nt'; row-gap: 6px; }
		.folder .nt { grid-area: nt; grid-template-columns: minmax(0, 1fr); }
		.nt .ql { padding: 0; font: 400 15px/1.3 var(--font-ui); color: var(--text); }
		.folder .qpick { grid-area: q; width: auto; border-block: 1px solid var(--line); }
		.qpick :global(.ui-pop) { width: 100%; }
		.qpick :global(.trigger.row), .qro { width: 100%; max-width: none; min-height: 44px; padding: 0; border-radius: 0; }
		.ql { flex: 1; color: var(--text); font: 400 15px/1.3 var(--font-ui); }
		.qv { font: 400 15px/1.3 var(--font-ui); }
		.qc { transform: none; }
		.folder .ic { grid-area: ic; align-self: start; } .folder .grow { grid-area: grow; } .folder .st { grid-area: st; justify-self: start; white-space: normal; line-height: 1.3; }
		.folder .free { grid-area: free; justify-content: flex-start; } .folder .acts { grid-area: acts; justify-self: end; opacity: 1 !important; }
		.folder .kind { display: none; }
		.folder .name { white-space: normal; }
	}

	/* The folder mark: a plain line icon, no tile. */
	.fl .folder { grid-template-columns: 24px minmax(0, 1fr) auto auto auto auto; }
	.ic, .k-show .ic, .k-music .ic { width: 24px; height: 24px; background: none; box-shadow: none; border-radius: 0; color: color-mix(in srgb, var(--text) 55%, transparent); }
	@container settings (max-width: 640px) { .fl .folder { grid-template-columns: 24px minmax(0, 1fr) auto; } }
	/* Phones: the buttons take their own line at the row's end (change path, then remove, last), so a long Rename label cannot squeeze the name. */
	/* The state takes its own line too, so a long state never squeezes the name. */
	@container settings (max-width: 440px) {
		.folder { grid-template-areas: 'ic grow grow' 'ic st st' 'ic free free' 'ic q q' 'ic acts acts' 'ic nt nt'; }
		.folder .st { justify-self: start; white-space: normal; line-height: 1.3; }
		.folder .acts { justify-self: end; flex-wrap: wrap; justify-content: flex-end; margin-inline-end: -8px; }
	}
</style>
