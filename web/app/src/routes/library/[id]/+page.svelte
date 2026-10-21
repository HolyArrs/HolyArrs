<script lang="ts">
	import { peopleNames } from '#lib/plain.ts';
	import { to } from '#lib/paths.ts';
	import { page } from '$app/state';
	import { api, message, ApiError } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { ago, bytes, date, dateTime, tone, words } from '#lib/format.ts';
	import Icon from '#lib/Icon.svelte';
	import { session } from '#lib/session.svelte.ts';
	import EmptyState from '#lib/EmptyState.svelte';
	import { secondVersion, slotNotes } from '#lib/library.ts';
	import { chooseWarnings, facts, keyReasons, pager, verdict, verdictTone } from '#lib/releases.ts';
	import { decided } from '#lib/wishlist.ts';
	import { downloadTone, percent, timeLeft } from '#lib/downloaders.ts';
	import RenamePreview from '#lib/RenamePreview.svelte';
	import SubtitlePanel from '#lib/SubtitlePanel.svelte';
	import ShowPage from '#lib/ShowPage.svelte';
	import CopyControls from '#lib/CopyControls.svelte';
	import ArtistPage from '#lib/ArtistPage.svelte';
	import { goto } from '$app/navigation';
	import { deleteSummary, type RemovePlan } from '#lib/organise.ts';
	import { groupLabel, kindTone } from '#lib/history.ts';
	import TitleHero, { richMetadata, runtimeWords, shortFolder, type RichMeta } from '#lib/ui/TitleHero.svelte';
	import { plainReason } from '#lib/plain.ts';
	import ActionButton from '#lib/ui/ActionButton.svelte';
	import AskedByLine from '#lib/ui/AskedByLine.svelte';
	import { loadAskedBy, type AskedByAnswer } from '#lib/askedby.ts';
	import { reducedMotion } from '#lib/motion.ts';
	import ActionMenu from '#lib/ui/ActionMenu.svelte';
	import Menu from '#lib/ui/Menu.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { SkTitle } from '#lib/ui/sk/index.ts';
	import StatusPill, { type StatusKind } from '#lib/ui/StatusPill.svelte';
	import Button from '#lib/ui/Button.svelte';
	import ProgressBar from '#lib/ui/ProgressBar.svelte';
	import TitleCast from '#lib/TitleCast.svelte';
	import TitleRows from '#lib/TitleRows.svelte';
	import LibrarySection from '#lib/ui/LibrarySection.svelte';
	import { artName, crumbFor, pageSections, qualityBadges } from '#lib/title-page.ts';
	import { audienceOf, factsOf, titleStatus } from '#lib/title-status.ts';
	import SectionNav from '#lib/ui/SectionNav.svelte';
	import TitleSection from '#lib/ui/TitleSection.svelte';
	import type { MenuItem } from '#lib/ui/Menu.svelte';
	import '#lib/title-page.css';
	import type { Choice, EventPage, EventRecord, LibraryFolder, PickRulesView, ReleasePage, ReleaseView, TitleView, VersionView } from '#lib/types.ts';

	const id = $derived(Number(page.params.id));
	const libraryBack = $derived.by(() => {
		const target = page.url.searchParams.get('returnTo');
		const library = to('/library');
		return target && (target === library || target.startsWith(`${library}?`)) ? target : library;
	});
	let title = $state<TitleView | null>(null);

	// Who asked, as the server lets this reader see it.
	let askedBy = $state<AskedByAnswer>({ titles: {}, sees_all: false });
	$effect(() => {
		const id = title?.id;
		if (!id) return;
		loadAskedBy(api.get, [id]).then((x) => (askedBy = x), () => {});
	});
	const askers = $derived(title ? (askedBy.titles[String(title.id)]?.askers ?? []) : []);
	let history = $state<EventRecord[]>([]);
	let historyTotal = $state(0);
	let releases = $state<ReleaseView[]>([]);
	let relTotal = $state(0);
	let relOffset = $state(0);
	const relLimit = 25;
	let running = $state<number[]>([]);
	/** The Slot whose Releases are shown; 0 shows every Version. */
	let slotPick = $state(0);
	const pages = $derived(pager(relTotal, relLimit, relOffset));
	let notFound = $state(false);
	let blocked = $state(false);
	let error = $state('');
	let searching = $state(false);
	let saving = $state(false);
	let refreshing = $state(false);

	// The edit dialog
	let editDialog: HTMLDialogElement | undefined = $state();
	let rules = $state<PickRulesView[]>([]);
	let eRules = $state(0);
	let eGate = $state('released');
	let eDelay = $state(0);
	let eKeep = $state(true);
	let editError = $state('');
	let editing = $state(false);
	// Versions are one row; See all lays every Version out.
	let allVersions = $state(false);
	const leadVersion = (vs: VersionView[]) => (vs.find((v) => v.main) ?? vs[0])?.id;
	const gates: [string, string][] = [
		['announced', 'As soon as it is announced'],
		['in_cinemas', 'Once it is in cinemas'],
		['released', 'Once it is out on digital or disc']
	];
	const stageWord: Record<string, string> = { announced: 'Announced', in_cinemas: 'In cinemas', released: 'Out on digital or disc' };
	const files = $derived(title ? title.slots.filter((s) => s.file) : []);

	// The add-a-Version dialog
	let versionDialog: HTMLDialogElement | undefined = $state();
	let folders = $state<LibraryFolder[]>([]);
	let vLabel = $state('4K');
	let vEdition = $state('');
	let vAudio = $state('');
	let vFolderName = $state('');
	let vRules = $state(0);
	let vFolder = $state(0);
	let vError = $state('');
	let vBusy = $state(false);
	const movieFolders = $derived(folders.filter((f) => f.kind === 'movie'));
	const slotOf = (v: VersionView) => title?.slots.find((s) => s.version_id === v.id);

	async function movieRules() {
		if (!rules.length) {
			rules = (await api.get<{ pick_rules: PickRulesView[] }>('/pick-rules')).pick_rules.filter((r) => !r.kinds.length || r.kinds.includes('movie'));
		}
	}

	async function openVersion() {
		if (!title) return;
		vError = '';
		try {
			await movieRules();
			if (!folders.length) folders = (await api.get<{ library_folders: LibraryFolder[] }>('/library-folders')).library_folders;
			const used = new Set(title.versions.map((v) => v.label.toLowerCase()));
			const sv = secondVersion(rules, movieFolders, title.library_folder_id ?? 0);
			vLabel = used.has(sv.label.toLowerCase()) ? '' : sv.label;
			vRules = sv.pick_rules_id;
			vEdition = ''; vAudio = ''; vFolderName = '';
			vFolder = sv.library_folder_id || movieFolders[0]?.id || 0;
		} catch (e) {
			vError = message(e);
		}
		// Open once the choices are filled in, so typing is never overwritten.
		versionDialog?.showModal();
	}

	async function addVersion(e: SubmitEvent) {
		e.preventDefault();
		vBusy = true;
		vError = '';
		try {
			await api.post(`/titles/${id}/versions`, { label: vLabel.trim(), pick_rules_id: vRules, library_folder_id: vFolder, edition: vEdition.trim(), audio_languages: vAudio.split(/[,\s]+/).filter(Boolean), folder_name: vFolderName });
			versionDialog?.close();
			toasts.push(`The ${vLabel.trim()} Version is added.`, 'ok');
			allVersions = true;
			await load('title');
		} catch (err) {
			vError = message(err);
		} finally {
			vBusy = false;
		}
	}

	async function removeVersion(v: VersionView) {
		if (!confirm(`Stop keeping the ${v.label} Version of this movie? Files on disk are not touched.`)) return;
		try {
			await api.del(`/titles/${id}/versions/${v.id}`);
			toasts.push(`The ${v.label} Version is removed.`, 'ok');
			await load('title');
		} catch (e) {
			toasts.push(message(e), 'fail');
		}
	}

	// Choose a release: the confirm dialog and the turn-down dialog.
	let chooseDialog: HTMLDialogElement | undefined = $state();
	let turnDialog: HTMLDialogElement | undefined = $state();
	let target = $state<ReleaseView | null>(null);
	let forceReasons = $state<string[]>([]);
	let chooseError = $state('');
	let choosing = $state(false);
	let turnReason = $state('');
	// The Slot a choice goes to: the picked Version, the one the Release
	// matched, or the only one the movie has.
	const targetSlotId = $derived(slotPick || target?.slot_id || (title?.slots.length === 1 ? title.slots[0].id : 0));
	const targetSlot = $derived(title?.slots.find((s) => s.id === targetSlotId));
	const warnings = $derived(target ? chooseWarnings(target, targetSlot?.state) : []);
	const needsForce = $derived(!!target && (target.needs_force || forceReasons.length > 0));

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
			if (targetSlotId) body.slot_id = targetSlotId;
			const ch = await api.post<Choice>(`/releases/${target.id}/choose`, body);
			chooseDialog?.close();
			toasts.push(`Chosen: ${target.name}. HolyArrs sends it to a Downloader now.`, 'ok');
			for (const w of ch.warnings) toasts.push(w, 'info');
			await load();
		} catch (e) {
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
			await api.post(`/releases/${target.id}/turn-down`, turnReason.trim() ? { reason: turnReason.trim() } : undefined);
			turnDialog?.close();
			toasts.push(`Turned down: ${target.name}. HolyArrs never picks it again for this movie.`, 'ok');
			await load('releases');
		} catch (err) {
			chooseError = message(err);
		} finally {
			choosing = false;
		}
	}

	function pickSlot(v: number) {
		slotPick = v;
		relOffset = 0;
		void load('releases');
	}

	function goPage(off: number | null) {
		if (off === null) return;
		relOffset = off;
		void load('releases');
	}

	async function load(which: 'all' | 'title' | 'releases' = 'all') {
		try {
			if (which !== 'releases') title = await api.get<TitleView>(`/titles/${id}`);
			// A show's or artist's page reads its own parts and history.
			if (title?.kind === 'show' || title?.kind === 'artist') {
				error = '';
				return;
			}
			if (which !== 'releases' && session.can('manage_library')) {
				const h = await api.get<EventPage>(`/events?title=${id}&limit=8`);
				history = h.events;
				historyTotal = h.total;
			}
			if (which !== 'title' && session.can('manage_library')) {
				const q = `limit=${relLimit}&offset=${relOffset}${slotPick ? `&slot_id=${slotPick}` : ''}`;
				const p = await api.get<ReleasePage>(`/titles/${id}/releases?${q}`);
				releases = p.releases;
				relTotal = p.total;
				running = p.running_job_ids ?? [];
			}
			error = '';
		} catch (e) {
			// A Kid gets 404 for a Library title above their age rating, the same as a missing one.
			if (e instanceof ApiError && e.status === 404) {
				if (session.kid) blocked = true;
				else notFound = true;
			} else error = message(e);
		}
	}

	$effect(() => {
		if (!Number.isFinite(id)) return;
		void load();
		const offs = [
			live.on('title', () => load('title')),
			live.on('slot', () => load('title')),
			live.on('download', (d) => {
				const dl = d as { ID?: number; id?: number };
				const dlID = dl?.ID ?? dl?.id;
				if (dlID && title?.slots.some((s) => s.active_download_id === dlID)) void load('title');
			}),
			live.on('releases', () => load()),
			live.on('event', (d) => {
				const e = d as { TitleID?: number | null };
				if (e?.TitleID === id) void load('title');
			}),
			live.on('job', (d) => {
				const j = d as { id?: number; state?: string };
				if (j?.id && running.includes(j.id) && j.state !== 'running' && j.state !== 'queued') void load('releases');
			}),
			live.on('resync', () => load())
		];
		return () => offs.forEach((off) => off());
	});

	// Rename and delete (5.1.4, 5.1.6).
	let renameDialog: HTMLDialogElement | undefined = $state();
	let renameOpen = $state(false);
	let deleteDialog: HTMLDialogElement | undefined = $state();
	let delFiles = $state(false);
	let delNever = $state(false);
	let delPlan = $state<RemovePlan | null>(null);
	let delError = $state('');
	let delBusy = $state(false);
	let delAsk = 0;

	function openRename() {
		renameOpen = true;
		renameDialog?.showModal();
	}

	async function planDelete() {
		const ask = ++delAsk;
		delError = '';
		try {
			const p = await api.post<RemovePlan>(`/titles/${id}/delete`, { delete_files: delFiles, never_add: delNever, apply: false });
			if (ask === delAsk) delPlan = p;
		} catch (e) {
			if (ask === delAsk) delError = message(e);
		}
	}

	function openDelete() {
		delFiles = false;
		delNever = false;
		delPlan = null;
		deleteDialog?.showModal();
		void planDelete();
	}

	async function doDelete() {
		delBusy = true;
		delError = '';
		try {
			const p = await api.post<RemovePlan>(`/titles/${id}/delete`, { delete_files: delFiles, never_add: delNever, apply: true });
			deleteDialog?.close();
			toasts.push(
				p.held.length ? `${p.name} is deleted. Its files are in the holding folder${p.holding_days ? ` for ${p.holding_days} days` : ''}.` : `${p.name} is deleted. Its files stay on disk.`,
				'ok'
			);
			await goto(to('/library'));
		} catch (e) {
			delError = message(e);
			if (e instanceof ApiError && e.body.plan) delPlan = e.body.plan as RemovePlan;
		} finally {
			delBusy = false;
		}
	}

	let failing = $state(0);
	async function markFailed(downloadID: number) {
		if (!confirm('Mark this download failed? Its Release is turned down, it is removed from the Downloader and HolyArrs looks for the next Release.')) return;
		failing = downloadID;
		try {
			await api.post(`/downloads/${downloadID}/mark-failed`);
			toasts.push('Marked failed. HolyArrs looks for the next Release.', 'ok');
			await load('title');
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			failing = 0;
		}
	}

	async function searchNow(pick = true) {
		searching = true;
		try {
			const body: Record<string, unknown> = {};
			if (!pick) body.choose = false;
			if (slotPick) body.slot_id = slotPick;
			const r = await api.post<{ job_ids: number[] }>(`/titles/${id}/search`, body);
			running = [...new Set([...running, ...r.job_ids])];
			relOffset = 0;
			toasts.push(
				pick ? 'Search started. HolyArrs picks the best Release by itself.' : 'Searching every Source. Pick one from the list once they answer.',
				'ok'
			);
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			setTimeout(() => (searching = false), 1500);
		}
	}

	// More › Choose a release: bring the Releases section into view and start
	// the same pick-it-yourself search as its own button.
	function chooseRelease() {
		const h = document.getElementById('h-rel');
		h?.closest('section')?.scrollIntoView({ behavior: reducedMotion() ? 'auto' : 'smooth', block: 'start' });
		h?.focus({ preventScroll: true });
		void searchNow(false);
	}

	/** dlKind maps a download state to the pill that says it. */
	function dlKind(state: string, active: boolean): StatusKind {
		if (active) return 'downloading';
		const t = downloadTone(state);
		return t === 'ok' ? 'in_library' : t === 'fail' ? 'failed' : t === 'idle' ? 'requested' : 'missing';
	}

	async function refresh() {
		refreshing = true;
		try {
			await api.post(`/titles/${id}/refresh`);
			toasts.push('Refreshing names, dates and posters from TMDB.', 'ok');
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			setTimeout(() => (refreshing = false), 1500);
		}
	}

	async function openEdit() {
		if (!title) return;
		editError = '';
		eRules = title.pick_rules_id ?? 0;
		eGate = title.release_gate;
		eDelay = title.gate_delay_days;
		eKeep = title.keep_up_to_date;
		editing = true;
		editDialog?.showModal();
		if (!rules.length) {
			try {
				rules = (await api.get<{ pick_rules: PickRulesView[] }>('/pick-rules')).pick_rules.filter((r) => !r.kinds.length || r.kinds.includes('movie'));
			} catch (e) {
				editError = message(e);
			}
		}
	}

	async function saveEdit(e: SubmitEvent) {
		e.preventDefault();
		saving = true;
		editError = '';
		try {
			const body: Record<string, unknown> = { release_gate: eGate, gate_delay_days: Number(eDelay) || 0, keep_up_to_date: eKeep };
			if (eRules) body.pick_rules_id = eRules;
			const r = await api.put<{ title: TitleView }>(`/titles/${id}`, body);
			title = r.title;
			editDialog?.close();
			toasts.push('Saved.', 'ok');
		} catch (err) {
			editError = message(err);
		} finally {
			saving = false;
		}
	}

	// The hero: breadcrumb, meta line, badges from the kept files, status and menus.
	const crumbs = $derived(
		title ? [{ href: to(crumbFor('movie').path), label: crumbFor('movie').label }, { href: to(`/library/${title.id}`), label: title.year ? `${title.name} (${title.year})` : title.name }] : []
	);
	// Richer art and facts from the metadata endpoint, when the backend has it.
	let rich = $state<RichMeta | null>(null);
	$effect(() => {
		const tmdb = title?.ids?.tmdb;
		rich = null;
		if (!tmdb || (title?.backdrop && title?.genres?.length)) return;
		let live = true;
		void richMetadata(api.get, 'movie', tmdb).then((m) => live && (rich = m));
		return () => (live = false);
	});
	const heroMeta = $derived(title ? [title.year ? String(title.year) : '', runtimeWords(title.runtime_minutes ?? rich?.runtime), ...(title.genres?.length ? title.genres : (rich?.genres ?? [])).slice(0, 3)].filter(Boolean) : []);
	const notes = $derived(title ? slotNotes(title.slots).map((n) => plainReason(n)) : []);
	const heroBadges = $derived(qualityBadges(files.map((s) => s.file)));
	// One status vocabulary (title-status.ts): staff or member words.
	const status = $derived(title ? titleStatus({ ...factsOf(title), titleKind: 'movie' }, audienceOf(session.can)) : { kind: 'missing' as const, label: 'Missing' });
	const manage = $derived(session.can('manage_library'));
	// Manager actions live in the hero's ⋯ menu; the Library section holds the rest.
	const heroItems = $derived<MenuItem[]>([
		{ label: 'Search now', icon: 'search', onselect: () => searchNow(), disabled: searching },
		{ label: 'Choose a release', icon: 'grid', onselect: chooseRelease, disabled: searching },
		{ label: 'Keep up to date', checked: !!title?.keep_up_to_date, disabled: saving, onselect: () => title && setKeep(!title.keep_up_to_date) },
		{ kind: 'separator' },
		{ label: 'Edit', icon: 'tools', onselect: openEdit },
		{ label: 'Rename files', icon: 'copy', onselect: openRename },
		{ label: 'Add a Version', icon: 'plus', onselect: openVersion },
		{ label: 'Refresh from TMDB', icon: 'refresh', onselect: refresh, disabled: refreshing },
		{ label: 'All history', icon: 'clock', href: to(`/downloads/history?title=${id}`) },
		{ kind: 'separator' },
		{ label: 'Delete movie', icon: 'trash', destructive: true, onselect: openDelete }
	]);
	function toLibrary() {
		const h = document.getElementById('h-library');
		h?.closest('section')?.scrollIntoView({ behavior: reducedMotion() ? 'auto' : 'smooth', block: 'start' });
	}

	async function setKeep(v: boolean) {
		if (!title) return;
		saving = true;
		try {
			const r = await api.put<{ title: TitleView }>(`/titles/${id}`, { keep_up_to_date: v });
			title = r.title;
			toasts.push(v ? 'HolyArrs keeps this up to date.' : 'HolyArrs stops looking for this.', 'ok');
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head><title>{title?.name ?? 'Title'} · HolyArrs</title></svelte:head>

{#if blocked}
	<EmptyState icon="shield" title="This title is not available on this account.">
		{#snippet actions()}<Button href={libraryBack}>Back to the Library</Button>{/snippet}
	</EmptyState>
{:else if notFound}
	<EmptyState icon="library" title="This title is not in your Library">
		<p>It may have been removed.</p>
		{#snippet actions()}<Button href={libraryBack}>Back to the Library</Button>{/snippet}
	</EmptyState>
{:else if error && !title}
	<LoadError hasData={false} message={error} onretry={() => load()} />
{:else if !title}
	<SkTitle />
{:else if title.kind === 'show'}
	<ShowPage {title} onchange={(t) => (title = t)} {error} onretry={() => load('title')} />
{:else if title.kind === 'artist'}
	<ArtistPage {title} onchange={(t) => (title = t)} {error} onretry={() => load('title')} />
{:else}
	<div class="tp">
		<TitleHero
			title={title.name}
			kind="movie"
			tmdb={title.ids?.tmdb}
			poster={title.artwork[0] ?? rich?.poster}
			backdrop={title.backdrop ?? rich?.backdrop}
			backdrops={rich?.backdrops}
			logo={title.logo ?? rich?.logo}
			breadcrumb={crumbs}
			meta={heroMeta}
			certification={title.rating ?? rich?.certification}
			score={rich?.score}
			badges={heroBadges}
			status={status.kind}
			statusLabel={status.label}
			description={title.overview || rich?.overview}
			vtName={artName(title.id)}
		>
			{#snippet byline()}<AskedByLine {askers} seesAll={askedBy.sees_all} />{/snippet}
			{#snippet actions()}
				{#if manage}
					<ActionButton icon="library" label="In Library: go to the Library section" caption="In Library" primary onclick={toLibrary} data-testid="hero-library" />
					<ActionMenu label="More actions" caption="More" icon="more" items={heroItems} />
				{/if}
			{/snippet}
		</TitleHero>

		{#if title.slots.some((s) => s.download)}
			<div class="tp-dls">
				{#each title.slots.filter((s) => s.download) as s (s.id)}
					{@const d = s.download!}
					<div class="tp-dl" data-testid="download">
						<StatusPill kind={dlKind(d.state, d.active)} testid="download-state" label="{s.version && title!.slots.length > 1 ? `${s.version}: ` : ''}{d.label}" />
						{#if d.active}<div class="tp-bar"><ProgressBar value={d.progress * 100} label="Download progress" /></div>
							<span class="small muted">{percent(d.progress)}{#if d.speed > 0} · {bytes(d.speed)}/s{/if}{#if timeLeft(d.eta)} · {timeLeft(d.eta)}{/if}</span>{/if}
						<p class="tp-dl-text" data-testid="download-text">{d.text}</p>
						{#if d.active && manage}
							<Button class="tp-btn" size="sm" variant="plain" onclick={() => markFailed(d.id)} busy={failing === d.id}>Mark failed</Button>
						{/if}
					</div>
				{/each}
			</div>
		{/if}

		{#if error}<LoadError hasData message={error} onretry={() => load()} />{/if}

		<SectionNav items={pageSections('movie', { manage })} />

		<TitleRows kind="movie" tmdb={title.ids?.tmdb}>
			{#snippet middle()}{#key title!.id}<TitleCast titleId={title!.id} />{/key}{/snippet}
		</TitleRows>

		<TitleSection id="h-about" title="About">
			<dl class="tp-about">
				<dt>Now</dt>
				<dd>{stageWord[title.stage] ?? words(title.stage)}</dd>
				{#if title.year}<dt>Released</dt><dd>{title.year}</dd>{/if}
				{#if title.cinema_date}<dt>In cinemas</dt><dd>{date(title.cinema_date)}</dd>{/if}
				{#if title.digital_date}<dt>Digital</dt><dd>{date(title.digital_date)}</dd>{/if}
				{#if title.physical_date}<dt>Disc</dt><dd>{date(title.physical_date)}</dd>{/if}
				{#if title.runtime_minutes ?? rich?.runtime}<dt>Runtime</dt><dd>{runtimeWords(title.runtime_minutes ?? rich?.runtime)}</dd>{/if}
				{#if title.rating ?? rich?.certification}<dt>Rated</dt><dd>{title.rating ?? rich?.certification}</dd>{/if}
				<dt>Original language</dt>
				<dd>{title.original_language ? title.original_language.toUpperCase() : 'Not known'}</dd>
				{#if title.genres?.length}<dt>Genres</dt><dd>{title.genres.join(', ')}</dd>{/if}
			</dl>
		</TitleSection>

		{#if title.asks?.length}
			<TitleSection id="h-asks" title="Wishlist" testid="title-asks">
				{#each title.asks as a (a.id)}
					<dl class="facts">
						<dt>Asked by</dt>
						<dd>{peopleNames(a.people.map((p) => p.name))} · {date(a.asked_at)}</dd>
						{#if decided(a)}<dt>Decided</dt>
							<dd>{decided(a)}{a.decided_at ? ` · ${date(a.decided_at)}` : ''}</dd>{/if}
						<dt>Now</dt>
						<dd>{a.state_label}</dd>
						{#if a.note}<dt>Note</dt>
							<dd>{a.note}</dd>{/if}
					</dl>
				{/each}
			</TitleSection>
		{/if}

		{#if manage}
			<LibrarySection>
				<TitleSection id="h-versions" title="Versions and files" primary>
					{#snippet head()}
						<div class="tp-head-end">
							{#if title!.versions.length > 1}
								<Button variant="plain" aria-expanded={allVersions} aria-controls="movie-versions" onclick={() => (allVersions = !allVersions)}>{allVersions ? 'Show less' : `See all ${title!.versions.length}`}</Button>
							{/if}
							<Button icon="search" onclick={() => searchNow()} disabled={searching} busy={searching} data-testid="search-now">Search now</Button>
							<Button icon="plus" onclick={openVersion}>Add a Version</Button>
						</div>
					{/snippet}
					<ul class="tp-list tp-vrow" id="movie-versions" aria-label="Versions">
						{#each title.versions as v (v.id)}
							{@const s = slotOf(v)}
							{@const f = s?.file}
							<li class="tp-row" data-testid="version" hidden={!allVersions && v.id !== leadVersion(title.versions)}>
								<div class="tp-row-main">
									<b>{v.label}{v.main ? ' (main)' : ''}</b>
									<span class="xs muted">Edition: {v.edition || 'Not specified'} · Audio required: {v.audio_languages?.join(', ') || 'Any'}</span>
									<span class="xs muted">Pick rules: {v.pick_rules_name}{v.keep_up_to_date ? '' : ' · not kept up to date'}</span>
									{#if f}
										<span class="xs muted">{bytes(f.Size)}{#if f.Quality?.resolution} · {f.Quality.resolution}p{f.Quality.source ? ` ${words(String(f.Quality.source))}` : ''}{/if} · Added {date(f.AddedAt)}</span>
									{:else}
										<span class="xs muted">No file yet.</span>
									{/if}
									{#if f || v.folder_path}
										<details class="tp-details">
											<summary>Details</summary>
											<ul>
												{#if f}<li>File: {shortFolder(f.RelativePath)}</li>{/if}
												{#if f?.ReleaseName}<li>From the release {f.ReleaseName}</li>{/if}
												{#if v.folder_path}<li>Folder: {shortFolder(v.folder_path)}</li>{/if}
											</ul>
										</details>
									{/if}
								</div>
								<div class="tp-row-end">
									{#if s}<span class="chip {tone(s.state)}">{s.state_label}</span>{/if}
									{#if !v.main}
										<Menu label="Version actions for {v.label}" items={[{ label: `Remove ${v.label}`, icon: 'trash', destructive: true, onselect: () => removeVersion(v) }]} width={220} />
									{/if}
								</div>
							</li>
						{/each}
					</ul>
					{#if notes.length}
						<details class="tp-details" data-testid="slot-notes">
							<summary>Details</summary>
							<ul>{#each notes as n (n.text + (n.detail ?? ''))}<li>{n.text}</li>{/each}</ul>
						</details>
					{/if}
				</TitleSection>

				<CopyControls {title} onchange={() => load('title')} />

				<TitleSection id="h-rel" title="Releases" testid="releases" focusable>
					{#snippet head()}
						{#if relTotal}<span class="xs muted">{relTotal} found in the last search</span>{/if}
					{/snippet}
					{#if title.slots.length > 1}
						<div class="field slot-pick">
							<label for="slot-pick">Version</label>
							<select id="slot-pick" class="input" value={slotPick} onchange={(e) => pickSlot(Number(e.currentTarget.value))}>
								<option value={0}>Every Version</option>
								{#each title.slots as s (s.id)}<option value={s.id}>{s.version || 'Main'}</option>{/each}
							</select>
							<span class="hint">Search now and Choose a release then look for this Version only.</span>
						</div>
					{/if}
					<p class="small muted" aria-live="polite" data-testid="search-running">
						{running.length ? 'Searching your Sources. The list fills in once they answer.' : ''}
					</p>
					{#if releases.length === 0}
						{#if !running.length}
							<p class="small muted">No search yet. Choose a release shows every Release HolyArrs finds, with why each one would be chosen or not, so you can pick one yourself.</p>
							<Button class="tp-start tp-btn" icon="grid" onclick={() => searchNow(false)} disabled={searching}>Choose a release</Button>
						{/if}
					{:else}
						<ul class="rels">
							{#each releases as r (r.id)}
								{@const key = keyReasons(r.reasons)}
								<li class="rel" data-testid="release">
									<div class="rel-top">
										<span class="rel-name mono">{r.name}</span>
										<span class="chip {verdictTone(r)}" data-testid="verdict">{verdict(r)}</span>
									</div>
									<div class="xs muted">{facts(r, bytes)}{title.slots.length > 1 && r.version ? ` · ${r.version}` : ''}</div>
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
								<Button variant="plain" disabled={pages.prev === null} onclick={() => goPage(pages.prev)}>Back</Button>
								<span class="xs muted">{pages.from} to {pages.to} of {relTotal}</span>
								<Button variant="plain" disabled={pages.next === null} onclick={() => goPage(pages.next)}>More</Button>
							</nav>
						{/if}
					{/if}
				</TitleSection>

				<div id="title-subtitles">{#key title.id}<SubtitlePanel titleId={title.id} movie />{/key}</div>

				<TitleSection id="h-dates" title="When HolyArrs looks">
					<dl class="tp-about">
						<dt>Release gate</dt>
						<dd>{words(title.release_gate)}{title.gate_delay_days ? ` + ${title.gate_delay_days} days` : ''}</dd>
						{#if title.gate_opens_at}<dt>Gate opens</dt>
							<dd>{date(title.gate_opens_at)}</dd>{/if}
						{#if title.pick_rules_name}<dt>Pick rules</dt>
							<dd>{title.pick_rules_name}</dd>{/if}
						<dt>Keep up to date</dt>
						<dd>{title.keep_up_to_date ? 'On' : 'Off'}</dd>
						{#if title.refreshed_at}<dt>Refreshed</dt>
							<dd>{date(title.refreshed_at)}</dd>{/if}
					</dl>
					<p class="tp-why" data-testid="gate-reason">{title.gate_reason}</p>
				</TitleSection>

				<TitleSection id="h-history" title="History" testid="title-history">
					{#snippet head()}
						{#if historyTotal > 0}<Button class="tp-btn" variant="plain" size="sm" href={to(`/downloads/history?title=${id}`)}>All {historyTotal} rows</Button>{/if}
					{/snippet}
					{#if history.length === 0}
						<p class="small muted">Nothing yet. Every search, download, import, rename and delete of this movie writes a row here.</p>
					{:else}
						<ol class="rows hist">
							{#each history as e (e.ID)}
								<li>
									<span class="chip {kindTone(e.Kind)}">{groupLabel(e.Kind)}</span>
									<span class="grow small">{e.Summary}</span>
									<time class="mono xs muted" datetime={e.At} title={dateTime(e.At)}>{ago(e.At)}</time>
								</li>
							{/each}
						</ol>
					{/if}
				</TitleSection>
			</LibrarySection>
		{/if}
	</div>
{/if}

<dialog bind:this={chooseDialog} aria-labelledby="choose-title" class="modal">
	{#if target}
		<h2 id="choose-title" class="h2">{needsForce ? 'Choose it anyway?' : 'Choose this Release?'}</h2>
		<p class="mono small path" style="margin-top: var(--s-3)">{target.name}</p>
		<p class="small muted">{facts(target, bytes)}</p>
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

<dialog bind:this={turnDialog} aria-labelledby="turn-title" class="modal">
	{#if target}
		<form onsubmit={turnDown}>
			<h2 id="turn-title" class="h2">Turn down this Release?</h2>
			<p class="mono small path" style="margin-top: var(--s-3)">{target.name}</p>
			<p class="small muted">HolyArrs never picks it again for this movie. You can take it off the Turned down list later.</p>
			<div class="field" style="margin-top: var(--s-4)">
				<label for="turn-reason">Why (optional)</label>
				<input id="turn-reason" class="input" bind:value={turnReason} maxlength="500" />
			</div>
			{#if chooseError}<p class="form-error" role="alert">{chooseError}</p>{/if}
			<div class="row modal-actions">
				<Button variant="plain" onclick={() => turnDialog?.close()}>Cancel</Button>
				<Button variant="primary" type="submit" disabled={choosing} busy={choosing}>Turn down</Button>
			</div>
		</form>
	{/if}
</dialog>

<dialog bind:this={editDialog} aria-labelledby="edit-title" class="modal" onclose={() => (editing = false)}>
	{#if title && editing}
		<form onsubmit={saveEdit}>
			<h2 id="edit-title" class="h2">Edit {title.name}</h2>
			<div class="field" style="margin-top: var(--s-5)">
				<label for="e-rules">Pick rules{title.versions.length > 1 ? ' (main Version)' : ''}</label>
				<select id="e-rules" class="input" bind:value={eRules}>
					{#each rules as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
				</select>
			</div>
			<div class="field">
				<label for="e-gate">Release gate</label>
				<select id="e-gate" class="input" bind:value={eGate}>
					{#each gates as [g, label] (g)}<option value={g}>{label}</option>{/each}
				</select>
			</div>
			<div class="field">
				<label for="e-delay">Wait extra days</label>
				<input id="e-delay" class="input" type="number" min="0" max="365" bind:value={eDelay} />
				<span class="hint">Days to wait after the gate date before looking.</span>
			</div>
			<label class="checkline">
				<input type="checkbox" bind:checked={eKeep} />
				<span><b>Keep up to date</b><br /><span class="hint">HolyArrs looks for it and for a better copy later.</span></span>
			</label>
			{#if editError}<p class="form-error" role="alert">{editError}</p>{/if}
			<div class="row modal-actions">
				<Button variant="plain" onclick={() => editDialog?.close()}>Cancel</Button>
				<Button variant="primary" type="submit" disabled={saving} busy={saving}>Save</Button>
			</div>
		</form>
	{/if}
</dialog>

<dialog bind:this={renameDialog} aria-labelledby="rename-title" class="modal wide" onclose={() => (renameOpen = false)}>
	{#if title && renameOpen}
		<h2 id="rename-title" class="h2">Rename the files of {title.name}</h2>
		<p class="small muted">The names now and the names they get from the naming pattern. Nothing changes until you press Rename.</p>
		<RenamePreview titleId={id} onDone={() => load('title')} />
		<div class="row modal-actions">
			<Button variant="plain" onclick={() => renameDialog?.close()}>Close</Button>
		</div>
	{/if}
</dialog>

<dialog bind:this={deleteDialog} aria-labelledby="delete-title" class="modal wide">
	{#if title}
		<h2 id="delete-title" class="h2">Delete {title.name}?</h2>
		<p class="small muted">Read what will happen below before you press Delete. Nothing has changed yet.</p>
		<label class="checkline">
			<input type="checkbox" bind:checked={delFiles} onchange={planDelete} />
			<span
				><b>Also delete its files</b><br /><span class="hint"
					>Files HolyArrs imported or adopted move to the holding folder in their Library folder, and are removed for good after the holding days (7 unless you
					changed it in Settings). Files it only found are never touched. Off: every file stays where it is.</span
				></span
			>
		</label>
		<label class="checkline">
			<input type="checkbox" bind:checked={delNever} onchange={planDelete} />
			<span
				><b>Add it to the Never-add list</b><br /><span class="hint">Outside lists can then never add it again. You can still add it yourself.</span></span
			>
		</label>
		<section class="plan" aria-labelledby="plan-title" aria-live="polite" data-testid="delete-plan">
			<h3 id="plan-title" class="h3">What will happen</h3>
			{#if !delPlan}
				<div class="sk" style="height: 60px"></div>
			{:else}
				<ul class="warns">
					{#each deleteSummary(delPlan) as line (line)}<li>{line}</li>{/each}
				</ul>
				{#if delPlan.held.length}
					<p class="xs muted">To the holding folder:</p>
					<ul class="files">
						{#each delPlan.held as f (f.file_id)}<li class="mono xs path">{f.path}</li>{/each}
					</ul>
				{/if}
				{#if delPlan.kept.length}
					<p class="xs muted">Staying where they are:</p>
					<ul class="files">
						{#each delPlan.kept as f (f.file_id)}<li class="xs"><span class="mono path">{f.path}</span> <span class="muted">({f.why})</span></li>{/each}
					</ul>
				{/if}
			{/if}
		</section>
		{#if delError}<p class="form-error" role="alert">{delError}</p>{/if}
		<div class="row modal-actions">
			<Button variant="plain" onclick={() => deleteDialog?.close()}>Cancel</Button>
			<Button variant="destructive" onclick={doDelete} disabled={delBusy || !delPlan} busy={delBusy}>Delete movie</Button>
		</div>
	{/if}
</dialog>

<dialog bind:this={versionDialog} aria-labelledby="version-title" class="modal">
	{#if title}
		<form onsubmit={addVersion}>
			<h2 id="version-title" class="h2">Add a Version of {title.name}</h2>
			<p class="small muted">Another copy to keep next to the others, for example 4K, with its own Pick rules and folder.</p>
			<div class="field" style="margin-top: var(--s-5)">
				<label for="v-label">Version name</label>
				<input id="v-label" class="input" bind:value={vLabel} maxlength="40" required />
			</div>
			<div class="field">
                <label for="v-edition">Movie edition</label>
                <input id="v-edition" class="input" bind:value={vEdition} maxlength="40" placeholder="For example, Theatrical or Directors Cut" />
                <span class="hint">Leave empty for the existing quality-only choice. An unknown edition needs review before it can fill an edition choice.</span>
            </div>
            <div class="field">
                <label for="v-audio">Required audio languages</label>
                <input id="v-audio" class="input" bind:value={vAudio} placeholder="For example, en, ja" />
                <span class="hint">Every language must be present in inspected audio tracks. Subtitles are separate.</span>
            </div>
            <div class="field">
                <label for="v-existing-folder">Existing movie folder (optional)</label>
                <input id="v-existing-folder" class="input" bind:value={vFolderName} maxlength="200" />
                <span class="hint">Use one folder name inside the chosen Library folder when adopting an existing copy. Leave empty for a new folder.</span>
            </div>
            <div class="field">
                <label for="v-rules">Pick rules for this Version</label>
				<select id="v-rules" class="input" bind:value={vRules}>
					{#each rules as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
				</select>
			</div>
			<div class="field">
				<label for="v-folder">Library folder for this Version</label>
				<select id="v-folder" class="input" bind:value={vFolder}>
					{#each movieFolders as f (f.id)}<option value={f.id}>{f.label || f.path} ({f.path})</option>{/each}
				</select>
			</div>
			{#if vError}<p class="form-error" role="alert">{vError}</p>{/if}
			<div class="row modal-actions">
				<Button variant="plain" onclick={() => versionDialog?.close()}>Cancel</Button>
				<Button variant="primary" type="submit" disabled={vBusy || !vLabel.trim()} busy={vBusy}>Add Version</Button>
			</div>
		</form>
	{/if}
</dialog>

<style>
	.path { overflow-wrap: anywhere; }
	.modal { width: min(480px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow: auto; padding: var(--s-6); border: 0; border-radius: var(--r-modal); background: var(--surface-1); color: var(--text); box-shadow: var(--shadow-modal); }
	.modal.wide { width: min(640px, calc(100vw - 32px)); }
	.modal::backdrop { background: var(--scrim); }
	.modal-actions { justify-content: flex-end; margin-top: var(--s-6); }
	.plan { margin-top: var(--s-5); }
	.files { margin: var(--s-1) 0 var(--s-2); padding-left: var(--s-5); }
	.rel { padding: var(--s-4) 0; }
	.rel + .rel { border-top: 1px solid var(--line); }
	.rel-top { display: flex; justify-content: space-between; gap: var(--s-3); align-items: flex-start; margin-bottom: 4px; }
	.rel-name { font-size: var(--text-sm); overflow-wrap: anywhere; min-width: 0; }
	.reasons { margin-top: var(--s-2); font-size: var(--text-xs); color: var(--text-muted); }
	.reasons li { padding: 2px 0 2px var(--s-3); border-left: 2px solid var(--line-strong); }
	.reasons li.fail { border-left-color: var(--fail); }
	.reasons li.hold, .reasons li.warn { border-left-color: var(--wait); }
	.rel-actions { margin-top: var(--s-2); gap: var(--s-2); }
	.pager { justify-content: space-between; margin-top: var(--s-3); }
	.slot-pick { max-width: 280px; }
	.warns { margin-top: var(--s-4); padding: var(--s-3); border-left: 3px solid var(--wait); background: var(--surface-2, var(--surface-1)); border-radius: var(--r-sm); font-size: var(--text-sm); }
	.warns li + li { margin-top: var(--s-2); }
	details { margin-top: var(--s-2); }
	summary { cursor: pointer; min-height: 28px; display: inline-flex; align-items: center; }
	.hist li { flex-wrap: wrap; gap: var(--s-2) var(--s-3); }
	.hist .grow { min-width: 0; overflow-wrap: anywhere; }
</style>
