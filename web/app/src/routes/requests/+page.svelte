<script lang="ts">
	import SearchOutcomeLine from '#lib/ui/SearchOutcomeLine.svelte';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import { glide } from '#lib/ui/transitions.ts';
	import Button from '#lib/ui/Button.svelte';
	import { to } from '#lib/paths.ts';
	import { onDestroy, onMount, tick } from 'svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import { api, message } from '#lib/api.ts';
	import Chip from '#lib/ui/Chip.svelte';
	import StatusPill, { type StatusKind } from '#lib/ui/StatusPill.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Menu from '#lib/ui/Menu.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Poster from '#lib/Poster.svelte';
	import { ago, dateTime } from '#lib/format.ts';
	import { live } from '#lib/live.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { loadProfile, profile } from '#lib/account/profile.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { looksLikeId } from '#lib/plain.ts';
	import { approverCounts as approverCountsOf, approverSort, approverTab as approverTabOf, askedBy as askedByRaw, askTab, askTabs, decided as decidedRaw, firstTab, limitLines, limitKinds, tabCounts, type ApproverTab, type AskTab } from '#lib/requests.ts';
	import { partsNow } from '#lib/parts.svelte.ts';
	import { inbox, markRead, refreshInbox } from '#lib/requests.svelte.ts';
	import type { Ask, AskLimit, LibraryFolder, PickRulesView, SearchView } from '#lib/types.ts';
	import type { DownloadRow } from '#lib/downloaders.ts';
	import { liveByTitle, liveFacts, type TitleLive } from '#lib/approvals/live.ts';
	import { seasonWords } from '#lib/shows.ts';
	import { albumWords } from '#lib/music/music.ts';
	import { titleStatus } from '#lib/title-status.ts';
	import { askHref, type ApprovalsCtx, type DecideOptions, type Member } from '#lib/approvals/approvals.ts';
	import { askDetails, askInfo, seasonEpisodes, wantDetails } from '#lib/approvals/details.svelte.ts';
	import { askedByOne, askers } from '#lib/approvals/inbox.ts';
	import { kidLabel } from '#lib/kid.ts';
	import ApprovalsDesktop from '#lib/approvals/ApprovalsDesktop.svelte';

	const approver = $derived(session.can('approve'));
	let asks = $state<Ask[]>([]);
	let loaded = $state(false);
	let error = $state('');
	let tab = $state<AskTab>('waiting');
	// Approvers see three tabs (Pending, Processing, Done); members keep the five.
	let apTab = $state<ApproverTab>('pending');
	let kind = $state<'all' | 'movie' | 'show' | 'artist'>('all');
	let limit = $state<AskLimit | null>(null);
	// One line per kind whose part is on.
	const onKinds = $derived(limitKinds.filter((k) => partsNow.answer?.parts.find((p) => p.part === k)?.on !== false));

	// Asked by: one person's asks (0 is everyone's), for the Inbox looks' filter.
	let person = $state(0);
	const ofKind = $derived((kind === 'all' ? asks : asks.filter((a) => a.kind === kind)).filter((a) => !person || askedByOne(a, person)));
	// A decision waiting out its Undo time already shows in its new tab.
	type Pending = { mode: 'approve' | 'turn_down'; note: string; rulesId: number; langs: string[]; seasons?: number[]; episodes?: { season: number; episodes: number[] }[]; albums?: string[]; songs?: { album: string; recordings: string[] }[]; timer: ReturnType<typeof setTimeout>; sending?: boolean };
	let pending = $state<Record<number, Pending>>({});
	const viewOf = (a: Ask): Ask => {
		const p = pending[a.id];
		if (!p) return a;
		return { ...a, state: p.mode === 'approve' ? 'approved' : 'turned_down', title_id: a.title_id ?? (p.mode === 'approve' ? -1 : undefined) };
	};
	const viewed = $derived(ofKind.map(viewOf));
	const shown = $derived(approver ? approverSort(apTab, viewed.filter((a) => approverTabOf(a) === apTab)) : viewed.filter((a) => askTab(a) === tab));
	const apCounts = $derived(approverCountsOf(viewed));
	const tabs = $derived(askTabs.filter((t) => approver || t.tab !== 'failed'));
	// The kind chips show once asks of more than one kind are listed.
	const bothKinds = $derived(new Set(asks.map((a) => a.kind)).size > 1);
	const counts = $derived(tabCounts(viewed));
	// Bulk choice: waiting asks only.
	let chosen = $state<Set<number>>(new Set());
	const choosable = $derived(approver && apTab === 'pending');
	const chosenAsks = $derived(shown.filter((a) => chosen.has(a.id)));
	function toggleChosen(id: number) {
		const next = new Set(chosen);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		chosen = next;
	}
	const UNDO_MS = 6000;

	async function commit(id: number) {
		const p = pending[id];
		// The ask keeps showing its new tab until the answer lands (no flick back to Pending).
		if (!p || p.sending) return;
		clearTimeout(p.timer);
		p.sending = true;
		try {
			const path = `/asks/${id}/${p.mode === 'approve' ? 'approve' : 'turn-down'}`;
			const body = p.mode === 'approve' ? { note: p.note, library_folder_id: 0, pick_rules_id: p.rulesId, ...(p.langs.length ? { audio_languages: p.langs } : {}), ...(p.seasons?.length || p.episodes?.length ? { seasons: p.seasons ?? [] } : {}), ...(p.episodes?.length ? { episodes: p.episodes } : {}), ...(p.albums?.length ? { albums: p.albums } : {}), ...(p.songs?.length ? { songs: p.songs } : {}) } : { note: p.note };
			const r = await api.post<{ ask: Ask }>(path, body);
			// A list load that started before this answer is older than it; drop that load.
			loadGen++;
			const i = asks.findIndex((x) => x.id === r.ask.id);
			if (i >= 0) asks[i] = r.ask;
			void refreshInbox();
		} catch (err) {
			const a = asks.find((x) => x.id === id);
			toasts.push(`Couldn’t ${p.mode === 'approve' ? 'approve' : 'decline'} ${a?.name ?? 'the request'}. ${message(err)} Try again.`, 'fail');
		} finally {
			delete pending[id];
		}
	}

	/** act decides a set of asks after an Undo window; the toast's Undo cancels them all. */
	function act(list: Ask[], mode: 'approve' | 'turn_down', why = '', opt?: DecideOptions) {
		// Waiting asks, and (to decline only) approved ones whose title never got added (Failed).
		const open = (a: Ask) => a.state === 'asked' || (mode === 'turn_down' && a.state === 'approved' && !a.title_id);
		const ids = list.filter((a) => open(a) && !pending[a.id]).map((a) => a.id);
		if (!ids.length) {
			if (list.length && !list.some((a) => pending[a.id])) toasts.push(`Couldn’t ${mode === 'approve' ? 'approve' : 'decline'} ${list.length === 1 ? list[0].name : `${list.length} requests`}. It was already answered.`, 'fail');
			return;
		}
		for (const id of ids) pending[id] = { mode, note: why, rulesId: opt?.rulesId ?? 0, langs: opt?.langs ?? [], seasons: opt?.seasons, episodes: opt?.episodes, albums: opt?.albums, songs: opt?.songs, timer: setTimeout(() => void commit(id), UNDO_MS) };
		chosen = new Set();
		// Words follow the asks actually decided (some in the list may be answered already).
		const one = ids.length === 1 ? (list.find((a) => a.id === ids[0])?.name ?? 'the request') : `${ids.length} requests`;
		const verb = mode === 'approve' ? 'Approved' : 'Declined';
		const toastId = toasts.push(`${verb} ${one}.`, 'ok', {
			ms: UNDO_MS,
			action: {
				label: 'Undo',
				run: () => {
					for (const id of ids) {
						const p = pending[id];
						if (!p || p.sending) continue;
						clearTimeout(p.timer);
						delete pending[id];
					}
				}
			}
		});
		for (const id of ids) undoToast.set(id, toastId);
	}
	// Each waiting decision's Undo toast, so leaving the page can take the Undo away.
	const undoToast = new Map<number, number>();
	// Leaving the page sends what is still waiting out its Undo time.
	// Its Undo can't work after that, so the toast goes too.
	onDestroy(() =>
		Object.keys(pending).forEach((id) => {
			const t = undoToast.get(Number(id));
			if (t) toasts.dismiss(t);
			void commit(Number(id));
		})
	);

	// Decline asks for an optional reason the asker sees.
	let declineOpen = $state(false);
	let declineList = $state<Ask[]>([]);
	let declineWhy = $state('');
	let whyBox = $state<HTMLTextAreaElement>();
	// One-tap reasons for the people who asked; the box stays free to type.
	const reasons = ['Already have it', 'Not for kids', 'Asked before', 'Not enough space right now'];
	function askDecline(list: Ask[]) {
		declineList = list;
		declineWhy = '';
		declineOpen = true;
		// The reason box takes focus, so the sheet opens ready to type (no ring on its close button).
		void tick().then(() => setTimeout(() => whyBox?.focus({ preventScroll: true }), 60));
	}
	// A reason chip adds its words to what is typed; tapping it again takes them out.
	const hasWhy = (why: string) => declineWhy.includes(why);
	function tapWhy(why: string) {
		const text = declineWhy.trim();
		if (hasWhy(why)) {
			declineWhy = text
				.split(/\.\s*/)
				.map((s) => s.trim())
				.filter((s) => s && s !== why)
				.join('. ');
			if (declineWhy === text) declineWhy = text.replace(why, '').replace(/\s{2,}/g, ' ').trim();
		} else declineWhy = text ? `${text.replace(/\.$/, '')}. ${why}` : why;
		whyBox?.focus({ preventScroll: true });
	}
	function confirmDecline(e: SubmitEvent) {
		e.preventDefault();
		// The button that opened the sheet leaves with the ask: focus goes to the window, or the next card.
		const ids = new Set(declineList.map((a) => a.id));
		const cards = [...document.querySelectorAll<HTMLElement>('.bc[data-ask]')];
		const at = cards.findIndex((c) => ids.has(Number(c.dataset.ask)));
		const nextCard = cards.slice(at + 1).find((c) => !ids.has(Number(c.dataset.ask))) ?? cards.slice(0, Math.max(at, 0)).find((c) => !ids.has(Number(c.dataset.ask)));
		act(declineList, 'turn_down', declineWhy.trim());
		declineOpen = false;
		setTimeout(() => {
			const a = document.activeElement;
			if (a && a !== document.body && a.isConnected && !a.closest('[inert]')) return;
			const win = document.querySelector<HTMLElement>('dialog.win[open]');
			(win ?? nextCard?.querySelector<HTMLElement>('.open') ?? null)?.focus({ preventScroll: true });
		}, 80);
	}
	let selectedId = $state(0);
	const selected = $derived(shown.find((a) => a.id === selectedId) ?? shown[0] ?? null);
	let reviewHeading: HTMLHeadingElement | undefined = $state();
	let reviewOpen = $state(false);
	let reviewTrigger: HTMLButtonElement | undefined = $state();
	let compactReview = $state(false);
	let markingRead = $state(false);
	let takingBack = $state<Set<number>>(new Set());

	async function selectAsk(id: number, trigger?: HTMLButtonElement) {
		selectedId = id;
		reviewTrigger = trigger;
		reviewOpen = true;
		await tick();
		if (!reviewHeading) return;
		reviewHeading.focus({ preventScroll: true });
		if (window.matchMedia('(max-width: 980px)').matches) {
			const rect = reviewHeading.getBoundingClientRect();
			if (rect.top < 72 || rect.bottom > window.innerHeight) {
				const behavior = window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth';
				reviewHeading.scrollIntoView({ behavior, block: 'start' });
			}
		}
	}

	async function closeReview() {
		reviewOpen = false;
		await tick();
		reviewTrigger?.focus();
	}

	// Approve and turn down dialogs.
	let decideOpen = $state(false);
	let picked = $state<Ask | null>(null);
	let mode = $state<'approve' | 'turn_down'>('approve');
	let note = $state('');
	let folders = $state<LibraryFolder[]>([]);
	let rules = $state<PickRulesView[]>([]);
	let folderId = $state(0);
	let rulesId = $state(0);
	let busy = $state(false);
	let dialogError = $state('');
	// The approve dialog offers the folders and Pick rules for the ask's kind.
	const pickKind = $derived(picked?.kind === 'show' ? 'show' : picked?.kind === 'artist' ? 'music' : 'movie');
	const kindFolders = $derived(folders.filter((f) => f.kind === pickKind));
	const kindRules = $derived(rules.filter((r) => !r.kinds.length || r.kinds.includes(pickKind)));

	let loadGen = 0;
	async function load() {
		const gen = ++loadGen;
		try {
			const r = await api.get<{ asks: Ask[] }>('/asks');
			if (gen !== loadGen) return;
			asks = r.asks;
			error = '';
		} catch (e) {
			// A background refresh that fails keeps the list on screen and says so once.
			if (asks.length) toasts.push(`Couldn’t refresh your requests. ${message(e)}`, 'fail');
			error = message(e);
		} finally {
			loaded = true;
		}
		if (session.can('ask') && !approver) {
			api
				.get<AskLimit>('/asks/limit')
				.then((r) => (limit = r))
				.catch(() => {});
		}
	}

	onMount(() => {
		if (approver) void loadProfile();
		void load().then(() => {
			// The layouts open on Waiting for approvers, so an empty queue shows its calm state.
			tab = approver ? 'waiting' : firstTab(asks);
			// ?ask=<id> (History's "Go to Requests"): a member's list opens on that ask, picked and scrolled to.
			// Approvers get its window from the Cards (lib/approvals/ApprovalsDesktop.svelte).
			const want = Number(new URLSearchParams(location.search).get('ask')) || 0;
			const a = !approver && want ? asks.find((x) => x.id === want) : undefined;
			if (!a) return;
			tab = askTab(a);
			selectedId = a.id;
			void tick().then(() => document.querySelector(`[data-ask-id="${a.id}"]`)?.scrollIntoView({ block: 'center' }));
		});
		void refreshInbox();
		myId = session.s.id;
		if (session.can('manage_library')) {
			api
				.get<{ library_folders: LibraryFolder[] }>('/library-folders')
				.then((r) => (folders = r.library_folders))
				.catch(() => {});
			api
				.get<{ pick_rules: PickRulesView[] }>('/pick-rules')
				.then((r) => (rules = r.pick_rules))
				.catch(() => {});
		}
		// The Requests page's own Pick rules, to name the default rule (settings people only).
		if (session.can('settings')) {
			api
				.get<Record<string, unknown>>('/settings/asks')
				.then((r) => (askSettings = r))
				.catch(() => {});
		}
		const compactQuery = window.matchMedia('(max-width: 600px)');
		const syncCompactReview = () => (compactReview = compactQuery.matches);
		syncCompactReview();
		compactQuery.addEventListener('change', syncCompactReview);
		void loadLive();
		const offs = [
			live.on('ask', () => void load()),
			live.on('ask_removed', () => void load()),
			...['download', 'slot', 'import', 'resync'].map((t) => live.on(t, () => void loadLive()))
		];
		return () => {
			compactQuery.removeEventListener('change', syncCompactReview);
			offs.forEach((off) => off());
		};
	});

	// Where approved asks are in the pipeline (local lists, refreshed on the same live events as the sidebar).
	let titleLive = $state(new Map<number, TitleLive>());
	let liveBusy = false;
	let liveAgain = false;
	async function loadLive() {
		if (!session.can('manage_library')) return;
		// A live event during a read asks for one more read once it ends, so the last change is never lost.
		if (liveBusy) return void (liveAgain = true);
		liveBusy = true;
		try {
			const [d, s] = await Promise.all([
				// The newest 500 downloads (the most the list gives): enough for every ask still being got.
				api.get<{ downloads: DownloadRow[] }>('/downloads/view?limit=500'),
				api.get<{ running: SearchView[] }>('/searches')
			]);
			titleLive = liveByTitle(d.downloads, s.running ?? []);
		} catch {
			// keep the last answer
		} finally {
			liveBusy = false;
			if (liveAgain) {
				liveAgain = false;
				void loadLive();
			}
		}
	}

	function open(a: Ask, m: 'approve' | 'turn_down') {
		picked = a;
		mode = m;
		note = '';
		dialogError = '';
		folderId = 0;
		rulesId = 0;
		decideOpen = true;
	}

	async function decide(e: SubmitEvent) {
		e.preventDefault();
		if (!picked) return;
		busy = true;
		dialogError = await decideNow(picked, mode, note, folderId, rulesId);
		if (!dialogError) decideOpen = false;
		busy = false;
	}

	/** decideNow approves (with a Library folder and Pick rules) or turns down one ask straight away; '' when done, else the error. */
	async function decideNow(a: Ask, mode: 'approve' | 'turn_down', note: string, folderId: number, rulesId: number): Promise<string> {
		try {
			const path = `/asks/${a.id}/${mode === 'approve' ? 'approve' : 'turn-down'}`;
			const body = mode === 'approve' ? { note, library_folder_id: folderId, pick_rules_id: rulesId } : { note };
			const r = await api.post<{ ask: Ask }>(path, body);
			// A list load that started before this answer is older than it; drop that load.
			loadGen++;
			const i = asks.findIndex((x) => x.id === r.ask.id);
			if (i >= 0) asks[i] = r.ask;
			// Short, like the card: what happens next is on the Processing tab.
			toasts.push(mode === 'approve' ? `Approved ${r.ask.name}.` : `Declined ${r.ask.name}.`, 'ok');
			void refreshInbox();
			return '';
		} catch (err) {
			return message(err);
		}
	}

	// Take back asks first in a confirm sheet.
	let takeBackOpen = $state(false);
	let takeBackAsk = $state<Ask | null>(null);
	let takeBackError = $state('');
	// Where focus goes once the sheet has closed after a Take back.
	let focusAfterTakeBack: (() => void) | null = null;
	function askTakeBack(a: Ask) {
		takeBackAsk = a;
		takeBackError = '';
		takeBackOpen = true;
	}

	// The sheet stays open, busy, while the ask is taken back; a failure shows in it.
	async function takeBack(a: Ask) {
		if (takingBack.has(a.id)) return;
		takeBackError = '';
		const at = shown.findIndex((x) => x.id === a.id);
		takingBack = new Set([...takingBack, a.id]);
		try {
			await api.del(`/asks/${a.id}`);
			toasts.push(`Took back ${a.name}.`);
			await load();
			focusAfterTakeBack = () => {
				const rows = document.querySelectorAll<HTMLElement>('[data-testid="ask"]');
				const row = rows.length ? rows[Math.min(Math.max(at, 0), rows.length - 1)] : undefined;
				const heading = document.querySelector<HTMLElement>('main h1');
				const target = row?.querySelector<HTMLElement>('button, a') ?? heading;
				if (target === heading) heading?.setAttribute('tabindex', '-1');
				target?.focus();
			};
			takeBackOpen = false;
		} catch (err) {
			takeBackError = message(err);
		} finally {
			takingBack = new Set([...takingBack].filter((id) => id !== a.id));
		}
	}

	function takeBackClosed() {
		const f = focusAfterTakeBack;
		focusAfterTakeBack = null;
		// After the browser has put focus back on the (now gone) opener.
		if (f) setTimeout(f, 0);
	}

	async function readAllMessages() {
		if (markingRead) return;
		markingRead = true;
		try {
			await markRead();
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			markingRead = false;
		}
	}

	// One status vocabulary (title-status.ts): approvers get the staff words.
	const askPill = (a: Ask) => titleStatus({ ask: a.state, titleKind: a.kind === 'artist' ? 'artist' : a.kind === 'show' ? 'show' : 'movie' }, approver ? 'staff' : 'member');
	const msgKind = (k: string): StatusKind => (k === 'ready' ? 'in_library' : k === 'turned_down' ? 'turned_down' : 'approved');

	// Id-like account names read as a person, never a raw id.
	const plain = (t: string) => t.replace(/[\w.-]+/g, (w) => (/\d/.test(w) && /\p{L}/u.test(w) && looksLikeId(w) ? 'someone in your household' : w));
	const askedBy = (a: Ask) => plain(askedByRaw(a));
	const decided = (a: Ask) => plain(decidedRaw(a, session.s.id != null ? { id: session.s.id, displayName: profile.displayName } : undefined) ?? '');

	function year(a: Ask) {
		return a.year ? ` (${a.year})` : '';
	}

	/** what says what a show or artist ask is for; '' for movies. */
	function what(a: Ask) {
		if (a.kind === 'artist') return a.album_words ?? albumWords(a.albums);
		return a.kind === 'show' ? (a.season_words ?? seasonWords(a.seasons)) : '';
	}

	/** approvedLine says what happens after approval. */
	function approvedLine(a: Ask) {
		if (a.kind === 'show') return `HolyArrs keeps ${what(a)} up to date and looks for the episodes.`;
		if (a.kind === 'artist') return `HolyArrs keeps ${what(a)} up to date. HolyArrs can't look for music downloads yet.`;
		return 'HolyArrs is looking for it.';
	}

	// Who asked: picture and age limit, for the Inbox looks.
	let members = $state<Record<number, Member>>({});
	let membersAsked = false;
	$effect(() => {
		// The people list is an admin route that wants a recent local password proof
		// (10 minutes): without one it answers 403, so it is not asked; the rows
		// then show initials and no age-limit mark.
		if (!approver || membersAsked || !session.s.localRecent) return;
		membersAsked = true;
		api
			.get<{ users: Member[] }>('/users')
			.then((r) => (members = Object.fromEntries(r.users.map((u) => [u.id, u]))))
			.catch(() => {});
	});
	let askSettings = $state<Record<string, unknown>>({});
	// The member the poster size and the panel are remembered for.
	let myId = $state<number | undefined>();
	const defaultRuleOf = (a: Ask) => {
		const k = a.kind === 'show' ? 'show_pick_rules_id' : a.kind === 'artist' ? 'music_pick_rules_id' : 'pick_rules_id';
		const id = Number(askSettings[k] ?? 0);
		return id ? (rules.find((r) => r.id === id)?.name ?? '') : '';
	};
	const foldersOf = (a: Ask) => {
		const k = a.kind === 'show' ? 'show' : a.kind === 'artist' ? 'music' : 'movie';
		return folders.filter((f) => f.kind === k);
	};
	const rulesOf = (a: Ask) => {
		const k = a.kind === 'show' ? 'show' : a.kind === 'artist' ? 'music' : 'movie';
		return rules.filter((r) => !r.kinds.length || r.kinds.includes(k));
	};
	const people = $derived(askers(asks));
	const ctx: ApprovalsCtx = {
		get shown() { return shown; },
		live: (a) => liveFacts(a.title_id ? titleLive.get(a.title_id) : undefined),
		get viewed() { return viewed; },
		get tab() { return tab; },
		get counts() { return counts; },
		setTab: (t) => ((tab = t), (chosen = new Set())),
		get approverTab() { return apTab; },
		get approverCounts() { return apCounts; },
		setApproverTab: (t) => ((apTab = t), (chosen = new Set())),
		get approver() { return approver; },
		get choosable() { return choosable; },
		isChosen: (id) => chosen.has(id),
		toggleChosen,
		chooseAll: (list) => {
			const all = list.length > 0 && list.every((a) => chosen.has(a.id));
			const next = new Set(chosen);
			for (const a of list) {
				if (all) next.delete(a.id);
				else if (a.state === 'asked') next.add(a.id);
			}
			chosen = next;
		},
		isPending: (id) => !!pending[id],
		approve: (list) => act(list, 'approve'),
		approveWith: (a, opt) => act([a], 'approve', opt.note ?? '', opt),
		turnDown: (list) => askDecline(list),
		turnDownWith: (a, note) => act([a], 'turn_down', note),
		withOptions: (a) => open(a, 'approve'),
		review: () => {},
		takeBack: (a) => askTakeBack(a),
		decideNow,
		foldersFor: foldersOf,
		rulesFor: rulesOf,
		who: (a) => {
			const p = a.people[0] ?? a.asked_by;
			const m = members[p.id];
			const kid = kidLabel(m?.max_rating);
			// The Kid badge says it already: no "(Kid)" in the name beside it.
			const name = plain(p.name);
			return { id: p.id, name: kid ? name.replace(/\s*\(kid\)\s*$/i, '') || name : name, avatar: m?.avatar ?? null, kid };
		},
		what,
		year,
		askedBy,
		decided,
		pill: askPill,
		approvedLine,
		href: askHref,
		details: askDetails,
		want: wantDetails,
		info: askInfo,
		episodes: seasonEpisodes,
		defaultRule: defaultRuleOf,
		get kind() { return kind; },
		setKind: (k) => ((kind = k), (chosen = new Set())),
		get person() { return person; },
		setPerson: (id) => ((person = id), (chosen = new Set())),
		get people() { return people; },
		get me() { return myId; }
	};
</script>

<svelte:head><title>Requests · HolyArrs</title></svelte:head>

<div class="ap-top" class:compact={approver}>
<h1 class="sr-only">Requests</h1>
{#if session.can('ask') && !approver}
	<div class="ask-actions">
			<Button icon="film" variant="primary" href={to(`/search`)}><span class="ask-d">Ask for a movie</span><span class="ask-m"><span class="sr-only">Ask for a </span>Movie</span></Button>
			<Button icon="tv" href={to(`/search?kind=show`)}><span class="ask-d">Ask for a show</span><span class="ask-m"><span class="sr-only">Ask for a </span>Show</span></Button>
			<Button icon="music" href={to(`/search?kind=music`)}><span class="ask-d">Ask for music</span><span class="ask-m"><span class="sr-only">Ask for </span>Music</span></Button>
		</div>
{/if}

{#if limitLines(limit, onKinds).length}
	<ul class="limit" data-testid="ask-limit">
		{#each limitLines(limit, onKinds) as l (l.kind)}<li>{l.text}</li>{/each}
	</ul>
{/if}

{#if (bothKinds || asks.length) && !approver}
	<div class="toolbar">
		{#if asks.length}
			<div class="chips tabs" role="group" aria-label="Show" data-testid="ask-tabs">
				{#each tabs as t (t.tab)}
					<Chip label={t.label} count={t.tab === 'waiting' && counts.waiting ? counts.waiting : undefined} selected={tab === t.tab} onclick={() => ((tab = t.tab), (chosen = new Set()))} />
				{/each}
			</div>
		{/if}
		{#if bothKinds}
			<div class="chips" role="group" aria-label="Kind">
				<Chip label="Everything" selected={kind === 'all'} onclick={() => (kind = 'all')} />
				<Chip label="Movies" selected={kind === 'movie'} onclick={() => (kind = 'movie')} />
				<Chip label="TV shows" selected={kind === 'show'} onclick={() => (kind = 'show')} />
				<Chip label="Music" selected={kind === 'artist'} onclick={() => (kind = 'artist')} />
			</div>
		{/if}
		{#if asks.length}<p class="count">{shown.length} {shown.length === 1 ? 'request' : 'requests'}</p>{/if}
	</div>
{/if}
</div>

{#snippet bulkBar()}
			{#if choosable}
				<div class="bulk" class:on={chosenAsks.length > 0} data-testid="ask-bulk">
					<label class="tick-all">
						<input
							type="checkbox"
							checked={chosenAsks.length === shown.length}
							indeterminate={chosenAsks.length > 0 && chosenAsks.length < shown.length}
							onchange={(e) => (chosen = e.currentTarget.checked ? new Set(shown.map((a) => a.id)) : new Set())}
						/>
						{chosenAsks.length ? `${chosenAsks.length} chosen` : 'Choose all'}
					</label>
					{#if chosenAsks.length}
						<Button size="sm" variant="primary" icon="check" type="button" onclick={() => act(chosenAsks, 'approve')}>Approve {chosenAsks.length}</Button>
						<Button size="sm" icon="x" type="button" onclick={() => askDecline(chosenAsks)}>Decline {chosenAsks.length}</Button>
					{/if}
				</div>
			{/if}
{/snippet}

{#if !approver}
<div class="workspace">
	<div class="main-col">
		{#if error && !asks.length}
			<LoadError hasData={false} message="Couldn’t load your requests. {error}" onretry={load} />
		{:else}
		<PageLoader loading={!loaded} label="Loading your requests…">
		{#if asks.length === 0}
			<EmptyState
				icon="wish"
				title="Nothing on the Requests page yet"
				text={session.can('ask')
					? "Find something in Search and press Ask. It shows up here, and you'll hear when it's approved and when it's ready."
					: "Your account can't ask for titles yet. Check with the owner."}
				action={{ label: 'Search', href: to('/search') }}
			/>
		{:else if shown.length === 0}
			<EmptyState
				compact
				icon="check"
				title={tab === 'waiting' ? 'You’re all caught up.' : tab === 'failed' ? 'Nothing failed.' : `Nothing ${askTabs.find((t) => t.tab === tab)?.label.toLowerCase()} yet.`}
			/>
		{:else}
			{@render bulkBar()}
			<ul class="asks" aria-label="Requests">
				{#each shown as a, i (a.id)}
					<li class="ask rise" animate:glide class:selected={selected?.id === a.id} class:chosen={chosen.has(a.id)} class:waiting={!!pending[a.id]} style="--i: {i}" data-testid="ask" data-ask-id={a.id}>
						{#if a.poster}<div class="strip" aria-hidden="true" style:background-image={`url("${a.poster}")`}></div>{/if}
						{#if choosable}
							<input class="tick" type="checkbox" aria-label="Choose {a.name}" checked={chosen.has(a.id)} onchange={() => toggleChosen(a.id)} />
						{/if}
						<div class="art"><Poster src={a.poster} name={a.name} /></div>
						<div class="body">
							<div class="top">
								<h2 class="name">
									{#if a.title_id}<a href={to(`/library/${a.title_id}`)}>{a.name}<span class="muted">{year(a)}</span></a>
									{:else}{a.name}<span class="muted">{year(a)}</span>{/if}
								</h2>
								<StatusPill kind={askPill(a).kind} label={askPill(a).label} testid="ask-state" />
								{#if a.kind === 'show'}<span class="kind" data-testid="ask-kind">Show</span>{/if}
								{#if a.kind === 'artist'}<span class="kind" data-testid="ask-kind">Music</span>{/if}
							</div>
							{#if what(a)}<p class="line" data-testid={a.kind === 'artist' ? 'ask-albums' : 'ask-seasons'}>Asked for {what(a)}</p>{/if}
							<p class="line muted" data-testid="asked-by">
								{askedBy(a)} · <time datetime={a.asked_at} title={dateTime(a.asked_at)}>{ago(a.asked_at)}</time>
							</p>
							{#if decided(a)}
								<p class="line muted" data-testid="decided-by">
									{decided(a)}{#if a.decided_at}{' '}· <time datetime={a.decided_at} title={dateTime(a.decided_at)}>{ago(a.decided_at)}</time>{/if}
								</p>
							{/if}
							{#if a.note}<p class="line note">“{a.note}”</p>{/if}
							{#if a.state === 'approved'}<p class="line muted">{approvedLine(a)}</p>{#if a.title_id && a.title_id > 0}<SearchOutcomeLine titleId={a.title_id} />{/if}{/if}
							<div class="actions">
								{#if pending[a.id]}<span class="muted pending">Sending…</span>{/if}
								<button
									class="inspect"
									class:on={selected?.id === a.id}
									type="button"
									aria-label="Review {a.name}"
									aria-pressed={selected?.id === a.id}
									aria-expanded={compactReview ? reviewOpen && selected?.id === a.id : undefined}
									aria-controls="requests-review"
									onclick={(event) => void selectAsk(a.id, event.currentTarget)}>Review</button
								>
								{#if a.mine && a.state === 'asked'}
									<span class="row-menu" data-testid="ask-menu">
										<Menu
											label="More for {a.name}"
											width={220}
											items={[{ label: 'Take back', icon: 'trash', destructive: true, disabled: takingBack.has(a.id), onselect: () => askTakeBack(a) }]}
										/>
									</span>
								{/if}
							</div>
						</div>
					</li>
				{/each}
			</ul>
		{/if}
		</PageLoader>
		{/if}
	</div>

	<div class="side-col">
		{#if loaded && !error && selected}
			<aside id="requests-review" class="review" class:mobile-open={reviewOpen} aria-label="Ask details">
				<div class="review-close"><Button variant="plain" size="sm" icon="x" type="button" aria-label="Close review" onclick={() => void closeReview()}>Close</Button></div>
				<div class="review-head">
					<div class="review-art"><Poster src={selected.poster} name={selected.name} /></div>
					<div>
						<h2 class="review-title" bind:this={reviewHeading} tabindex="-1">{selected.name}<span class="muted">{year(selected)}</span></h2>
						<StatusPill kind={askPill(selected).kind} label={askPill(selected).label} />
					</div>
				</div>
				<dl class="review-facts">
					<dt>Asked by</dt><dd>{askedBy(selected).replace(/^Asked by /, '')}</dd>
					<dt>Asked</dt><dd>{ago(selected.asked_at)}</dd>
					{#if what(selected)}<dt>For</dt><dd>{what(selected)}</dd>{/if}
					{#if decided(selected)}<dt>Decision</dt><dd>{decided(selected)}</dd>{/if}
				</dl>
				{#if selected.note}<p class="review-note">“{selected.note}”</p>{/if}
				{#if selected.state === 'approved'}<p class="line muted">{approvedLine(selected)}</p>{#if selected.title_id && selected.title_id > 0}<SearchOutcomeLine titleId={selected.title_id} />{/if}{/if}
			</aside>
		{/if}

		{@render messages()}
	</div>
</div>
{:else}
	<div class="variant">
		{#if error && !asks.length}
			<LoadError hasData={false} message="Couldn’t load your requests. {error}" onretry={load} />
		{:else}
			<PageLoader loading={!loaded} label="Loading your requests…">
				{#if asks.length === 0}
					<EmptyState
						icon="wish"
						title="Nothing on the Requests page yet"
						text={session.can('ask')
							? "Find something in Search and press Ask. It shows up here, and you'll hear when it's approved and when it's ready."
							: "Your account can't ask for titles yet. Check with the owner."}
						action={{ label: 'Search', href: to('/search') }}
					/>
				{:else}
					<!-- Approvers: Cards (every width) or the Stage (wide screens); design-notes/approvals-stage.md. -->
					<ApprovalsDesktop {ctx} />
				{/if}
			</PageLoader>
		{/if}
			</div>
{/if}

{#snippet messages()}
		{#if inbox.messages.length}
			<section class="messages" aria-labelledby="h-msg">
				<div class="msg-head">
					<h2 id="h-msg">Messages{#if inbox.unread}{' '}<span class="badge">{inbox.unread} new</span>{/if}</h2>
					{#if inbox.unread}<Button variant="plain" size="sm" type="button" disabled={markingRead} aria-busy={markingRead} onclick={() => void readAllMessages()}>Mark all read</Button>{/if}
				</div>
				<details>
					<summary>Show the latest {Math.min(inbox.messages.length, 5)}</summary>
					<ul class="msg-list">
						{#each inbox.messages.slice(0, 5) as m (m.id)}
							<li class:unread={!m.read_at} data-testid="message">
								<StatusPill kind={msgKind(m.kind)} label={m.kind === 'ready' ? 'Ready' : m.kind === 'turned_down' ? 'Declined' : 'Approved'} />
								<span class="msg-text">{plain(m.text)}</span>
								<time class="muted" datetime={m.created_at} title={dateTime(m.created_at)}>{ago(m.created_at)}</time>
							</li>
						{/each}
					</ul>
				</details>
			</section>
		{/if}
{/snippet}


<Sheet
	bind:open={decideOpen}
	title={picked ? `${mode === 'approve' ? 'Approve' : 'Decline'} ${picked.name}${year(picked)}` : 'Approve or decline'}
	description={picked ? `${askedBy(picked)}${what(picked) ? `, for ${what(picked)}` : ''}.` : undefined}
>
	{#if picked}
		<form class="decide" onsubmit={decide}>
			{#if mode === 'approve' && kindFolders.length > 0}
				<div class="field">
					<label for="ap-folder">Library folder</label>
					<select id="ap-folder" class="input" bind:value={folderId}>
						<option value={0}>Default</option>
						{#each kindFolders as f (f.id)}
							<option value={f.id}>{f.label || f.path} ({f.path})</option>
						{/each}
					</select>
				</div>
				<div class="field">
					<label for="ap-rules">Quality</label>
					<select id="ap-rules" class="input" bind:value={rulesId}>
						<option value={0}>Default</option>
						{#each kindRules as r (r.id)}
							<option value={r.id}>{r.name}</option>
						{/each}
					</select>
				</div>
			{/if}
			<div class="field">
				<label for="ap-note">{mode === 'approve' ? 'Note (optional)' : 'Reason (the person who asked sees it)'}</label>
				<textarea id="ap-note" class="input" rows="3" maxlength="500" bind:value={note}></textarea>
			</div>
			{#if mode === 'approve'}
				<p class="hint">
					{picked.kind === 'show'
						? `Approving adds the show to the Library and keeps ${what(picked)} up to date. HolyArrs looks for episodes soon and again each night.`
						: picked.kind === 'artist'
							? `Approving adds the artist to the Library with ${what(picked)} kept up to date. HolyArrs can't look for music downloads yet.`
							: 'Approving adds the movie to the Library and starts looking for it.'}
				</p>
			{/if}
			{#if dialogError}<p class="form-error" role="alert">{dialogError}</p>{/if}
			<div class="decide-actions">
				<Button variant="plain" type="button" onclick={() => (decideOpen = false)}>Cancel</Button>
				<Button variant={mode === 'approve' ? 'primary' : 'destructive'} type="submit" {busy}>
					{mode === 'approve' ? 'Approve' : 'Decline'}
				</Button>
			</div>
		</form>
	{/if}
</Sheet>

<Sheet
	bind:open={declineOpen}
	size="sm"
	title={declineList.length === 1 ? `Decline ${declineList[0].name}${year(declineList[0])}?` : `Decline ${declineList.length} asks?`}
	description="You can undo this for a few seconds."
>
	<form class="decide" onsubmit={confirmDecline}>
		<div class="field">
			<label for="dc-why">Reason (optional, shown to {declineList.length === 1 ? ctx.who(declineList[0]).name : 'the people who asked'})</label>
			<textarea id="dc-why" class="input" rows="3" maxlength="500" placeholder="For example: we already have it" bind:this={whyBox} bind:value={declineWhy} data-testid="decline-why"></textarea>
			<div class="why-chips" role="group" aria-label="Quick reasons">
				{#each reasons as why (why)}
					<button type="button" class="why-chip" aria-pressed={hasWhy(why)} onclick={() => tapWhy(why)}>{why}</button>
				{/each}
			</div>
		</div>
		<div class="decide-actions">
			<Button variant="plain" type="button" onclick={() => (declineOpen = false)}>Cancel</Button>
			<Button variant="destructive" type="submit">Decline</Button>
		</div>
	</form>
</Sheet>

<Sheet bind:open={takeBackOpen} onclose={takeBackClosed} size="sm" title={takeBackAsk ? `Take back ${takeBackAsk.name}${year(takeBackAsk)}?` : 'Take back'} description="It leaves Requests. You can ask again later.">
	{#if takeBackAsk}
		{#if takeBackError}<p class="form-error" role="alert" data-testid="take-back-error">{takeBackError}</p>{/if}
		<div class="decide-actions">
			<Button variant="plain" type="button" onclick={() => (takeBackOpen = false)}>Keep it</Button>
			<Button variant="destructive" type="button" busy={takingBack.has(takeBackAsk.id)} onclick={() => takeBackAsk && void takeBack(takeBackAsk)}>Take back</Button>
		</div>
	{/if}
</Sheet>

<style>
	.ask-actions { display: flex; flex-wrap: wrap; gap: 8px; margin: 0 0 16px; }
	.limit { margin: 0 0 16px; padding: 12px 16px; border-radius: var(--r-card); background: var(--surface-2); color: var(--text-muted); font: var(--t-callout); }
	.toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 10px 16px; margin-bottom: 16px; }
	.chips { display: flex; flex-wrap: wrap; gap: 8px; }
	.count { margin-left: auto; color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.workspace { display: grid; grid-template-columns: minmax(0, 1fr) minmax(300px, 360px); gap: 24px; align-items: start; }
	.main-col, .side-col { min-width: 0; }
	.side-col { position: sticky; top: calc(var(--topbar) + 16px); display: flex; flex-direction: column; gap: 16px; }
	.asks { border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); overflow: hidden; }
	.ask { position: relative; isolation: isolate; display: flex; align-items: flex-start; gap: 14px; padding: 14px 16px; transition: background var(--dur-fast) var(--ease-out), opacity var(--dur-fast) var(--ease-out); }
	.strip { position: absolute; inset: 0; z-index: -1; background-size: cover; background-position: center 30%; filter: blur(18px) saturate(1.2); opacity: 0.22; mask-image: linear-gradient(90deg, #000 0%, transparent 70%); pointer-events: none; }
	.ask.chosen { background: var(--accent-soft); }
	.ask.waiting { opacity: 0.7; }
	.tick, .tick-all input { width: 18px; height: 18px; margin-top: 4px; flex: none; accent-color: var(--accent-fill); }
	.bulk { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; min-height: 44px; margin-bottom: 10px; padding: 6px 12px; border-radius: 999px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.bulk.on { background: var(--surface-2); }
	.tick-all { display: inline-flex; align-items: center; gap: 8px; margin-right: auto; font: var(--t-subhead); color: var(--text-muted); }
	.tick-all input { margin-top: 0; }
	.pending { font: var(--t-footnote); }
	.ask + .ask { border-top: 1px solid var(--line); }
	.ask.selected { background: var(--surface-2); }
	.art { width: 52px; flex: none; border-radius: 8px; overflow: hidden; align-self: flex-start; }
	.body { min-width: 0; flex: 1; display: flex; flex-direction: column; gap: 3px; }
	.top { display: flex; align-items: center; flex-wrap: wrap; gap: 6px 10px; }
	.name { font: var(--t-headline); overflow-wrap: anywhere; }
	.name a:hover { text-decoration: underline; text-underline-offset: 3px; }
	.name .muted, .review-title .muted { margin-left: 4px; font-weight: 400; }
	.kind { padding: 2px 8px; border-radius: var(--r-sm); background: var(--surface-2); color: var(--text-muted); font: var(--t-footnote); }
	.line { font: var(--t-callout); overflow-wrap: anywhere; }
	.muted { color: var(--text-muted); }
	.note { color: var(--text); }
	.actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin-top: 8px; }
	.inspect { min-height: 30px; padding: 0 12px; border-radius: 8px; color: var(--accent-text); font: var(--t-subhead); font-weight: 600; transition: background var(--dur-fast) var(--ease-out); }
	.row-menu { margin-left: auto; }
	.inspect:hover { background: var(--accent-soft); }
	.inspect.on { background: var(--surface-3); color: var(--text); }
	.inspect:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.review, .messages { border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); padding: 16px 18px; }
	.review-close { display: none; }
	.review-head { display: flex; gap: 14px; align-items: flex-start; }
	.review-art { width: 72px; flex: none; border-radius: 8px; overflow: hidden; }
	.review-title { font: var(--t-title3); margin-bottom: 6px; overflow-wrap: anywhere; scroll-margin-top: calc(var(--topbar) + 16px); }
	.review-title:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 6px; }
	.review-facts { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: 6px 12px; margin-top: 14px; padding-top: 12px; border-top: 1px solid var(--line); font: var(--t-callout); }
	.review-facts dt { color: var(--text-muted); }
	.review-facts dd { text-align: right; overflow-wrap: anywhere; }
	.review-note { margin-top: 12px; padding: 10px 12px; border-radius: 10px; background: var(--surface-2); font: var(--t-callout); overflow-wrap: anywhere; }
	.msg-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
	.msg-head h2 { font: var(--t-headline); white-space: nowrap; display: flex; align-items: center; gap: 8px; }
	.badge { padding: 2px 8px; border-radius: var(--r-pill); background: var(--accent-fill); color: var(--text-on-red); font: var(--t-footnote); font-weight: 600; }
	summary { min-height: 36px; display: flex; align-items: center; color: var(--accent-text); font: var(--t-subhead); font-weight: 600; cursor: pointer; border-radius: 6px; }
	summary:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.msg-list li { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 4px 8px; align-items: center; padding: 10px 0; border-top: 1px solid var(--line); }
	.msg-text { grid-column: 1 / -1; font: var(--t-callout); color: var(--text-muted); overflow-wrap: anywhere; }
	.unread .msg-text { color: var(--text); font-weight: 500; }
	.msg-list time { grid-column: 2; grid-row: 1; font: var(--t-footnote); white-space: nowrap; }
	.decide { display: flex; flex-direction: column; gap: 14px; }
	.decide .field + .field { margin-top: 0; }
	textarea.input { height: auto; padding-top: 10px; resize: vertical; }
	.why-chips { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 10px; }
	.why-chip { min-height: 32px; padding: 0 12px; border: 1px solid var(--line); border-radius: 999px; background: var(--surface-2); color: var(--text); font: var(--t-footnote); font-weight: 600; cursor: pointer; transition: background var(--dur-fast) var(--ease-out), border-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out); }
	.why-chip:hover { background: var(--surface-3); }
	.why-chip[aria-pressed='true'] { background: var(--text); border-color: var(--text); color: var(--bg); }
	.why-chip:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.decide-actions { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: 8px; margin-top: 6px; }
	@media (pointer: coarse) { .inspect, summary { min-height: 44px; } }
	.variant { min-width: 0; }
	.ask-m { display: none; }
	/* The layouts' phone design: one row of Ask buttons and sideways chip rows. */
	@media (max-width: 767px) {
		.compact .ask-d { display: none; }
		.compact .ask-m { display: inline; }
		.compact :global(.ui-head .actions) { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); width: 100%; gap: 6px; }
		.compact :global(.ui-head .actions .ui-btn) { min-width: 0; padding: 0 10px; }
		.compact .toolbar { gap: 8px; }
		.compact .chips { flex-wrap: nowrap; width: calc(100% + 2 * var(--gutter)); margin: 0 calc(-1 * var(--gutter)); padding: 0 var(--gutter); overflow-x: auto; scrollbar-width: none; }
		.compact .chips::-webkit-scrollbar { display: none; }
		.compact .chips > :global(*) { flex: none; }
	}
	@media (max-width: 1023px) {
		.workspace { grid-template-columns: minmax(0, 1fr); }
		.side-col { position: static; }
	}
	@media (max-width: 600px) {
		.review:not(.mobile-open) { display: none; }
		.review-close { display: flex; justify-content: flex-end; margin: -6px -8px 6px 0; }
		.count { order: -1; width: 100%; margin-left: 0; }
		.ask { padding: 12px; gap: 12px; }
		.art { width: 44px; }
		.actions > :global(*):not(.row-menu) { flex: 1 1 auto; }
	}
</style>
