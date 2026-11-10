<script lang="ts">
	import { createAskChoices } from '#lib/requests/index.ts';
	import { createDeclineDialog } from '#lib/requests/index.ts';
	import { createAskLive } from '#lib/requests/index.ts';
	import { createTakeBack } from '#lib/requests/index.ts';
	import { createAskCards } from '#lib/requests/index.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Menu from '#lib/ui/Menu.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { SearchOutcomeLine, approverCounts as approverCountsOf, approverSort, approverTab as approverTabOf, askTab, askTabs, firstTab, limitLines, limitKinds, tabCounts, type ApproverTab, type AskTab, markRead, listReports, liveFacts, stepFacts, askHref, type BoardCtx, type DecideOptions, type Member, artistPhoto, askDetails, askInfo, seasonEpisodes, wantDetails, askedByOne, askers, RequestsBoard } from '#lib/requests/index.ts';
	import { loadProfile, kidLabel, watchlistLine } from '#lib/accounts/index.ts';
	import { partsNow } from '#lib/mediafiles/index.ts';
	import type { AskLimit } from '#lib/requests/index.ts';
	import { goto } from '$app/navigation';
	import { onDestroy, onMount, tick, untrack } from 'svelte';
	import { proxied } from '#lib/core/tmdb-image.ts';
	import { inbox, refreshInbox } from '#lib/core/inbox.svelte.ts';
	import { glide } from '#lib/core/transitions.ts';
	import { to } from '#lib/core/paths.ts';
	import { api, ApiError, message } from '#lib/core/api.ts';
	import { ago, dateTime } from '#lib/core/format.ts';
	import { live } from '#lib/core/live.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import type { Ask, LibraryFolder, PickRulesView } from '#lib/core/types.ts';

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

	// Asked by: one person's asks (0 is everyone's), for the Inbox filter.
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
	// Members see Failed only when one of their asks failed (else it would hide there unseen).
	const tabs = $derived(askTabs.filter((t) => approver || t.tab !== 'failed' || counts.failed > 0));
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
			const name = a?.name ?? t('requests.page.theRequest');
			if (err instanceof ApiError && err.status === 409 && /already decided/.test(String(err.body.error ?? ''))) {
				// Someone else answered it first (two approvers at once): show their answer, not "Try again". The one ask is read on its own, so a list load started meanwhile cannot drop it.
				const now = await api.get<Ask>(`/asks/${id}`).catch(() => undefined);
				if (now) {
					const i = asks.findIndex((x) => x.id === id);
					if (i >= 0) asks[i] = now;
				}
				const declined = now?.state === 'turned_down';
				const by = now && (declined || !now.auto_approved) ? askCards.decidedBy(now) : '';
				toasts.push(by ? t(declined ? 'requests.page.alreadyDeclined' : 'requests.page.alreadyApproved', { name }) : t('requests.page.alreadyAnswered', { name }), 'info', by ? { body: t('requests.board.by', { name: by }) } : {});
				return;
			}
			toasts.push(t(p.mode === 'approve' ? 'requests.page.couldNotApprove' : 'requests.page.couldNotDecline', { name }), 'fail', { body: message(err), action: { label: t('requests.board.requests'), run: () => void goto(to(`/requests?ask=${id}`)) } });
		} finally {
			delete pending[id];
		}
	}

	/** Decides a set of asks after an Undo window; the toast's Undo cancels them all. */
	function act(list: Ask[], mode: 'approve' | 'turn_down', why = '', opt?: DecideOptions) {
		// Waiting asks, and (to decline only) approved ones whose title never got added (Failed).
		const open = (a: Ask) => a.state === 'asked' || (mode === 'turn_down' && a.state === 'approved' && !a.title_id);
		const ids = list.filter((a) => open(a) && !pending[a.id]).map((a) => a.id);
		if (!ids.length) {
			if (list.length && !list.some((a) => pending[a.id])) toasts.push(t(mode === 'approve' ? 'requests.page.couldNotApprove' : 'requests.page.couldNotDecline', { name: list.length === 1 ? list[0].name : t('requests.page.nRequests', { n: list.length }) }), 'fail', { body: t('requests.page.answeredAlready') });
			return;
		}
		for (const id of ids) pending[id] = { mode, note: why, rulesId: opt?.rulesId ?? 0, langs: opt?.langs ?? [], seasons: opt?.seasons, episodes: opt?.episodes, albums: opt?.albums, songs: opt?.songs, timer: setTimeout(() => void commit(id), UNDO_MS) };
		chosen = new Set();
		// Words follow the asks actually decided (some in the list may be answered already).
		const one = ids.length === 1 ? (list.find((a) => a.id === ids[0])?.name ?? t('requests.page.theRequest')) : t('requests.page.nRequests', { n: ids.length });
		const toastId = toasts.push(t(mode === 'approve' ? 'requests.page.approvedToast' : 'requests.page.declinedToast', { one }), 'ok', {
			ms: UNDO_MS,
			action: {
				label: t('requests.page.undo'),
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
	// Leaving the page sends what is still waiting out its Undo time. Its Undo cannot work after that, so the toast goes too.
	onDestroy(() =>
		Object.keys(pending).forEach((id) => {
			const t = undoToast.get(Number(id));
			if (t) toasts.dismiss(t);
			void commit(Number(id));
		})
	);

	// Decline asks for an optional reason the asker sees.
	const declineFlowCtx = {
		act,
	};
	const declineFlow = createDeclineDialog(declineFlowCtx);
	let selectedId = $state(0);
	let takingBack = $state<Set<number>>(new Set());
	let myReports = $state(0);

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
	/** Reads the list; quiet (a poll) keeps a failed read to itself. */
	async function load(quiet = false) {
		const gen = ++loadGen;
		try {
			const r = await api.get<{ asks: Ask[] }>('/asks');
			if (gen !== loadGen) return;
			asks = r.asks;
			error = '';
		} catch (e) {
			// A background refresh that fails keeps the list on screen and says so once; a poll says nothing and tries again on its next turn.
			if (quiet && asks.length) return;
			if (asks.length) toasts.push(t('requests.page.refreshFailed'), 'fail', { body: message(e) });
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
			// ?ask=<id> (History's "Go to Requests"): a member's cards open on that ask, ringed and scrolled to. Approvers get its window from the Cards (lib/requests/RequestsBoard.svelte).
			const want = Number(new URLSearchParams(location.search).get('ask')) || 0;
			const a = !approver && want ? asks.find((x) => x.id === want) : undefined;
			if (!a) return;
			tab = askTab(a);
			selectedId = a.id;
			void tick().then(() => document.querySelector(`[data-ask-id="${a.id}"]`)?.scrollIntoView({ block: 'center' }));
		});
		void refreshInbox();
		// People who do not manage the Library get a way back to the reports they sent, once they have sent one.
		if (!session.can('manage_library')) void listReports('all', { limit: 1 }).then((r) => (myReports = r.reports.length), () => {});
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
				.then((r) => (askChoices.askSettings = r))
				.catch(() => {});
		}
		void askLive.loadLive();
		const offs = [
			live.on('ask', () => void load()),
			// A slot moving (a search found nothing, an import waits on a person) changes an ask's step.
			live.on('slot', () => {
				if (askLive.slotSoon) return;
				askLive.slotSoon = setTimeout(() => {
					askLive.slotSoon = undefined;
					void load();
				}, 2000);
			}),
			live.on('ask_removed', () => void load()),
			...['download', 'slot', 'import', 'resync'].map((t) => live.on(t, () => void askLive.loadLive())),
			// Members get their own downloads as dl patches: read again when one changes.
			live.on('dl', askLive.onDlPatch)
		];
		return () => offs.forEach((off) => off());
	});

	// A dl patch with rows in it means a download moved; one read for a burst of them.
	const askLiveCtx = {
		get approver() { return approver; },
		load,
	};
	const askLive = createAskLive(askLiveCtx);

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

	/** Approves (with a Library folder and Pick rules) or turns down one ask straight away; '' when done, else the error. */
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
			toasts.push(t(mode === 'approve' ? 'requests.page.approvedToast' : 'requests.page.declinedToast', { one: r.ask.name }), 'ok');
			void refreshInbox();
			return '';
		} catch (err) {
			return message(err);
		}
	}

	// Take back asks first in a confirm sheet.
	const takeBackFlowCtx = {
		get takingBack() { return takingBack; },
		set takingBack(v) { takingBack = v; },
		get shown() { return shown; },
		load,
	};
	const takeBackFlow = createTakeBack(takeBackFlowCtx);
	let fresh = $state(new Set<number>());
	let reading = false;
	$effect(() => {
		if (!inbox.unread || reading) return;
		const unread = inbox.messages.filter((m) => !m.read_at);
		if (!unread.length) return;
		reading = true;
		untrack(() => (fresh = new Set([...fresh, ...unread.flatMap((m) => (m.ask_id ? [m.ask_id] : []))])));
		// Their alerts come from the bell (bell-feeds), on every page.
		void markRead()
			.catch(() => {})
			.finally(() => (reading = false));
	});

	// The member cards (people who cannot approve) speak the approvers' card language: the title's wide art (its backdrop, an artist's photo, else the poster blurred), then a glass panel with the status and its lines.
	$effect(() => {
		if (!approver) for (const a of shown.slice(0, 16)) wantDetails(a);
	});
	const askCardsCtx = {
		get approver() { return approver; },
		get titleLive() { return askLive.titleLive; },
		get asks() { return asks; },
		load,
		loadLive: askLive.loadLive,
	};
	const askCards = createAskCards(askCardsCtx);
	const askChoicesCtx = {
		get rules() { return rules; },
		get folders() { return folders; },
	};
	const askChoices = createAskChoices(askChoicesCtx);
	$effect(() => {
		// The people list is an admin route: when it is refused, the rows show initials and no age-limit mark.
		if (!approver || askChoices.membersAsked) return;
		askChoices.membersAsked = true;
		api
			.get<{ users: Member[] }>('/users')
			.then((r) => (askChoices.members = Object.fromEntries(r.users.map((u) => [u.id, u]))))
			.catch(() => {});
	});
	// The member the poster size and the panel are remembered for.
	let myId = $state<number | undefined>();
	const people = $derived(askers(asks));
	const ctx: BoardCtx = {
		get shown() { return shown; },
		live: (a) => {
			if (a.problem) return { phase: 'failed', reason: a.problem };
			// The page's own live search or download first, else the server's step.
			const f = liveFacts(a.title_id ? askLive.titleLive.get(a.title_id) : undefined);
			return f.phase ? f : { ...f, ...stepFacts(a.step) };
		},
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
		turnDown: (list) => declineFlow.askDecline(list),
		turnDownWith: (a, note) => act([a], 'turn_down', note),
		withOptions: (a) => open(a, 'approve'),
		review: () => {},
		takeBack: (a) => takeBackFlow.askTakeBack(a),
		decideNow,
		foldersFor: askChoices.foldersOf,
		rulesFor: askChoices.rulesOf,
		who: (a) => {
			const p = a.people[0] ?? a.asked_by;
			const m = askChoices.members[p.id];
			const kid = kidLabel(m?.max_rating);
			// The Kid badge says it already: no "(Kid)" in the name beside it.
			const name = askCards.plain(p.name);
			return { id: p.id, name: kid ? name.replace(/\s*\(kid\)\s*$/i, '') || name : name, avatar: m?.avatar ?? null, kid };
		},
		what: askCards.what,
		year: askCards.year,
		askedBy: askCards.askedBy,
		decided: askCards.decided,
		decidedBy: askCards.decidedBy,
		alsoDeclined: askCards.alsoDeclined,
		pill: askCards.askPill,
		approvedLine: askCards.approvedLine,
		href: askHref,
		details: askDetails,
		want: wantDetails,
		info: askInfo,
		episodes: seasonEpisodes,
		defaultRule: askChoices.defaultRuleOf,
		get kind() { return kind; },
		setKind: (k) => ((kind = k), (chosen = new Set())),
		get person() { return person; },
		setPerson: (id) => ((person = id), (chosen = new Set())),
		get people() { return people; },
		get me() { return myId; },
		isNew: (id) => fresh.has(id),
		get reports() { return session.can('manage_library') ? inbox.reports : -1; }
	};
</script>

<svelte:head><title>{t('requests.page.title')}</title></svelte:head>

<div class="ap-top" class:mem={!approver}>
<h1 class="sr-only">{t('requests.board.requests')}</h1>
<!-- Reports: approvers get it in the cards' toolbar; this link covers no toolbar (no asks yet, or no approving). -->
{#if session.can('manage_library') && (!approver || (loaded && asks.length === 0))}<a class="rp-link" href={to('/requests/reports')} data-testid="reports-link">{t('requests.reports.title')}{#if inbox.reports}{' '}({inbox.reports}){/if}<span class="sr-only"> {t('requests.page.ofProblems')}</span></a>{/if}
{#if (session.can('ask') && !approver) || myReports > 0}
	<div class="ask-actions">
		{#if session.can('ask') && !approver}
			<!-- Wide screens: one button per kind. Phones: one button to Search, where the kinds are a tap away. -->
			<a class="tb ink ask-k" href={to(`/search`)}><Icon name="film" size={16} />{t('requests.page.askMovie')}</a>
			<a class="tb ask-k" href={to(`/search?kind=show`)}><Icon name="tv" size={16} />{t('requests.page.askShow')}</a>
			<a class="tb ask-k" href={to(`/search?kind=music`)}><Icon name="music" size={16} />{t('requests.page.askMusic')}</a>
			<a class="tb ink ask-one" href={to(`/search`)}><Icon name="plus" size={16} />{t('requests.page.askSomething')}</a>
		{/if}
		{#if myReports > 0}<a class="tb ask-rp" href={to('/requests/reports')} data-testid="my-reports-link"><Icon name="alert" size={16} />{t('requests.reports.titleMine')}</a>{/if}
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
			<div class="tabs" role="group" aria-label={t('requests.reports.show')} data-testid="ask-tabs">
				{#each tabs as tb (tb.tab)}
					{@const n = counts[tb.tab] ?? 0}
					<div class="pw" class:on={tab === tb.tab} role="presentation">
						<button type="button" class="tab" class:zero={!n} aria-pressed={tab === tb.tab} onclick={() => ((tab = tb.tab), (chosen = new Set()))}>{tb.label}{#if n}<span class="n" class:wait={tb.tab === 'waiting'}>{n}</span>{/if}</button>
					</div>
				{/each}
			</div>
		{/if}
		{#if bothKinds}
			<div class="tabs kinds" role="group" aria-label={t('requests.page.kind')}>
				{#each askCards.kinds as k (k.k)}
					<div class="pw" class:on={kind === k.k} role="presentation"><button type="button" class="tab" aria-pressed={kind === k.k} onclick={() => (kind = k.k)}>{k.label}</button></div>
				{/each}
			</div>
		{/if}
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
						{chosenAsks.length ? t('requests.page.chosen', { n: chosenAsks.length }) : t('requests.page.chooseAll')}
					</label>
					{#if chosenAsks.length}
						<Button size="sm" variant="primary" icon="check" type="button" onclick={() => act(chosenAsks, 'approve')}>{t('requests.page.approveN', { n: chosenAsks.length })}</Button>
						<Button size="sm" icon="x" type="button" onclick={() => declineFlow.askDecline(chosenAsks)}>{t('requests.page.declineN', { n: chosenAsks.length })}</Button>
					{/if}
				</div>
			{/if}
{/snippet}

{#if !approver}
<div class="member">
	{#if error && !asks.length}
		<LoadError hasData={false} message={t('requests.page.loadFailed', { error })} onretry={load} />
	{:else}
	<PageLoader loading={!loaded} label={t('requests.page.loading')}>
	{#if asks.length === 0}
		<EmptyState
			icon="wish"
			title={t('requests.page.emptyTitle')}
			text={session.can('ask')
				? t('requests.page.emptyAsk')
				: t('requests.page.emptyNoAsk')}
			action={{ label: t('requests.page.search'), href: to('/search') }}
		/>
	{:else if shown.length === 0}
		<EmptyState
			compact
			icon="check"
			title={tab === 'waiting' ? t('requests.board.calm.pending.title') : tab === 'failed' ? t('requests.page.nothingFailed') : t('requests.page.nothingTab', { tab: (askTabs.find((x) => x.tab === tab)?.label ?? '').toLowerCase() })}
		/>
	{:else}
		{@render bulkBar()}
		<ul class="cards" aria-label={t('requests.board.requests')}>
			{#each shown as a, i (a.id)}
				{@const wide = proxied(askCards.wideOf(a)) || null}
				{@const photo = a.kind === 'artist' ? proxied(artistPhoto(a)) || null : null}
				<li class="mc rise" animate:glide class:hl={selectedId === a.id} class:waiting={!!pending[a.id]} class:answered={a.state === 'turned_down'} style="--i: {i}" data-testid="ask" data-ask-id={a.id}>
					<div class="ban" class:noart={!wide && !photo && !a.poster}>
						{#if wide}<img class="ban-img" src={wide} alt="" loading="lazy" decoding="async" />{:else if photo}<span class="ban-img" aria-hidden="true" style:background-image={`url("${photo}")`}></span>{:else if a.poster}<span class="ban-img soft" aria-hidden="true" style:background-image={`url("${proxied(a.poster)}")`}></span>{/if}
					</div>
					<div class="top">
						<div class="ban-cap">
							{#if fresh.has(a.id)}<span class="new-mark" data-testid="ask-new">{t('requests.board.new')}</span>{/if}
							<h2 class="ban-t"><a href={to(askHref(a))}>{a.name}</a></h2>
							<span class="ban-f">{#if a.kind === 'show'}<span data-testid="ask-kind">{t('requests.kind.show')}</span>{:else if a.kind === 'artist'}<span data-testid="ask-kind">{t('requests.kind.artist')}</span>{:else}{t('requests.kind.movie')}{/if}{#if a.year}{' · '}{a.year}{/if}</span>
						</div>
					</div>
					<div class="sheet-b">
						<div class="st-row">
							<span class="st-chip st-{askCards.cardTone(a)}"><span class="st-sq"><Icon name={askCards.cardIcon(a)} size={15} /></span><b data-testid="ask-state">{askCards.askPill(a).label}</b></span>
							{#if pending[a.id]}<span class="muted pending">{t('requests.window.sending')}</span>{/if}
							{#if a.mine && a.state === 'asked'}
								<span class="row-menu" data-testid="ask-menu">
									<Menu
										label={t('requests.page.moreFor', { name: a.name })}
										width={220}
										items={[{ label: t('requests.page.takeBack'), icon: 'trash', destructive: true, disabled: takingBack.has(a.id), onselect: () => takeBackFlow.askTakeBack(a) }]}
									/>
								</span>
							{/if}
						</div>
						<div class="lines">
							{#if askCards.what(a)}<p class="line what" data-testid={a.kind === 'artist' ? 'ask-albums' : 'ask-seasons'}>{t('requests.page.askedFor', { what: askCards.what(a) })}</p>{/if}
							<p class="line muted" data-testid="asked-by">
								{askCards.askedBy(a)} · <time datetime={a.asked_at} title={dateTime(a.asked_at)}>{ago(a.asked_at)}</time>
							</p>
							{#if watchlistLine(a.people, session.s.id)}<p class="line muted" data-testid="ask-from-watchlist">{watchlistLine(a.people, session.s.id)}</p>{/if}
							{#if askCards.decided(a)}
								<p class="line muted" data-testid="decided-by">
									{askCards.decided(a)}{#if a.decided_at}{' '}· <time datetime={a.decided_at} title={dateTime(a.decided_at)}>{ago(a.decided_at)}</time>{/if}
								</p>
							{/if}
							{#if a.note}<p class="line note">“{a.note}”</p>{/if}
							{#if a.state === 'approved'}
								{@const why = askCards.whyFailed(a)}
								{#if why}<p class="line rq-fail" data-testid="ask-failed">{why}</p>
								{:else}<p class="line muted">{askCards.approvedLine(a)}</p>
									{#if askCards.stepLine(a)}<p class="line rq-step" data-testid="ask-step">{askCards.stepLine(a)}</p>{/if}
									{#if a.title_id && a.title_id > 0 && !a.step}<SearchOutcomeLine titleId={a.title_id} />{/if}{/if}
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
{:else}
	<div class="variant">
		{#if error && !asks.length}
			<LoadError hasData={false} message={t('requests.page.loadFailed', { error })} onretry={load} />
		{:else}
			<PageLoader loading={!loaded} label={t('requests.page.loading')}>
				{#if asks.length === 0}
					<EmptyState
						icon="wish"
						title={t('requests.page.emptyTitle')}
						text={session.can('ask')
							? t('requests.page.emptyAsk')
							: t('requests.page.emptyNoAsk')}
						action={{ label: t('requests.page.search'), href: to('/search') }}
					/>
				{:else}
					<!-- Approvers: Cards (every width). -->
					<RequestsBoard {ctx} />
				{/if}
			</PageLoader>
		{/if}
			</div>
{/if}

<Sheet
	bind:open={decideOpen}
	title={picked ? t(mode === 'approve' ? 'requests.page.approveTitle' : 'requests.page.declineTitle', { name: `${picked.name}${askCards.year(picked)}` }) : t('requests.page.approveOrDecline')}
	description={picked ? (askCards.what(picked) ? t('requests.page.descFor', { by: askCards.askedBy(picked), what: askCards.what(picked) }) : t('requests.page.desc', { by: askCards.askedBy(picked) })) : undefined}
>
	{#if picked}
		<form class="decide" onsubmit={decide}>
			{#if mode === 'approve' && kindFolders.length > 0}
				<div class="field">
					<label for="ap-folder">{t('requests.add.folder')}</label>
					<select id="ap-folder" class="input" bind:value={folderId}>
						<option value={0}>{t('requests.window.default')}</option>
						{#each kindFolders as f (f.id)}
							<option value={f.id}>{f.label || f.path} ({f.path})</option>
						{/each}
					</select>
				</div>
				<div class="field">
					<label for="ap-rules">{t('requests.window.quality')}</label>
					<select id="ap-rules" class="input" bind:value={rulesId}>
						<option value={0}>{t('requests.window.default')}</option>
						{#each kindRules as r (r.id)}
							<option value={r.id}>{r.name}</option>
						{/each}
					</select>
				</div>
			{/if}
			<div class="field">
				<label for="ap-note">{mode === 'approve' ? t('requests.page.noteOptional') : t('requests.page.reasonSeen')}</label>
				<textarea id="ap-note" class="input" rows="3" maxlength="500" bind:value={note}></textarea>
			</div>
			{#if mode === 'approve'}
				<p class="hint">
					{picked.kind === 'show'
						? t('requests.page.hintShow', { what: askCards.what(picked) })
						: picked.kind === 'artist'
							? t('requests.page.hintArtist', { what: askCards.what(picked) })
							: t('requests.page.hintMovie')}
				</p>
			{/if}
			{#if dialogError}<p class="form-error" role="alert">{dialogError}</p>{/if}
			<div class="decide-actions">
				<Button variant="plain" type="button" onclick={() => (decideOpen = false)}>{t('requests.add.cancel')}</Button>
				<Button variant={mode === 'approve' ? 'primary' : 'destructive'} type="submit" {busy}>
					{mode === 'approve' ? t('requests.window.approve') : t('requests.window.decline')}
				</Button>
			</div>
		</form>
	{/if}
</Sheet>

<Sheet
	bind:open={declineFlow.declineOpen}
	size="sm"
	title={declineFlow.declineList.length === 1 ? t('requests.page.declineOne', { name: `${declineFlow.declineList[0].name}${askCards.year(declineFlow.declineList[0])}` }) : t('requests.page.declineMany', { n: declineFlow.declineList.length })}
	description={t('requests.page.undoFew')}
>
	<form class="decide" onsubmit={declineFlow.confirmDecline}>
		<div class="field">
			<label for="dc-why">{t('requests.page.reasonOptional', { who: declineFlow.declineList.length === 1 ? ctx.who(declineFlow.declineList[0]).name : t('requests.page.thePeople') })}</label>
			<textarea id="dc-why" class="input" rows="3" maxlength="500" placeholder={t('requests.page.reasonPlaceholder')} bind:this={declineFlow.whyBox} bind:value={declineFlow.declineWhy} data-testid="decline-why"></textarea>
			<div class="why-chips" role="group" aria-label={t('requests.page.quickReasons')}>
				{#each declineFlow.reasons as why (why)}
					<button type="button" class="why-chip" aria-pressed={declineFlow.hasWhy(why)} onclick={() => declineFlow.tapWhy(why)}>{why}</button>
				{/each}
			</div>
		</div>
		<div class="decide-actions">
			<Button variant="plain" type="button" onclick={() => (declineFlow.declineOpen = false)}>{t('requests.add.cancel')}</Button>
			<Button variant="destructive" type="submit">{t('requests.window.decline')}</Button>
		</div>
	</form>
</Sheet>

<Sheet bind:open={takeBackFlow.takeBackOpen} onclose={takeBackFlow.takeBackClosed} size="sm" title={takeBackFlow.takeBackAsk ? t('requests.page.takeBackTitle', { name: `${takeBackFlow.takeBackAsk.name}${askCards.year(takeBackFlow.takeBackAsk)}` }) : t('requests.page.takeBack')} description={t('requests.page.takeBackDesc')}>
	{#if takeBackFlow.takeBackAsk}
		{#if takeBackFlow.takeBackError}<p class="form-error" role="alert" data-testid="take-back-error">{takeBackFlow.takeBackError}</p>{/if}
		<div class="decide-actions">
			<Button variant="plain" type="button" onclick={() => (takeBackFlow.takeBackOpen = false)}>{t('requests.page.keepIt')}</Button>
			<Button variant="destructive" type="button" busy={takingBack.has(takeBackFlow.takeBackAsk.id)} onclick={() => takeBackFlow.takeBackAsk && void takeBackFlow.takeBack(takeBackFlow.takeBackAsk)}>{t('requests.page.takeBack')}</Button>
		</div>
	{/if}
</Sheet>

<style>
	.limit { margin: 0 0 16px; padding: 12px 16px; border-radius: var(--r-card); background: var(--surface-2); color: var(--text-muted); font: var(--t-callout); }
	.tick-all input { width: 18px; height: 18px; margin-top: 4px; flex: none; accent-color: var(--accent-fill); }
	.bulk { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; min-height: 44px; margin-bottom: 10px; padding: 6px 12px; border-radius: 999px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.bulk.on { background: var(--surface-2); }
	.tick-all { display: inline-flex; align-items: center; gap: 8px; margin-right: auto; font: var(--t-subhead); color: var(--text-muted); }
	.tick-all input { margin-top: 0; }
	.pending { font: var(--t-footnote); }
	.decide { display: flex; flex-direction: column; gap: 14px; }
	.decide .field + .field { margin-top: 0; }
	textarea.input { height: auto; padding-top: 10px; resize: vertical; }
	.why-chips { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 10px; }
	.why-chip { min-height: 32px; padding: 0 12px; border: 1px solid var(--line); border-radius: 999px; background: var(--surface-2); color: var(--text); font: var(--t-footnote); font-weight: 600; cursor: pointer; transition: background var(--dur-fast) var(--ease-out), border-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out); }
	.why-chip:hover { background: var(--surface-3); }
	.why-chip[aria-pressed='true'] { background: var(--text); border-color: var(--text); color: var(--bg); }
	.why-chip:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.decide-actions { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: 8px; margin-top: 6px; }
	.variant { min-width: 0; }
	/* Reported problems: a small link for the owner and admins. */
	.rp-link { display: inline-flex; margin-bottom: 10px; align-items: center; min-height: 32px; padding: 0 12px; border-radius: 999px; background: var(--surface-2); color: var(--text); font: 600 13px/1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.rp-link:hover { background: color-mix(in srgb, var(--text) 10%, var(--surface-2)); }
	.rp-link:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (max-width: 767px) and (pointer: coarse) { .rp-link { min-height: 44px; } }
	/* People who cannot approve: the approvers' card language (lib/requests/RequestsBoard.svelte) around their own lines. */
	.ap-top.mem { display: flex; flex-direction: column; gap: 12px; margin-bottom: 16px; }
	.ask-actions { display: flex; flex-wrap: wrap; gap: 10px; }
	.tb { display: inline-flex; align-items: center; justify-content: center; gap: 7px; height: 40px; padding: 0 14px; border: 1px solid var(--line); border-radius: 12px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); color: var(--text); font: 600 13px/1 var(--font-ui); white-space: nowrap; text-decoration: none; -webkit-backdrop-filter: blur(18px) saturate(1.4); backdrop-filter: blur(18px) saturate(1.4); transition: transform 160ms var(--ease-out), background 200ms var(--ease-out); }
	.tb:hover { background: var(--surface-2); }
	.tb:active { transform: scale(0.97); }
	.tb:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.tb.ink { border-color: transparent; background: var(--text); color: var(--bg); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.2), 0 6px 14px -8px rgb(0 0 0 / 0.6); }
	.tb.ink:hover { background: color-mix(in srgb, var(--text) 88%, var(--bg)); }
	.ask-rp { margin-left: auto; font-weight: 500; }
	.tb.ask-one { display: none; }
	.limit { margin: 0; }
	.toolbar { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px 16px; }
	.tabs { position: relative; display: flex; align-items: center; gap: 2px; height: 40px; max-width: 100%; box-sizing: border-box; padding: 3px; border: 1px solid var(--line); border-radius: 12px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); backdrop-filter: blur(18px) saturate(1.4); -webkit-backdrop-filter: blur(18px) saturate(1.4); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 6px 20px -10px rgb(0 0 0 / 0.35); overflow-x: auto; overflow-y: hidden; scrollbar-width: none; overscroll-behavior-x: contain; }
	.tabs::-webkit-scrollbar { display: none; }
	.pw { position: relative; display: flex; align-items: center; flex: none; height: 32px; }
	.pw.on .tab { background: var(--seg-thumb); box-shadow: 0 0 0 1px var(--line-strong), 0 1px 2px rgb(0 0 0 / 0.12); }
	.tab { position: relative; z-index: 1; display: inline-flex; align-items: center; gap: 7px; height: 32px; padding: 0 13px; border: 0; border-radius: 9px; background: none; color: color-mix(in srgb, var(--text) 88%, var(--bg)); font: 500 13.5px/1 var(--font-ui); white-space: nowrap; cursor: pointer; transition: color 200ms var(--ease-out), background-color 200ms var(--ease-out), box-shadow 200ms var(--ease-out); }
	.tab:hover, .tab[aria-pressed='true'] { color: var(--text); }
	.tab:hover:not([aria-pressed='true']) { background: color-mix(in srgb, var(--text) 6%, transparent); }
	.tab.zero:not([aria-pressed='true']):not(:hover) { color: var(--text-muted); }
	.tab:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.n { font-weight: 600; font-variant-numeric: tabular-nums; }
	.n.wait { color: var(--wait-text, var(--wait)); }
	:global(:root[data-theme='dark']) .pw.on .n.wait { color: #ffd08a; }
	@media (prefers-color-scheme: dark) { :global(:root:not([data-theme='light'])) .pw.on .n.wait { color: #ffd08a; } }
	@media (pointer: coarse) { .tab::after { content: ''; position: absolute; inset: -6px 0; } .tb { height: 44px; } }

	.member { min-width: 0; }
	.cards { position: relative; display: grid; grid-template-columns: repeat(auto-fill, minmax(min(290px, 100%), 1fr)); gap: 16px; margin: 0; padding: 0; list-style: none; }
	.mc { --top: 170px; --gap: 10px; position: relative; isolation: isolate; overflow: hidden; display: flex; flex-direction: column; border-radius: 20px; background: #17181d; box-shadow: 0 0 0 1px var(--line), 0 12px 32px -16px rgb(0 0 0 / 0.6); transition: opacity var(--dur-fast) var(--ease-out), box-shadow 300ms var(--ease-out); }
	.mc.waiting { opacity: 0.7; }
	/* Opened from History's "Go to Requests": the card wears a ring. */
	.mc.hl { box-shadow: 0 0 0 2px var(--text), 0 24px 48px -24px rgb(0 0 0 / 0.6); }
	.ban { position: absolute; inset: 0; z-index: -1; overflow: hidden; border-radius: inherit; background: #17181d; }
	.ban.noart { background: radial-gradient(120% 90% at 15% 0%, #3b4263 0%, transparent 60%), radial-gradient(90% 80% at 100% 100%, #583a58 0%, transparent 58%), #17181d; }
	.ban-img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; object-position: center 28%; background-size: cover; background-position: center 28%; transform: scale(1.02); transition: transform 400ms cubic-bezier(0.32, 0.72, 0, 1), filter 500ms var(--ease-out); }
	img.ban-img { animation: ban-in 600ms var(--ease-out) both; }
	@keyframes ban-in { from { opacity: 0; } }
	.ban-img.soft { filter: blur(18px) saturate(1.3); transform: scale(1.3); opacity: 0.8; }
	.ban::after { content: ''; position: absolute; inset: 0; background: linear-gradient(to bottom, rgb(0 0 0 / 0.22), transparent calc(var(--top) * 0.22), rgb(0 0 0 / 0.6) calc(var(--top) * 0.6), rgb(0 0 0 / 0.8) var(--top), rgb(0 0 0 / 0.4)); }
	/* A declined ask steps back: its art loses some colour. */
	.answered .ban-img:not(.soft) { filter: saturate(0.6); }
	.top { flex: none; height: var(--top); display: flex; align-items: flex-end; box-sizing: border-box; padding: 0 14px 10px; }
	.ban-cap { min-width: 0; display: flex; flex-direction: column; gap: 3px; color: #fff; }
	.new-mark { align-self: flex-start; margin-bottom: 4px; padding: 2px 8px; border-radius: var(--r-pill); background: var(--accent-fill); color: var(--text-on-red); font: 600 11px/16px var(--font-ui); }
	.ban-t { margin: 0; font: 800 22px/1.05 var(--font-display, var(--font-ui)); letter-spacing: -0.02em; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
	.ban-t a { color: inherit; text-decoration: none; }
	/* The whole art opens the title's page once it is in the Library. */
	.ban-t a::after { content: ''; position: absolute; inset: 0 0 auto; height: var(--top); }
	.ban-t a:focus-visible { outline: none; }
	.mc:has(.ban-t a:focus-visible) { box-shadow: 0 0 0 2px var(--text), 0 24px 48px -24px rgb(0 0 0 / 0.6); }
	.ban-f { font: 600 12px/1.3 var(--font-ui); color: rgb(255 255 255 / 0.85); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	/* The glass panel redefines --text, --text-muted and --bg so the ink reads on the blurred art. */
	.sheet-b { --blur: blur(22px) saturate(1.3); --text: light-dark(#14161c, #f5f5f7); --text-muted: light-dark(#1c2029, rgb(235 235 245 / 0.8)); --bg: light-dark(#eef0f5, #17171b); --gl-fail: light-dark(#86160b, #ff8f8a); --g1: light-dark(rgb(255 255 255 / 0.72), rgb(34 34 40 / 0.7)); --g2: light-dark(rgb(246 248 252 / 0.7), rgb(10 10 13 / 0.8)); --g3: light-dark(rgb(255 255 255 / 0.06), transparent); --edge: inset 0 1px 0 light-dark(rgb(255 255 255 / 0.75), rgb(255 255 255 / 0.16)), inset 0 0 0 1px light-dark(rgb(30 36 58 / 0.07), rgb(255 255 255 / 0.05)); --st-wait: light-dark(#8a5a00, #ffb340); --st-wait-ink: light-dark(#5c3b00, #ffd699); --st-ok: light-dark(#18843f, #34c26a); --st-ok-ink: light-dark(#073d1b, #a6efc0); --st-go: light-dark(#1d5fd6, #4b8dff); --st-go-ink: light-dark(#072a6b, #b5d1ff); --st-bad: light-dark(#a3170c, #ff6b63); --st-bad-ink: light-dark(#5e0e06, #ffb3ae); --st-soft: light-dark(#5a6070, #9aa0ad); --st-soft-ink: var(--text); position: relative; z-index: 1; display: flex; flex-direction: column; gap: 10px; flex: 1; margin: 0 var(--gap) var(--gap); padding: 12px; border-radius: calc(20px - var(--gap)); background: linear-gradient(180deg, var(--g1), var(--g2)), linear-gradient(var(--g3), var(--g3)); color: var(--text); -webkit-backdrop-filter: var(--blur); backdrop-filter: var(--blur); box-shadow: var(--edge); }
	:global(:root[data-theme='light']) .sheet-b { --blur: blur(30px) saturate(1.3) brightness(1.14); }
	@media (prefers-color-scheme: light) { :global(:root:not([data-theme])) .sheet-b { --blur: blur(30px) saturate(1.3) brightness(1.14); } }
	/* The status: an icon tile and the word, as on the approvers' Done cards; the row's menu (Take back) at its end. */
	.st-row { display: flex; align-items: center; gap: 9px; min-width: 0; min-height: 32px; }
	.st-wait { --st-c: var(--st-wait); --st-ink: var(--st-wait-ink); }
	.st-ok { --st-c: var(--st-ok); --st-ink: var(--st-ok-ink); }
	.st-go { --st-c: var(--st-go); --st-ink: var(--st-go-ink); }
	.st-bad { --st-c: var(--st-bad); --st-ink: var(--st-bad-ink); }
	.st-soft { --st-c: var(--st-soft); --st-ink: var(--st-soft-ink); }
	.st-chip { display: flex; align-items: center; gap: 9px; min-width: 0; }
	.st-sq { display: grid; place-items: center; flex: none; width: 26px; height: 26px; border-radius: 8px; background: linear-gradient(180deg, rgb(255 255 255 / 0.1), transparent 60%), color-mix(in srgb, var(--st-c) 20%, transparent); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.14), inset 0 0 0 1px color-mix(in srgb, var(--st-c) 42%, transparent); color: var(--st-ink); }
	.st-chip > b { color: var(--st-ink); font: 700 14px/1.3 var(--font-ui); }
	.row-menu { margin-left: auto; }
	.lines { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
	.line { margin: 0; font: 500 13px/1.38 var(--font-ui); overflow-wrap: anywhere; }
	.line.what { font-weight: 600; }
	.muted { color: var(--text-muted); }
	.note { color: var(--text); display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
	.rq-fail { color: var(--gl-fail); font-weight: 600; }
	.rq-step { color: var(--text); font-weight: 600; }
	.lines :global(.outcome) { font-size: 13px; }
	@media (hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference) {
		.mc:hover .ban-img:not(.soft) { transform: scale(1.0608); transition: transform 5s cubic-bezier(0.42, 0, 0.3, 1), filter 500ms var(--ease-out); }
	}
	/* Phones: one Ask button beside Your reports, the tabs swipe, one card per row so the lines stay readable. */
	@media (max-width: 767px) {
		.member { padding-bottom: calc(var(--tabbar, 64px) + 16px + env(safe-area-inset-bottom, 0px)); }
		.tb.ask-k { display: none; }
		.tb.ask-one { display: inline-flex; flex: 1; }
		.ask-actions { flex-wrap: nowrap; gap: 8px; }
		.ask-rp { flex: none; margin-left: 0; }
		.toolbar { flex-direction: column; align-items: stretch; }
		.cards { grid-template-columns: minmax(0, 1fr); gap: 12px; }
		.mc { --top: 132px; }
	}
	@media (min-width: 560px) and (max-width: 767px) { .cards { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
	@media (max-width: 767px) and (pointer: coarse) { .tabs { height: 46px; padding-block: 6px; border-radius: 15px; } }
	@media (prefers-reduced-motion: reduce) { img.ban-img { animation: none; } .ban-img, .tb, .mc { transition: none; } .tb:active { transform: none; } }
</style>
