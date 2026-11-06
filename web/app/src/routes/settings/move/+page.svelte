<script lang="ts">
	import { onDestroy, onMount, tick, untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { SkSettings } from '#lib/ui/sk/index.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import { session } from '#lib/session.svelte.ts';
	import AdminLinkButton from '#lib/ui/AdminLinkButton.svelte';
	import MoveSteps from '#lib/move/MoveSteps.svelte';
	import StepFoot from '#lib/move/StepFoot.svelte';
	import ReadSheet, { type ReadAnswer, type SavedKey } from '#lib/move/ReadSheet.svelte';
	import ItemsSheet, { type SheetRow, type ShowList } from '#lib/move/ItemsSheet.svelte';
	import ReadStep from '#lib/move/ReadStep.svelte';
	import ChooseStep from '#lib/move/ChooseStep.svelte';
	import BeforeSheet from '#lib/move/BeforeSheet.svelte';
	import MoveStep from '#lib/move/MoveStep.svelte';
	import DoneStep from '#lib/move/DoneStep.svelte';
	import { createFollower } from '#lib/move/poll.ts';
	import { api, ApiError, message } from '#lib/api.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { to } from '#lib/paths.ts';
	import {
		appName,
		asStep,
		buckets,
		countWords,
		extraProblems,
		folderProblem,
		moveTotal,
		OTHER,
		plural,
		rulesFrom,
		running,
		setChoice,
		stepGate,
		type AppKind,
		type Line,
		type Move,
		type PathRule,
		type PersonChoice,
		type Plan,
		type Report,
		type Snapshot,
		type Step
	} from '#lib/move.ts';

	// Settings > Move from other apps: four steps, one at a time. The step and
	// the reads in use live in the address (?step=choose&read=3,5, or ?move=7
	// for one move), so Back and a reload land on the same step. Move on
	// Choose runs a quiet test first; only problems open "Before you move".
	type MoveAnswer = { move: Move; progress: number; progress_note: string };

	// The first-run wizard shows this same flow inside its panel. There the
	// address keeps the wizard's own query (query) and the step goes in
	// mstep, since the wizard uses step; Done becomes Continue (ondone).
	let {
		embedded
	}: {
		embedded?: { query: string; ondone?: () => void; onstep?: (s: Step) => void };
	} = $props();
	const stepKey = embedded ? 'mstep' : 'step';

	// What the page knows.
	// An owner who signs in another way has no password to confirm: moving
	// asks for one, so the step shows how to set it (Settings and the wizard).
	let mustSetPw = $state(false);
	// Set it right here: the first password, then a fresh check with it, so Move works without leaving.
	let pwA = $state('');
	let pwB = $state('');
	let pwErr = $state('');
	let pwBusy = $state(false);
	async function setFirstPw(e: SubmitEvent) {
		e.preventDefault();
		pwErr = '';
		const min = session.s.minPassword;
		if (pwA.length < min) return void (pwErr = `Use at least ${min} characters.`);
		if (pwA !== pwB) return void (pwErr = "Passwords don't match.");
		pwBusy = true;
		try {
			await api.post('/auth/password', { new: pwA });
			session.s.hasPassword = true;
			await session.reauth(pwA);
			pwA = pwB = '';
			mustSetPw = false;
			pwSheet = false;
			if (resumeMove) {
				resumeMove = false;
				await start();
			} else toasts.push('Password set.', 'ok');
		} catch (err) {
			// A first password needs a fresh sign-in with the media server (the server's rule).
			if (err instanceof ApiError && err.body?.code === 'reauth') pwStale = true;
			else pwErr = message(err);
		} finally {
			pwBusy = false;
		}
	}
	let pwStale = $state(false);
	let pwSheet = $state(false);
	let resumeMove = $state(false);
	async function signInAgain() {
		const back = location.pathname + location.search;
		await session.logout();
		await goto(to(`/login?next=${encodeURIComponent(back)}`));
	}
	onMount(() => {
		const onReauth = (e: Event) => {
			const path = (e as CustomEvent<{ path?: string }>).detail?.path ?? '';
			if (!session.s.hasPassword && path.startsWith('/migrate')) {
				mustSetPw = true;
				// Pressed Move (in Before you move or the confirm): ask right there, then carry on.
				if (beforeOpen || confirmMove || path === '/migrate/moves') {
					beforeOpen = false;
					confirmMove = false;
					resumeMove = true;
					pwSheet = true;
				}
			}
		};
		window.addEventListener('holyarrs:reauth', onReauth);
		return () => window.removeEventListener('holyarrs:reauth', onReauth);
	});
	$effect(() => {
		if (mustSetPw && session.s.hasPassword) mustSetPw = false;
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
	let reading = $state('');
	let forgetting = $state<Snapshot | null>(null);
	let forgetOpen = $state(false);
	let forgetBusy = $state(false);
	let forgetError = $state('');

	// Step 2: the plan and the owner's choices. changes counts every change
	// in steps 1 and 2 (and person choices), so a test run knows it is old.
	let plan = $state.raw<Plan | null>(null);
	let planning = $state(false);
	let planError = $state('');
	let picks = $state<Record<string, string>>({});
	let others = $state<Record<string, string>>({});
	let extra = $state<PathRule[]>([]);
	let suggested = $state<Record<string, string>>({});
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
		onEnd: (a) => finished(a.move),
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

	// Derived state.
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
		[...new Set((dry?.apps ?? []).map(appName))].reduce((s, a, i, all) => (i === 0 ? a : i === all.length - 1 ? `${s} and ${a}` : `${s}, ${a}`), '')
	);

	function quiet(err: unknown): boolean {
		return (err instanceof ApiError && err.body.code === 'reauth') || (err instanceof DOMException && err.name === 'AbortError');
	}

	// Loading: the reads first (the page needs them); the moves, saved keys
	// and Library folders each fail on their own.
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

	/** defaultPicks switches on the newest read of each app, unless a move already used it. */
	function defaultPicks(): number[] {
		const out: number[] = [];
		const kinds = new Set<string>();
		for (const s of snapshots) {
			if (kinds.has(s.kind)) continue;
			kinds.add(s.kind);
			if (!usedBy.has(s.id)) out.push(s.id);
		}
		return out;
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

	/** arrive shows what the address names (a reload, a link, Back). */
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

	/** enter opens a step; a step that needs an earlier one falls back to it. */
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
		if (s === 'choose') void makePlan();
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
		toasts.push(`${appName(kind)} read${what ? ': ' + what : ''}.${keys > 0 ? ` ${plural(keys, 'key', 'keys')} to type again after the move.` : ''}`, 'ok');
	}
	function addApp() {
		readStart = 'radarr';
		readAgain = false;
		readOpen = true;
	}
	function readAgainFor(kind: AppKind) {
		readStart = kind;
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

	// Step 2: every change plans again; switches wait 400ms, typed paths
	// wait for blur or Enter. Older answers are dropped.
	function replan(wait: boolean) {
		clearTimeout(planTimer);
		if (wait) planTimer = setTimeout(() => void makePlan(), 400);
		else void makePlan();
	}
	async function makePlan() {
		clearTimeout(planTimer);
		if (picked.length === 0 || ruleErrors > 0) return;
		const seq = ++planSeq;
		planCtrl?.abort();
		planCtrl = new AbortController();
		planning = true;
		planError = '';
		try {
			const r = await api.request<{ plan: Plan; suggested_path_rules?: PathRule[] }>('/migrate/plan', { method: 'POST', body: choices(), signal: planCtrl.signal });
			if (seq !== planSeq || destroyed) return;
			plan = r.plan;
			const sug: Record<string, string> = {};
			for (const s of r.suggested_path_rules ?? []) sug[s.from] = s.to;
			suggested = sug;
			// A suggestion fills a folder nobody set yet; the plan then runs with it.
			const next = { ...picks };
			let added = false;
			for (const [from, dest] of Object.entries(sug)) {
				if (!(from in next) && !extra.some((r) => r.from.trim() === from)) {
					next[from] = dest;
					added = true;
				}
			}
			if (added) {
				picks = next;
				changed();
				void makePlan();
			}
		} catch (err) {
			if (seq !== planSeq || quiet(err)) return;
			if (err instanceof ApiError && err.status === 404) gone = true;
			else planError = message(err);
		} finally {
			if (seq === planSeq) planning = false;
		}
	}
	function onfolder(from: string, value: string) {
		picks = { ...picks, [from]: value };
		changed();
		if (value !== OTHER) replan(true);
	}
	function onother(from: string, value: string) {
		others = { ...others, [from]: value };
		changed();
	}
	function onextra(rules: PathRule[]) {
		extra = rules;
		changed();
	}
	function ongroup(name: string, on: boolean) {
		skip = on ? skip.filter((x) => x !== name) : [...skip, name];
		changed();
		replan(true);
	}
	function onperson(key: string, c: PersonChoice | null) {
		people = setChoice(people, key, c);
		changed();
		replan(true);
	}

	// The test run. A choice runs it again in place.
	async function runTry() {
		if (picked.length === 0 || ruleErrors > 0) return;
		// No plan yet (a reload, a link): plan first, so the suggested
		// folder matches are filled in before the test runs.
		if (!plan) {
			await makePlan();
			if (destroyed) return;
		}
		const seq = ++trySeq;
		tryCtrl?.abort();
		tryCtrl = new AbortController();
		const at = changes;
		trying = true;
		tryFailed = false;
		actionError = '';
		try {
			const r = await api.request<{ report: Report; plan_hash: string }>('/migrate/dry-run', { method: 'POST', body: choices(), signal: tryCtrl.signal });
			if (seq !== trySeq || destroyed) return;
			dry = r.report;
			planHash = r.plan_hash;
			dryAt = at;
			const m = new Map(seen);
			for (const l of r.report.lines) if (l.choose) m.set(l.key, l);
			seen = m;
		} catch (err) {
			if (seq !== trySeq || quiet(err)) return;
			if (err instanceof ApiError && err.status === 404) gone = true;
			else {
				tryFailed = true;
				actionError = message(err);
			}
		} finally {
			if (seq === trySeq) trying = false;
		}
	}
	function onchoose(key: string, c: PersonChoice) {
		people = setChoice(people, key, c);
		changed();
		void runTry();
	}

	/** moveNow is Choose's Move: a quiet test run, then the move (after a
	 * confirm), or "Before you move" when the test found problems. */
	async function moveNow() {
		if (checking || starting) return;
		checking = true;
		actionError = '';
		try {
			// A change while it runs makes it old: run again (a few times at most).
			for (let i = 0; i < 3 && (!dry || stale || trying); i++) {
				await runTry();
				if (destroyed || tryFailed || gone) return;
			}
			if (!dry || stale || step !== 'choose') return;
			if (problems > 0) beforeOpen = true;
			else confirmMove = true;
		} finally {
			checking = false;
		}
	}
	function moveAnyway() {
		beforeOpen = false;
		void start();
	}
	async function startMove(e: SubmitEvent) {
		e.preventDefault();
		confirmMove = false;
		await start();
	}

	// Step 3: the move.
	async function start() {
		if (!dry || stale) return;
		let recheck = false;
		starting = true;
		actionError = '';
		try {
			const r = await api.post<{ move: Move }>('/migrate/moves', { ...choices(), plan_hash: planHash });
			current = r.move;
			progress = 0;
			progressNote = '';
			lost = false;
			moves = [r.move, ...moves.filter((m) => m.id !== r.move.id)];
			// Choose starts the move now (there is no Move step before it): show
			// the Move step before following, so finished() lands on Done.
			step = 'move';
			beforeOpen = false;
			follower.follow(r.move.id);
			setUrl('move', r.move.id, false);
			// The Move button is gone now: keep focus on the step.
			focusStep();
		} catch (err) {
			if (quiet(err)) return;
			const text = message(err);
			if (err instanceof ApiError && err.status === 409 && /safe mode/i.test(text)) safeMode = true;
			else if (err instanceof ApiError && err.status === 409 && /changed/i.test(text)) {
				// The plan moved on since the test: check again and ask again.
				dryAt = -1;
				tryNotice = 'Something changed, so HolyArrs checked again.';
				// After finally: moveNow skips while starting is still true.
				recheck = true;
			} else if (err instanceof ApiError && err.status === 404) gone = true;
			else actionError = text;
		} finally {
			starting = false;
		}
		if (recheck) void moveNow();
	}

	/** finished hears a move or an undo end. */
	function finished(m: Move) {
		moves = [m, ...moves.filter((x) => x.id !== m.id)];
		if (m.state === 'undone') toasts.push(`Move ${m.id} undone.`, 'ok');
		else if (m.undo_state === 'failed') toasts.push(`Undo of move ${m.id} stopped. Try again.`, 'fail');
		else toasts.push(`Move ${m.id} finished.`, m.state === 'done' ? 'ok' : 'fail');
		// A finished move forgets the kept keys.
		if (m.state === 'done') void loadSaved();
		if (m.state !== 'undone' && !m.undo_state) {
			// A finished move needs a new test before another one: the reads
			// it used are switched off.
			dry = null;
			dryAt = -1;
			planHash = '';
			suggested = {};
			picked = picked.filter((id) => !m.snapshots.includes(id));
			changed();
			// Still on Choose for another round: it plans again with the reads left (or falls back to step 1).
			if (step === 'choose' || (step === 'move' && current?.id !== m.id)) enter('choose', false);
		}
		if (current?.id === m.id) {
			current = m;
			progress = 1;
			if (step === 'move' || step === 'done') {
				step = 'done';
				setUrl('done', m.id, true);
				focusStep();
			}
		}
	}

	// Step 4.
	async function undo(e: SubmitEvent) {
		e.preventDefault();
		confirmUndo = false;
		const m = current;
		if (!m || undoing) return;
		undoing = true;
		actionError = '';
		try {
			const r = await api.post<{ move: Move }>(`/migrate/moves/${m.id}/undo`);
			current = r.move;
			progress = 0;
			progressNote = '';
			lost = false;
			follower.follow(m.id);
			// The Undo button is gone now: keep focus on the step.
			focusStep();
		} catch (err) {
			if (quiet(err)) return;
			if (err instanceof ApiError && err.status === 409 && /safe mode/i.test(message(err))) safeMode = true;
			actionError = message(err);
		} finally {
			undoing = false;
		}
	}
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

	const railBusy = $derived(step === 'choose' && (checking || starting) ? 'Checking…' : step === 'choose' && planning ? 'Updating…' : '');
	const railFill = $derived(step === 'move' && moveRunning ? progress : 0.5);
</script>

<svelte:head>{#if !embedded}<title>Move from other apps · HolyArrs</title>{/if}</svelte:head>

{#if !embedded}
	<PageHeader title="Move from other apps" subtitle="Your old apps stay untouched." />
{/if}

{#if !loaded}
	<div class="sk-rail" aria-hidden="true">{#each [1, 2, 3, 4] as n (n)}<span class="sk"></span>{/each}</div>
	<SkSettings cards={2} rows={4} />
{:else if loadError}
	<LoadError hasData={false} message={loadError} onretry={load} />
{:else}
	{#if !embedded}<MoveSteps {step} busy={railBusy} fill={railFill} locked={moveRunning && step === 'move'} onpick={back} />{/if}
	{#if mustSetPw && !pwSheet}
		<SettingsCard id="move-password" title="Set a password first" icon="shield">
			<form class="setpw" onsubmit={setFirstPw}>
				<input class="input" type="password" bind:value={pwA} placeholder="Password" aria-label="Password" autocomplete="new-password" />
				<input class="input" type="password" bind:value={pwB} placeholder="Confirm password" aria-label="Confirm password" autocomplete="new-password" />
				<Button variant="primary" type="submit" busy={pwBusy} disabled={pwBusy}>Set password</Button>
			</form>
			{#if pwStale}
				<p class="form-error" role="alert">Sign in again to continue. You'll return here.</p>
				<div><Button onclick={signInAgain}>Sign in again</Button></div>
			{:else if pwErr}<p class="form-error" role="alert">{pwErr}</p>{/if}
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
			{skip}
			{people}
			{onfolder}
			{onother}
			{onextra}
			{ongroup}
			{onperson}
			oncommit={() => replan(false)}
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
			<span>A read you picked is gone. Pick again.</span>
			<Button onclick={goBackToReads}>Back to Read your apps</Button>
		</div>
	{/if}
	{#if actionError}<p class="form-error" role="alert">{actionError}</p>{/if}

	{#if step === 'read'}
		{#if snapshots.length > 0 || reading}
			<StepFoot label="Continue" onclick={() => enter('choose')} disabled={!gate.ok} reason={gate.reason} />
		{/if}
	{:else if step === 'choose'}
		<StepFoot
			back={() => back('read')}
			label="Move"
			onclick={() => void moveNow()}
			disabled={!gate.ok || safeMode || moveRunning}
			busy={checking || starting}
			reason={safeMode ? 'HolyArrs started in safe mode.' : checking ? 'Checking…' : gate.reason}
		/>
	{:else if step === 'move'}
		<StepFoot back={() => back('choose')} backDisabled={moveRunning || starting} />
	{:else if step === 'done' && current}
		{@const m = current}
		{@const busyNow = running(m)}
		{@const canUndo = !busyNow && (m.state === 'done' || m.state === 'failed') && m.undo_state !== 'failed'}
		{@const retry = !busyNow && m.undo_state === 'failed' && m.state !== 'undone'}
		<StepFoot
			label={busyNow ? undefined : retry ? 'Try undo again' : embedded?.ondone ? 'Continue' : 'Done'}
			variant={retry ? 'destructive' : 'primary'}
			onclick={retry ? () => (confirmUndo = true) : (embedded?.ondone ?? done)}
			disabled={(safeMode || undoing) && retry}
			busy={undoing && retry}
		>
			{#snippet more()}
				{#if canUndo}<Button variant="destructive" onclick={() => (confirmUndo = true)} disabled={safeMode} busy={undoing}>Undo this move</Button>{/if}
				{#if m.report && !busyNow}<AdminLinkButton icon="download" href={to(`/api/v1/migrate/moves/${m.id}/report`)} download>Download the report</AdminLinkButton>{/if}
			{/snippet}
		</StepFoot>
	{/if}
{/if}

<ReadSheet bind:open={readOpen} bind:reading start={readStart} again={readAgain} saved={savedKeys} {onread} />

<ItemsSheet bind:open={listOpen} title={listTitle} items={listItems} row={listRow} />

<Sheet bind:open={forgetOpen} title={forgetting ? `Forget this ${appName(forgetting.kind)} read?` : 'Forget this read?'} size="sm">
	<form id="move-forget-form" onsubmit={forget}>
		<p class="small muted">Finished moves stay. You can read it again.</p>
		{#if forgetError}<p class="form-error" role="alert">{forgetError}</p>{/if}
	</form>
	{#snippet footer()}
		<Button variant="plain" onclick={() => (forgetOpen = false)}>Cancel</Button>
		<Button variant="destructive" type="submit" form="move-forget-form" busy={forgetBusy}>Remove</Button>
	{/snippet}
</Sheet>

<Sheet bind:open={pwSheet} title="Set a password first" size="sm">
	<form class="setpw one-col" onsubmit={setFirstPw}>
		<input class="input" type="password" bind:value={pwA} placeholder="Password" aria-label="Password" autocomplete="new-password" />
		<input class="input" type="password" bind:value={pwB} placeholder="Confirm password" aria-label="Confirm password" autocomplete="new-password" />
		{#if pwStale}
			<p class="form-error" role="alert">Sign in again to continue. You'll return here.</p>
			<Button onclick={signInAgain}>Sign in again</Button>
		{:else if pwErr}<p class="form-error" role="alert">{pwErr}</p>{/if}
		<Button variant="primary" type="submit" busy={pwBusy} disabled={pwBusy}>Set password and move</Button>
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
	{onchoose}
	onmove={moveAnyway}
	onshow={show}
/>

<Sheet bind:open={confirmMove} title="Move {plural(total, 'item')}?" size="sm">
	<form id="move-confirm-form" onsubmit={startMove}>
		{#if tryNotice}<p class="notice warn" role="status">{tryNotice}</p>{/if}
		<p class="small muted">{fromApps ? `From ${fromApps}. ` : ''}Paused items stay paused. You can undo this later from Earlier moves {embedded ? 'in Settings, Move from other apps' : 'on this page'}.</p>
	</form>
	{#snippet footer()}
		<Button variant="plain" onclick={() => (confirmMove = false)}>Cancel</Button>
		<Button variant="primary" type="submit" form="move-confirm-form">Move</Button>
	{/snippet}
</Sheet>

<Sheet bind:open={confirmUndo} title={current ? `Undo move ${current.id}?` : 'Undo this move?'} size="sm">
	<form id="move-undo-form" onsubmit={undo}>
		<p class="small muted">Removes what this move added; your later changes stay. Download the report first if you want it.</p>
	</form>
	{#snippet footer()}
		<Button variant="plain" onclick={() => (confirmUndo = false)}>Cancel</Button>
		<Button variant="destructive" type="submit" form="move-undo-form" disabled={undoing}>Undo move</Button>
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
