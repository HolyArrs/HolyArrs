<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { createMovePassword } from '#lib/migrate/index.ts';
	import { createMovePlan } from '#lib/migrate/index.ts';
	import { createMoveRun } from '#lib/migrate/index.ts';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import AdminLinkButton from '#lib/ui/AdminLinkButton.svelte';
	import { MoveSteps, StepFoot, ReadSheet, type ReadAnswer, type SavedKey, ItemsSheet, type SheetRow, type ShowList, ReadStep, ChooseStep, BeforeSheet, MoveStep, DoneStep, createFollower, appName, asStep, latestPerInstance, buckets, countWords, extraProblems, folderProblem, moveTotal, plural, rulesFrom, running, stepGate, type AppKind, type Line, type Move, type PathRule, type PersonChoice, type Plan, type Report, type Snapshot, type Step } from '#lib/migrate/index.ts';
	import { onDestroy, onMount, tick, untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { session } from '#lib/core/session.svelte.ts';
	import { api, ApiError } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { to } from '#lib/core/paths.ts';

	// Settings > Move from other apps: four steps, one at a time. The step and the reads in use live in the address (?step=choose&read=3,5, or ?move=7 for one move), so Back and a reload land on the same step. Move on Choose runs a quiet test first; only problems open "Before you move".
	type MoveAnswer = { move: Move; progress: number; progress_note: string };

	// The first-run wizard shows this same flow inside its panel. There the address keeps the wizard's own query (query) and the step goes in mstep, since the wizard uses step; Done becomes Continue (ondone).
	let {
		embedded
	}: {
		embedded?: { query: string; ondone?: () => void; onstep?: (s: Step) => void };
	} = $props();
	// svelte-ignore state_referenced_locally
	const stepKey = embedded ? 'mstep' : 'step';

	// What the page knows. An owner who signs in another way has no password to confirm: moving asks for one, so the step shows how to set it (Settings and the wizard).
	const passwordFlowCtx = {
		get runFlow() { return runFlow; },
	};
	const passwordFlow = createMovePassword(passwordFlowCtx);
	onMount(() => {
		const onReauth = (e: Event) => {
			const path = (e as CustomEvent<{ path?: string }>).detail?.path ?? '';
			if (!session.s.hasPassword && path.startsWith('/migrate')) {
				passwordFlow.mustSetPw = true;
				// Pressed Move (in Before you move or the confirm): ask right there, then carry on.
				if (beforeOpen || confirmMove || path === '/migrate/moves') {
					beforeOpen = false;
					confirmMove = false;
					passwordFlow.resumeMove = true;
					passwordFlow.pwSheet = true;
				}
			}
		};
		window.addEventListener('holyarrs:reauth', onReauth);
		return () => window.removeEventListener('holyarrs:reauth', onReauth);
	});
	$effect(() => {
		if (passwordFlow.mustSetPw && session.s.hasPassword) passwordFlow.mustSetPw = false;
	});
	let loaded = $state(false);
	let loadError = $state('');
	let movesError = $state('');
	let snapshots = $state<Snapshot[]>([]);
	let moves = $state.raw<Move[]>([]);
	let savedKeys = $state<SavedKey[]>([]);
	let folders = $state.raw<{ path: string; kind: string; label: string }[]>([]);
	let picked = $state<number[]>([]);
	let step = $state<Step>('read');
	let destroyed = false;

	// Step 1: the read sheet and Forget.
	let readOpen = $state(false);
	let readStart = $state<AppKind>('radarr');
	let readAgain = $state(false);
	let readAddress = $state('');
	let reading = $state('');
	let forgetting = $state<Snapshot | null>(null);
	let forgetOpen = $state(false);
	let forgetBusy = $state(false);
	let forgetError = $state('');

	// Step 2: the plan and the owner's choices. `changes` counts every change in steps 1 and 2 (and person choices), so a test run knows it is old.
	let plan = $state.raw<Plan | null>(null);
	let planning = $state(false);
	let planError = $state('');
	let picks = $state<Record<string, string>>({});
	let others = $state<Record<string, string>>({});
	let extra = $state<PathRule[]>([]);
	let suggested = $state<Record<string, string>>({});
	let missingPaths = $state<Record<string, string>>({});
	let seenFolders = $state<string[]>([]);
	let skip = $state<string[]>([]);
	let people = $state<Record<string, PersonChoice>>({});
	let changes = $state(0);
	let planTimer: ReturnType<typeof setTimeout> | undefined;
	let planSeq = 0;
	let planCtrl: AbortController | null = null;

	// The quiet test run before a move, kept in place while it runs again.
	let dry = $state.raw<Report | null>(null);
	let dryAt = $state(-1);
	let planHash = '';
	let trying = $state(false);
	let tryFailed = $state(false);
	let tryNotice = $state('');
	let seen = $state.raw(new Map<string, Line>());
	let trySeq = 0;
	let tryCtrl: AbortController | null = null;
	let checking = $state(false);
	let beforeOpen = $state(false);

	// Steps 3 and 4: the move.
	let current = $state.raw<Move | null>(null);
	let progress = $state(0);
	let progressNote = $state('');
	let lost = $state(false);
	let starting = $state(false);
	let actionError = $state('');
	let safeMode = $state(false);
	let gone = $state(false);
	let confirmMove = $state(false);
	let confirmUndo = $state(false);
	let undoing = $state(false);

	// The one list sheet every "Show" opens.
	let listOpen = $state(false);
	let listTitle = $state('');
	let listItems = $state.raw<readonly unknown[]>([]);
	let listRow = $state.raw<(t: unknown, i: number) => SheetRow>(() => ({ key: '', name: '' }));
	const show: ShowList = (title, items, row) => {
		listTitle = title;
		listItems = items;
		listRow = row as (t: unknown, i: number) => SheetRow;
		listOpen = true;
	};

	const follower = createFollower<MoveAnswer>({
		get: (id, signal) => api.get<MoveAnswer>(`/migrate/moves/${id}`, signal),
		done: (a) => !running(a.move),
		onTick: (a) => {
			lost = false;
			moves = moves.map((m) => (m.id === a.move.id ? { ...a.move, report: m.report } : m));
			if (current?.id === a.move.id) {
				current = a.move;
				progress = a.progress;
				progressNote = a.progress_note;
			}
		},
		onEnd: (a) => runFlow.finished(a.move),
		onLost: () => (lost = true),
		onBack: () => (lost = false)
	});
	onDestroy(() => {
		destroyed = true;
		follower.destroy();
		clearTimeout(planTimer);
		planCtrl?.abort();
		tryCtrl?.abort();
	});

	const usedBy = $derived.by(() => {
		const m = new Map<number, number>();
		// Newest first: the newest done move that used a read names it.
		for (const mv of moves) if (mv.state === 'done') for (const id of mv.snapshots ?? []) if (!m.has(id)) m.set(id, mv.id);
		return m;
	});
	// Only rows on screen count: a folder from a read switched off is not marked anywhere.
	const folderErrors = $derived(Object.keys(picks).filter((p) => (plan?.old_paths ?? []).includes(p) && folderProblem(p, picks[p], others[p]) !== '').length);
	const extraErrors = $derived(extraProblems(extra));
	const ruleErrors = $derived(folderErrors + extraErrors.filter((e) => e !== '').length);
	const tried = $derived(buckets(dry));
	const stale = $derived(!!dry && dryAt !== changes);
	const chooseLines = $derived.by(() => {
		const out = [...tried.choose];
		// A choice row stays after the choice, so it can be changed back.
		for (const [k, l] of seen) if (!out.some((x) => x.key === k) && (people[k] === 'merge' || people[k] === 'skip') && dry?.lines.some((x) => x.key === k)) out.push(l);
		return out;
	});
	const openChoices = $derived(chooseLines.filter((l) => l.result === 'skipped' && people[l.key] !== 'skip').length);
	const doneGroups = $derived(buckets(current?.report).groups);
	const doneOpen = $derived((current?.report?.lines ?? []).filter((l) => l.choose && l.result === 'skipped' && current?.choices?.people?.[l.key] !== 'skip').length);
	const withItems = $derived((plan?.groups ?? []).filter((g) => g.items.length > 0 && g.name !== 'other'));
	const moveRunning = $derived(!!current && running(current) && !current.undo_state);
	const total = $derived(moveTotal(dry?.counts ?? {}));
	// Lines that will not move as asked: not found here, won't work, or a person still to choose.
	const problems = $derived((dry?.lines ?? []).filter((l) => l.result === 'not_found' || l.result === 'failed').length + openChoices);
	const gate = $derived(
		stepGate({
			step,
			picked: picked.length,
			planning,
			planned: !!plan,
			folderErrors,
			ruleErrors: extraErrors.filter((e) => e !== '').length,
			groupsOn: withItems.filter((g) => !skip.includes(g.name)).length,
			groupsWithItems: withItems.length,
			trying,
			tried: !!dry,
			failed: tryFailed,
			stale,
			openChoices,
			moving: moveRunning || starting,
			safeMode
		})
	);
	const fromApps = $derived(
		[...new Set((dry?.apps ?? []).map(appName))].reduce((s, a, i, all) => (i === 0 ? a : i === all.length - 1 ? `${s} ${t('settings.move.and')} ${a}` : `${s}, ${a}`), '')
	);

	function quiet(err: unknown): boolean {
		return (err instanceof ApiError && err.body.code === 'reauth') || (err instanceof DOMException && err.name === 'AbortError');
	}

	// Loading: the reads first (the page needs them); the moves, saved keys and Library folders each fail on their own.
	async function load() {
		loadError = '';
		loaded = false;
		try {
			snapshots = (await api.get<{ snapshots: Snapshot[] }>('/migrate/snapshots')).snapshots;
		} catch (e) {
			if (destroyed) return;
			loadError = message(e);
			loaded = true;
			return;
		}
		if (destroyed) return;
		await Promise.all([loadMoves(), loadSaved(), loadFolders()]);
		// Left the page while it loaded: start nothing.
		if (destroyed) return;
		picked = defaultPicks();
		loaded = true;
		applied = page.url.search;
		const busy = moves.find(running);
		if (busy) follower.follow(busy.id);
		await arrive(page.url.searchParams);
	}
	async function loadMoves() {
		try {
			moves = (await api.get<{ moves: Move[] }>('/migrate/moves')).moves;
			movesError = '';
		} catch (e) {
			movesError = message(e);
		}
	}
	async function loadSaved() {
		// Only the owner sees them; anyone else simply has none.
		savedKeys = await api
			.get<{ saved: SavedKey[] }>('/migrate/saved-keys')
			.then((r) => r.saved ?? [])
			.catch(() => []);
	}
	async function loadFolders() {
		folders = await api
			.get<{ library_folders: { path: string; kind: string; label: string }[] }>('/library-folders')
			.then((r) => r.library_folders ?? [])
			.catch(() => []);
	}
	onMount(() => void load());

	/** Switches on the newest read of each copy of an app, unless a move already used it. */
	function defaultPicks(): number[] {
		return latestPerInstance(snapshots)
			.filter((s) => !usedBy.has(s.id))
			.map((s) => s.id);
	}

	// The address: one entry per step, so the browser's Back works.
	let applied = '';
	function urlFor(s: Step, moveId?: number): string {
		// Plain commas between read ids, so the address stays readable.
		const own = moveId ? `move=${moveId}` : s === 'read' ? '' : `${stepKey}=${s}` + (picked.length ? `&read=${picked.join(',')}` : '');
		const all = [embedded?.query ?? '', own].filter(Boolean).join('&');
		return all ? `?${all}` : '';
	}
	function setUrl(s: Step, moveId?: number, replace = false) {
		const search = urlFor(s, moveId);
		if (search === page.url.search) return;
		applied = search;
		void goto((embedded ? page.url.pathname : to('/settings/move')) + search, { replace, reset: false });
	}
	$effect(() => {
		const search = page.url.search;
		if (!loaded) return;
		untrack(() => {
			if (search === applied) return;
			applied = search;
			void arrive(page.url.searchParams);
		});
	});

	/** Shows what the address names (a reload, a link, Back). */
	async function arrive(q: { get(name: string): string | null }) {
		const mv = Number(q.get('move'));
		if (mv > 0) return openMove(mv, false);
		const s = asStep(q.get(stepKey)) ?? 'read';
		const ids = (q.get('read') ?? '')
			.split(',')
			.map(Number)
			.filter((id) => snapshots.some((x) => x.id === id));
		if (s !== 'read' && ids.length > 0 && ids.join(',') !== picked.join(',')) {
			picked = ids;
			changes++;
		}
		enter(s, false);
		// An old Try it address says Choose from now on.
		if (q.get(stepKey) === 'try' && step === 'choose') setUrl('choose', undefined, true);
	}

	function focusStep() {
		void tick().then(() => document.getElementById('move-step')?.focus({ preventScroll: false }));
	}

	/** Opens a step; a step that needs an earlier one falls back to it. */
	function enter(want: Step, push = true) {
		let s = want;
		if (s === 'done' && !current) s = 'read';
		if ((s === 'choose' || s === 'move') && picked.length === 0) s = 'read';
		// Move only shows a move that started; without one it is Choose.
		if (s === 'move' && !current) s = 'choose';
		const moved = s !== step;
		step = s;
		actionError = '';
		if (s !== 'choose') {
			tryNotice = '';
			beforeOpen = false;
		}
		if (s === 'read') gone = false;
		if (s !== 'choose') {
			// A re-plan still waiting from step 2 is not wanted any more.
			clearTimeout(planTimer);
			planCtrl?.abort();
			planning = false;
		}
		if (s === 'choose') void planFlow.makePlan();
		// A step that fell back puts its own name in the address, so Back stays one step.
		if (push) setUrl(s);
		else if (s !== want) setUrl(s, undefined, true);
		if (moved) focusStep();
	}

	function choices() {
		// Only rules that check out are sent; a row with a problem is marked on screen and holds the step.
		const ok = Object.fromEntries(Object.entries(picks).filter(([p, v]) => folderProblem(p, v, others[p]) === ''));
		return { snapshots: picked, path_rules: rulesFrom(ok, others, extra.filter((_, i) => extraErrors[i] === '')), skip, people };
	}

	function changed() {
		changes++;
		gone = false;
	}

	// Step 1.
	function onread(r: ReadAnswer, kind: AppKind, usedSaved: boolean) {
		snapshots = [r.snapshot, ...snapshots.filter((x) => x.id !== r.snapshot.id)];
		picked = [...picked.filter((id) => id !== r.snapshot.id), r.snapshot.id];
		// The server keeps the key until a move finishes: ask what it keeps now.
		void loadSaved();
		changed();
		const what = countWords(kind, r.snapshot.counts, 1);
		const keys = r.needs_reentry?.length ?? 0;
		toasts.push(t('settings.move.read', { app: appName(kind) }), 'ok', { body: keys > 0 ? t(keys === 1 ? 'settings.move.keysOne' : 'settings.move.keysOther', { n: keys }) : what || undefined });
	}
	function addApp() {
		readStart = 'radarr';
		readAgain = false;
		readAddress = '';
		readOpen = true;
	}
	function readAgainFor(s: Snapshot) {
		readStart = s.kind as AppKind;
		readAddress = s.address ?? '';
		readAgain = true;
		readOpen = true;
	}
	function askForget(s: Snapshot) {
		forgetting = s;
		forgetError = '';
		forgetOpen = true;
	}
	async function forget(e: SubmitEvent) {
		e.preventDefault();
		const s = forgetting;
		if (!s) return;
		forgetBusy = true;
		forgetError = '';
		try {
			await api.del(`/migrate/snapshots/${s.id}`);
			snapshots = snapshots.filter((x) => x.id !== s.id);
			picked = picked.filter((x) => x !== s.id);
			changed();
			forgetOpen = false;
			// Forgetting a read forgets the key kept for that app too.
			void loadSaved();
		} catch (err) {
			if (!quiet(err)) forgetError = message(err);
		} finally {
			forgetBusy = false;
		}
	}
	function pick(id: number, on: boolean) {
		picked = on ? [...picked.filter((x) => x !== id), id] : picked.filter((x) => x !== id);
		changed();
	}

	// Step 2: every change plans again; switches wait 400ms, typed paths wait for blur or Enter. Older answers are dropped.
	const planFlowCtx = {
		get planTimer() { return planTimer; },
		set planTimer(v) { planTimer = v; },
		get picked() { return picked; },
		get ruleErrors() { return ruleErrors; },
		get planSeq() { return planSeq; },
		set planSeq(v) { planSeq = v; },
		get planCtrl() { return planCtrl; },
		set planCtrl(v) { planCtrl = v; },
		get planning() { return planning; },
		set planning(v) { planning = v; },
		get planError() { return planError; },
		set planError(v) { planError = v; },
		choices,
		get destroyed() { return destroyed; },
		get plan() { return plan; },
		set plan(v) { plan = v; },
		get suggested() { return suggested; },
		set suggested(v) { suggested = v; },
		get missingPaths() { return missingPaths; },
		set missingPaths(v) { missingPaths = v; },
		get seenFolders() { return seenFolders; },
		set seenFolders(v) { seenFolders = v; },
		get picks() { return picks; },
		set picks(v) { picks = v; },
		get extra() { return extra; },
		set extra(v) { extra = v; },
		changed,
		quiet,
		get gone() { return gone; },
		set gone(v) { gone = v; },
		get others() { return others; },
		set others(v) { others = v; },
		get skip() { return skip; },
		set skip(v) { skip = v; },
		get people() { return people; },
		set people(v) { people = v; },
		get trySeq() { return trySeq; },
		set trySeq(v) { trySeq = v; },
		get tryCtrl() { return tryCtrl; },
		set tryCtrl(v) { tryCtrl = v; },
		get changes() { return changes; },
		get trying() { return trying; },
		set trying(v) { trying = v; },
		get tryFailed() { return tryFailed; },
		set tryFailed(v) { tryFailed = v; },
		get actionError() { return actionError; },
		set actionError(v) { actionError = v; },
		get dry() { return dry; },
		set dry(v) { dry = v; },
		get planHash() { return planHash; },
		set planHash(v) { planHash = v; },
		get dryAt() { return dryAt; },
		set dryAt(v) { dryAt = v; },
		get seen() { return seen; },
		set seen(v) { seen = v; },
	};
	const planFlow = createMovePlan(planFlowCtx);
	const runFlowCtx = {
		get checking() { return checking; },
		set checking(v) { checking = v; },
		get starting() { return starting; },
		set starting(v) { starting = v; },
		get actionError() { return actionError; },
		set actionError(v) { actionError = v; },
		get dry() { return dry; },
		set dry(v) { dry = v; },
		get stale() { return stale; },
		get trying() { return trying; },
		runTry: planFlow.runTry,
		get destroyed() { return destroyed; },
		get tryFailed() { return tryFailed; },
		get gone() { return gone; },
		set gone(v) { gone = v; },
		get step() { return step; },
		set step(v) { step = v; },
		get problems() { return problems; },
		get beforeOpen() { return beforeOpen; },
		set beforeOpen(v) { beforeOpen = v; },
		get confirmMove() { return confirmMove; },
		set confirmMove(v) { confirmMove = v; },
		choices,
		get planHash() { return planHash; },
		set planHash(v) { planHash = v; },
		get current() { return current; },
		set current(v) { current = v; },
		get progress() { return progress; },
		set progress(v) { progress = v; },
		get progressNote() { return progressNote; },
		set progressNote(v) { progressNote = v; },
		get lost() { return lost; },
		set lost(v) { lost = v; },
		get moves() { return moves; },
		set moves(v) { moves = v; },
		get follower() { return follower; },
		setUrl,
		focusStep,
		quiet,
		get safeMode() { return safeMode; },
		set safeMode(v) { safeMode = v; },
		get dryAt() { return dryAt; },
		set dryAt(v) { dryAt = v; },
		get tryNotice() { return tryNotice; },
		set tryNotice(v) { tryNotice = v; },
		loadSaved,
		get suggested() { return suggested; },
		set suggested(v) { suggested = v; },
		get picked() { return picked; },
		set picked(v) { picked = v; },
		changed,
		enter,
		get confirmUndo() { return confirmUndo; },
		set confirmUndo(v) { confirmUndo = v; },
		get undoing() { return undoing; },
		set undoing(v) { undoing = v; },
	};
	const runFlow = createMoveRun(runFlowCtx);
	async function openMove(id: number, push = true) {
		actionError = '';
		try {
			const r = await api.get<MoveAnswer>(`/migrate/moves/${id}`);
			current = r.move;
			progress = r.progress;
			progressNote = r.progress_note;
			lost = false;
			step = running(r.move) && !r.move.undo_state ? 'move' : 'done';
			if (running(r.move)) follower.follow(id);
			if (push) setUrl(step, id);
			focusStep();
		} catch (err) {
			if (quiet(err)) return;
			enter('read', false);
			actionError = message(err);
		}
	}
	function done() {
		current = null;
		dry = null;
		dryAt = -1;
		planHash = '';
		plan = null;
		suggested = {};
		missingPaths = {};
		tryNotice = '';
		enter('read');
	}
	function chooseAgain() {
		const m = current;
		if (!m) return;
		picked = m.snapshots.filter((id) => snapshots.some((s) => s.id === id));
		people = { ...people, ...((m.choices?.people ?? {}) as Record<string, PersonChoice>) };
		// The move's other choices too: the groups it left out and its folders.
		skip = [...(m.choices?.skip ?? skip)];
		const rules = m.choices?.path_rules ?? [];
		if (rules.length > 0) {
			const p = { ...picks };
			const free: PathRule[] = [];
			for (const r of rules) {
				if (r.from in p) continue;
				if (folders.some((f) => f.path === r.to)) p[r.from] = r.to;
				else free.push(r);
			}
			picks = p;
			extra = [...extra.filter((x) => !free.some((r) => r.from === x.from.trim())), ...free];
		}
		current = null;
		dry = null;
		changed();
		enter('choose');
	}
	function back(s: Step) {
		enter(s);
	}
	function goBackToReads() {
		gone = false;
		void reloadReads();
		enter('read');
	}
	async function reloadReads() {
		try {
			snapshots = (await api.get<{ snapshots: Snapshot[] }>('/migrate/snapshots')).snapshots;
			const n = picked.length;
			picked = picked.filter((id) => snapshots.some((s) => s.id === id));
			// A read gone means the last test run is old.
			if (picked.length !== n) changed();
		} catch {
			// The list stays as it was.
		}
	}

	$effect(() => embedded?.onstep?.(step));

	const railBusy = $derived(step === 'choose' && (checking || starting) ? t('settings.move.checking') : step === 'choose' && planning ? t('settings.move.updating') : '');
	const railFill = $derived(step === 'move' && moveRunning ? progress : 0.5);
</script>

<svelte:head>{#if !embedded}<title>{t('settings.move.title')} · HolyArrs</title>{/if}</svelte:head>

{#if !embedded}
	<PageHeader title={t('settings.move.title')} subtitle={t('settings.move.subtitle')} />
{/if}

{#if !loaded}
	<div class="sk-rail" aria-hidden="true">{#each [1, 2, 3, 4] as n (n)}<span class="sk"></span>{/each}</div>
	<SkSettings cards={2} rows={4} />
{:else if loadError}
	<LoadError hasData={false} message={loadError} onretry={load} />
{:else}
	{#if !embedded}<MoveSteps {step} busy={railBusy} fill={railFill} locked={moveRunning && step === 'move'} onpick={back} />{/if}
	{#if passwordFlow.mustSetPw && !passwordFlow.pwSheet}
		<SettingsCard id="move-password" title={t('settings.move.setPasswordFirst')} icon="shield">
			<form class="setpw" onsubmit={passwordFlow.setFirstPw}>
				<input class="input" type="password" bind:value={passwordFlow.pwA} placeholder={t('settings.move.passwordLabel')} aria-label={t('settings.move.passwordLabel')} autocomplete="new-password" />
				<input class="input" type="password" bind:value={passwordFlow.pwB} placeholder={t('settings.move.confirmPasswordLabel')} aria-label={t('settings.move.confirmPasswordLabel')} autocomplete="new-password" />
				<Button variant="primary" type="submit" busy={passwordFlow.pwBusy} disabled={passwordFlow.pwBusy}>{t('settings.move.setPasswordLabel')}</Button>
			</form>
			{#if passwordFlow.pwStale}
				<p class="form-error" role="alert">{t('settings.move.signInAgainNote')}</p>
				<div><Button onclick={passwordFlow.signInAgain}>{t('settings.move.signInAgain')}</Button></div>
			{:else if passwordFlow.pwErr}<p class="form-error" role="alert">{passwordFlow.pwErr}</p>{/if}
		</SettingsCard>
	{/if}

	{#if step === 'read'}
		<ReadStep
			{snapshots}
			{picked}
			{usedBy}
			{reading}
			{moves}
			{movesError}
			onpick={pick}
			onadd={addApp}
			onagain={readAgainFor}
			onforget={askForget}
			onopen={(m) => openMove(m.id)}
			onretrymoves={loadMoves}
		/>
	{:else if step === 'choose'}
		<ChooseStep
			{plan}
			{planning}
			{folders}
			{picks}
			{others}
			{extra}
			{extraErrors}
			{suggested}
			missing={missingPaths}
			seen={seenFolders}
			{skip}
			{people}
			onfolder={planFlow.onfolder}
			onother={planFlow.onother}
			onextra={planFlow.onextra}
			ongroup={planFlow.ongroup}
			onperson={planFlow.onperson}
			oncommit={() => planFlow.replan(false)}
			onshow={show}
		/>
		{#if planError}<p class="form-error" role="alert">{planError}</p>{/if}
	{:else if step === 'move'}
		<MoveStep {dry} running={moveRunning} {progress} note={progressNote} {lost} {safeMode} onshow={show} />
	{:else if step === 'done' && current}
		<DoneStep move={current} groups={doneGroups} openChoices={doneOpen} {progress} note={progressNote} {lost} onchoose={chooseAgain} onshow={show} />
	{/if}

	{#if gone}
		<div class="notice warn gone" role="alert">
			<span>{t('settings.move.gone')}</span>
			<Button onclick={goBackToReads}>{t('settings.move.backToRead')}</Button>
		</div>
	{/if}
	{#if actionError}<p class="form-error" role="alert">{actionError}</p>{/if}

	{#if step === 'read'}
		{#if snapshots.length > 0 || reading}
			<StepFoot label={t('settings.move.continue')} onclick={() => enter('choose')} disabled={!gate.ok} reason={gate.reason} />
		{/if}
	{:else if step === 'choose'}
		<StepFoot
			back={() => back('read')}
			label={t('settings.move.move')}
			onclick={() => void runFlow.moveNow()}
			disabled={!gate.ok || safeMode || moveRunning}
			busy={checking || starting}
			reason={safeMode ? t('settings.move.safeMode') : checking ? t('settings.move.checking') : gate.reason}
		/>
	{:else if step === 'move'}
		<StepFoot back={() => back('choose')} backDisabled={moveRunning || starting} />
	{:else if step === 'done' && current}
		{@const m = current}
		{@const busyNow = running(m)}
		{@const canUndo = !busyNow && (m.state === 'done' || m.state === 'failed') && m.undo_state !== 'failed'}
		{@const retry = !busyNow && m.undo_state === 'failed' && m.state !== 'undone'}
		{@const restart = !!embedded?.ondone && (m.state === 'undone' || m.state === 'failed')}
		<StepFoot
			label={busyNow ? undefined : retry ? t('settings.move.tryUndo') : restart ? t('settings.move.startOver') : embedded?.ondone ? t('settings.move.continue') : t('settings.move.done')}
			variant={retry ? 'destructive' : 'primary'}
			onclick={retry ? () => (confirmUndo = true) : restart ? done : (embedded?.ondone ?? done)}
			disabled={(safeMode || undoing) && retry}
			busy={undoing && retry}
		>
			{#snippet more()}
				{#if canUndo}<Button variant="destructive" onclick={() => (confirmUndo = true)} disabled={safeMode} busy={undoing}>{t('settings.move.undoThis')}</Button>{/if}
				{#if m.report && !busyNow}<AdminLinkButton icon="download" href={to(`/api/v1/migrate/moves/${m.id}/report`)} download>{t('settings.move.downloadReport')}</AdminLinkButton>{/if}
			{/snippet}
		</StepFoot>
	{/if}
{/if}

<ReadSheet bind:open={readOpen} bind:reading start={readStart} startAddress={readAddress} again={readAgain} saved={savedKeys} known={snapshots.map((s) => s.address ?? '')} {onread} />

<ItemsSheet bind:open={listOpen} title={listTitle} items={listItems} row={listRow} />

<Sheet bind:open={forgetOpen} title={forgetting ? t('settings.move.forgetApp', { app: appName(forgetting.kind) }) : t('settings.move.forget')} size="sm">
	<form id="move-forget-form" onsubmit={forget}>
		<p class="small muted">{t('settings.move.forgetNote')}</p>
		{#if forgetError}<p class="form-error" role="alert">{forgetError}</p>{/if}
	</form>
	{#snippet footer()}
		<Button variant="plain" onclick={() => (forgetOpen = false)}>{t('settings.move.cancel')}</Button>
		<Button variant="destructive" type="submit" form="move-forget-form" busy={forgetBusy}>{t('settings.move.remove')}</Button>
	{/snippet}
</Sheet>

<Sheet bind:open={passwordFlow.pwSheet} title={t('settings.move.setPasswordFirst')} size="sm">
	<form class="setpw one-col" onsubmit={passwordFlow.setFirstPw}>
		<input class="input" type="password" bind:value={passwordFlow.pwA} placeholder={t('settings.move.passwordLabel')} aria-label={t('settings.move.passwordLabel')} autocomplete="new-password" />
		<input class="input" type="password" bind:value={passwordFlow.pwB} placeholder={t('settings.move.confirmPasswordLabel')} aria-label={t('settings.move.confirmPasswordLabel')} autocomplete="new-password" />
		{#if passwordFlow.pwStale}
			<p class="form-error" role="alert">{t('settings.move.signInAgainNote')}</p>
			<Button onclick={passwordFlow.signInAgain}>{t('settings.move.signInAgain')}</Button>
		{:else if passwordFlow.pwErr}<p class="form-error" role="alert">{passwordFlow.pwErr}</p>{/if}
		<Button variant="primary" type="submit" busy={passwordFlow.pwBusy} disabled={passwordFlow.pwBusy}>{t('settings.move.setPasswordMove')}</Button>
	</form>
</Sheet>

<BeforeSheet
	bind:open={beforeOpen}
	groups={tried.groups}
	choose={chooseLines}
	{people}
	{total}
	{trying}
	ready={!!dry && !stale && !tryFailed}
	notice={tryNotice}
	onchoose={planFlow.onchoose}
	onmove={runFlow.moveAnyway}
	onshow={show}
/>

<Sheet bind:open={confirmMove} title={t('settings.move.moveItems', { items: plural(total, 'item') })} size="sm">
	<form id="move-confirm-form" onsubmit={runFlow.startMove}>
		{#if tryNotice}<p class="notice warn" role="status">{tryNotice}</p>{/if}
		<p class="small muted">{fromApps ? t('settings.move.from', { apps: fromApps }) + ' ' : ''}{embedded ? t('settings.move.undoLaterEmbedded') : t('settings.move.undoLaterPage')}</p>
	</form>
	{#snippet footer()}
		<Button variant="plain" onclick={() => (confirmMove = false)}>{t('settings.move.cancel')}</Button>
		<Button variant="primary" type="submit" form="move-confirm-form">{t('settings.move.move')}</Button>
	{/snippet}
</Sheet>

<Sheet bind:open={confirmUndo} title={current ? t('settings.move.undoId', { id: current.id }) : t('settings.move.undoQuestion')} size="sm">
	<form id="move-undo-form" onsubmit={runFlow.undo}>
		<p class="small muted">{t('settings.move.undoNote')}</p>
	</form>
	{#snippet footer()}
		<Button variant="plain" onclick={() => (confirmUndo = false)}>{t('settings.move.cancel')}</Button>
		<Button variant="destructive" type="submit" form="move-undo-form" disabled={undoing}>{t('settings.move.undoMove')}</Button>
	{/snippet}
</Sheet>

<style>
	.setpw { display: grid; grid-template-columns: 1fr 1fr auto; gap: 8px; }
	.setpw.one-col { grid-template-columns: 1fr; }
	@media (max-width: 600px) { .setpw { grid-template-columns: 1fr; } }
	.sk-rail { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px; }
	.sk-rail .sk { display: block; height: 40px; border-radius: 12px; }
	@media (pointer: coarse) {
		.sk-rail .sk { height: 44px; }
	}
	.gone { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px; }
	@media (max-width: 600px) {
		.sk-rail { grid-template-columns: minmax(0, 1fr) 44px 44px 44px; }
		.sk-rail .sk { height: 44px; }
	}
</style>
