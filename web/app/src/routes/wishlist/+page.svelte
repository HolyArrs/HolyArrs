<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import { to } from '#lib/paths.ts';
	import { onMount, tick } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import StatusPill, { type StatusKind } from '#lib/ui/StatusPill.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import Poster from '#lib/Poster.svelte';
	import { ago, dateTime } from '#lib/format.ts';
	import { live } from '#lib/live.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { looksLikeId } from '#lib/plain.ts';
	import { askedBy as askedByRaw, decided as decidedRaw, limitLines, limitKinds } from '#lib/wishlist.ts';
	import { partsNow } from '#lib/parts.svelte.ts';
	import { inbox, markRead, refreshInbox } from '#lib/wishlist.svelte.ts';
	import type { Ask, AskLimit, LibraryFolder, PickRulesView } from '#lib/types.ts';
	import { seasonWords } from '#lib/shows.ts';
	import { albumWords } from '#lib/music.ts';

	const approver = $derived(session.can('approve'));
	let asks = $state<Ask[]>([]);
	let loaded = $state(false);
	let error = $state('');
	let show = $state<'waiting' | 'all' | 'mine'>('all');
	let kind = $state<'all' | 'movie' | 'show' | 'artist'>('all');
	let limit = $state<AskLimit | null>(null);
	// One line per kind whose part is on.
	const onKinds = $derived(limitKinds.filter((k) => partsNow.answer?.parts.find((p) => p.part === k)?.on !== false));

	const ofKind = $derived(kind === 'all' ? asks : asks.filter((a) => a.kind === kind));
	const shown = $derived(
		show === 'waiting' ? ofKind.filter((a) => a.state === 'asked') : show === 'mine' ? ofKind.filter((a) => a.mine) : ofKind
	);
	// The kind chips show once asks of more than one kind are listed.
	const bothKinds = $derived(new Set(asks.map((a) => a.kind)).size > 1);
	const waiting = $derived(asks.filter((a) => a.state === 'asked').length);
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

	async function load() {
		try {
			const r = await api.get<{ asks: Ask[] }>('/asks');
			asks = r.asks;
			error = '';
		} catch (e) {
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
		void load().then(() => {
			if (approver && asks.some((a) => a.state === 'asked')) show = 'waiting';
		});
		void refreshInbox();
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
		const compactQuery = window.matchMedia('(max-width: 600px)');
		const syncCompactReview = () => (compactReview = compactQuery.matches);
		syncCompactReview();
		compactQuery.addEventListener('change', syncCompactReview);
		const offs = [live.on('ask', () => void load()), live.on('ask_removed', () => void load())];
		return () => {
			compactQuery.removeEventListener('change', syncCompactReview);
			offs.forEach((off) => off());
		};
	});

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
		dialogError = '';
		try {
			const path = `/asks/${picked.id}/${mode === 'approve' ? 'approve' : 'turn-down'}`;
			const body = mode === 'approve' ? { note, library_folder_id: folderId, pick_rules_id: rulesId } : { note };
			const r = await api.post<{ ask: Ask }>(path, body);
			const i = asks.findIndex((x) => x.id === r.ask.id);
			if (i >= 0) asks[i] = r.ask;
			decideOpen = false;
			const added =
				r.ask.kind === 'show'
					? `${r.ask.name} is approved and added; HolyArrs keeps ${seasonWords(r.ask.seasons)} up to date.`
					: r.ask.kind === 'artist'
						? `${r.ask.name} is approved and added; HolyArrs keeps ${albumWords(r.ask.albums)} up to date.`
						: `${r.ask.name} is approved and added; HolyArrs is looking for it.`;
			toasts.push(mode === 'approve' ? added : `${r.ask.name} is turned down.`, 'ok');
			void refreshInbox();
		} catch (err) {
			dialogError = message(err);
		} finally {
			busy = false;
		}
	}

	async function takeBack(a: Ask) {
		if (takingBack.has(a.id)) return;
		if (!confirm(`Take back your ask for ${a.name}?`)) return;
		takingBack = new Set([...takingBack, a.id]);
		try {
			await api.del(`/asks/${a.id}`);
			toasts.push(`Took back the ask for ${a.name}.`);
			await load();
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			takingBack = new Set([...takingBack].filter((id) => id !== a.id));
		}
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

	const pillKind: Record<string, StatusKind> = { asked: 'requested', approved: 'approved', turned_down: 'turned_down', available: 'in_library' };
	const msgKind = (k: string): StatusKind => (k === 'ready' ? 'in_library' : k === 'turned_down' ? 'turned_down' : 'approved');

	// Id-like account names read as a person, never a raw id.
	const plain = (t: string) => t.replace(/[\w.-]+/g, (w) => (/\d/.test(w) && /\p{L}/u.test(w) && looksLikeId(w) ? 'someone in your household' : w));
	const askedBy = (a: Ask) => plain(askedByRaw(a));
	const decided = (a: Ask) => plain(decidedRaw(a) ?? '');

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
		if (a.kind === 'artist') return `HolyArrs keeps ${what(a)} up to date. Looking for music Releases comes in a later version.`;
		return 'HolyArrs is looking for it.';
	}
</script>

<svelte:head><title>{approver ? 'Approvals' : 'Your Wishlist'} · HolyArrs</title></svelte:head>

<PageHeader
	title={approver ? 'Approvals' : 'Your Wishlist'}
	subtitle={approver ? 'What people asked for. Approve to add, or turn down with a note.' : 'The movies, shows and music you asked for, and where each one is.'}
>
	{#snippet actions()}
		{#if session.can('ask')}
			<Button icon="film" variant="primary" href={to(`/search`)}>Ask for a movie</Button>
			<Button icon="tv" href={to(`/search?kind=show`)}>Ask for a show</Button>
			<Button icon="music" href={to(`/search?kind=music`)}>Ask for music</Button>
		{/if}
	{/snippet}
</PageHeader>

{#if limitLines(limit, onKinds).length}
	<ul class="limit" data-testid="ask-limit">
		{#each limitLines(limit, onKinds) as l (l.kind)}<li>{l.text}</li>{/each}
	</ul>
{/if}

{#if bothKinds || asks.length}
	<div class="toolbar">
		{#if approver && asks.length}
			<div class="chips" role="group" aria-label="Show">
				<Chip label="Waiting" count={waiting} selected={show === 'waiting'} onclick={() => (show = 'waiting')} />
				<Chip label="Everyone's" count={asks.length} selected={show === 'all'} onclick={() => (show = 'all')} />
				<Chip label="Mine" selected={show === 'mine'} onclick={() => (show = 'mine')} />
			</div>
		{/if}
		{#if bothKinds}
			<div class="chips" role="group" aria-label="Kind">
				<Chip label="Everything" selected={kind === 'all'} onclick={() => (kind = 'all')} />
				<Chip label="Movies" selected={kind === 'movie'} onclick={() => (kind = 'movie')} />
				<Chip label="Shows" selected={kind === 'show'} onclick={() => (kind = 'show')} />
				<Chip label="Music" selected={kind === 'artist'} onclick={() => (kind = 'artist')} />
			</div>
		{/if}
		{#if asks.length}<p class="count">{shown.length} {shown.length === 1 ? 'request' : 'requests'}</p>{/if}
	</div>
{/if}

<div class="workspace">
	<div class="main-col">
		{#if error}
			<LoadError hasData={false} message="Your Wishlist could not be loaded. {error}" onretry={load} />
		{:else if !loaded}
			<div aria-label="Loading Wishlist"><Skeleton shape="block" height="320px" /></div>
		{:else if asks.length === 0}
			<EmptyState
				icon="wish"
				title="Nothing on the Wishlist yet"
				text={session.can('ask')
					? 'Find a movie, a show or an artist in Search and press Ask. It shows up here, and you hear when it is approved and when it is ready.'
					: 'Your account may not ask for movies, shows or music yet; ask the owner.'}
				action={{ label: 'Search', href: to('/search') }}
			/>
		{:else if shown.length === 0}
			<EmptyState compact icon="check" title={show === 'waiting' ? 'Nothing waits for approval.' : 'Nothing here.'} />
		{:else}
			<ul class="asks" aria-label="Wishlist requests">
				{#each shown as a, i (a.id)}
					<li class="ask rise" class:selected={selected?.id === a.id} style="--i: {i}" data-testid="ask">
						<div class="art"><Poster src={a.poster} name={a.name} /></div>
						<div class="body">
							<div class="top">
								<h2 class="name">
									{#if a.title_id}<a href={to(`/library/${a.title_id}`)}>{a.name}<span class="muted">{year(a)}</span></a>
									{:else}{a.name}<span class="muted">{year(a)}</span>{/if}
								</h2>
								<StatusPill kind={pillKind[a.state] ?? 'requested'} label={a.state_label} testid="ask-state" />
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
							{#if a.state === 'approved'}<p class="line muted">{approvedLine(a)}</p>{/if}
							<div class="actions">
								{#if approver && a.state === 'asked'}
									<Button variant="primary" size="sm" icon="check" type="button" onclick={() => open(a, 'approve')}>Approve</Button>
									<Button size="sm" icon="x" type="button" onclick={() => open(a, 'turn_down')}>Turn down</Button>
								{/if}
								{#if a.mine && a.state === 'asked'}
									<Button variant="destructive" size="sm" type="button" disabled={takingBack.has(a.id)} aria-busy={takingBack.has(a.id)} onclick={() => takeBack(a)}>Take back</Button>
								{/if}
								<button
									class="inspect"
									class:on={selected?.id === a.id}
									type="button"
									aria-label="Review {a.name}"
									aria-pressed={selected?.id === a.id}
									aria-expanded={compactReview ? reviewOpen && selected?.id === a.id : undefined}
									aria-controls="wishlist-review"
									onclick={(event) => void selectAsk(a.id, event.currentTarget)}>Review</button
								>
							</div>
						</div>
					</li>
				{/each}
			</ul>
		{/if}
	</div>

	<div class="side-col">
		{#if loaded && !error && selected}
			<aside id="wishlist-review" class="review" class:mobile-open={reviewOpen} aria-label="Selected Wishlist item">
				<div class="review-close"><Button variant="plain" size="sm" icon="x" type="button" aria-label="Close review" onclick={() => void closeReview()}>Close</Button></div>
				<div class="review-head">
					<div class="review-art"><Poster src={selected.poster} name={selected.name} /></div>
					<div>
						<h2 class="review-title" bind:this={reviewHeading} tabindex="-1">{selected.name}<span class="muted">{year(selected)}</span></h2>
						<StatusPill kind={pillKind[selected.state] ?? 'requested'} label={selected.state_label} />
					</div>
				</div>
				<dl class="review-facts">
					<dt>Asked by</dt><dd>{askedBy(selected).replace(/^Asked by /, '')}</dd>
					<dt>Asked</dt><dd>{ago(selected.asked_at)}</dd>
					{#if what(selected)}<dt>For</dt><dd>{what(selected)}</dd>{/if}
					{#if decided(selected)}<dt>Decision</dt><dd>{decided(selected)}</dd>{/if}
				</dl>
				{#if selected.note}<p class="review-note">“{selected.note}”</p>{/if}
				{#if selected.state === 'approved'}<p class="line muted">{approvedLine(selected)}</p>{/if}
				{#if approver && selected.state === 'asked'}
					<div class="review-actions">
						<Button variant="primary" icon="check" full type="button" onclick={() => selected && open(selected, 'approve')}>Approve</Button>
						<Button icon="x" full type="button" onclick={() => selected && open(selected, 'turn_down')}>Turn down</Button>
					</div>
				{/if}
			</aside>
		{/if}

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
								<StatusPill kind={msgKind(m.kind)} label={m.kind === 'ready' ? 'Ready' : m.kind === 'turned_down' ? 'Turned down' : 'Approved'} />
								<span class="msg-text">{plain(m.text)}</span>
								<time class="muted" datetime={m.created_at} title={dateTime(m.created_at)}>{ago(m.created_at)}</time>
							</li>
						{/each}
					</ul>
				</details>
			</section>
		{/if}
	</div>
</div>

<Sheet
	bind:open={decideOpen}
	title={picked ? `${mode === 'approve' ? 'Approve' : 'Turn down'} ${picked.name}${year(picked)}` : 'Decide'}
	description={picked ? `${askedBy(picked)}${what(picked) ? `, for ${what(picked)}` : ''}.` : undefined}
>
	{#if picked}
		<form class="decide" onsubmit={decide}>
			{#if mode === 'approve' && kindFolders.length > 0}
				<div class="field">
					<label for="ap-folder">Library folder</label>
					<select id="ap-folder" class="input" bind:value={folderId}>
						<option value={0}>The Wishlist default</option>
						{#each kindFolders as f (f.id)}
							<option value={f.id}>{f.label || f.path} ({f.path})</option>
						{/each}
					</select>
				</div>
				<div class="field">
					<label for="ap-rules">Pick rules</label>
					<select id="ap-rules" class="input" bind:value={rulesId}>
						<option value={0}>The Wishlist default</option>
						{#each kindRules as r (r.id)}
							<option value={r.id}>{r.name}</option>
						{/each}
					</select>
				</div>
			{/if}
			<div class="field">
				<label for="ap-note">{mode === 'approve' ? 'Note (optional)' : 'Why (sent to the people who asked)'}</label>
				<textarea id="ap-note" class="input" rows="3" maxlength="500" bind:value={note}></textarea>
			</div>
			{#if mode === 'approve'}
				<p class="hint">
					{picked.kind === 'show'
						? `Approving adds the show to the Library and keeps ${what(picked)} up to date; HolyArrs looks for the episodes at the next feed check and each night.`
						: picked.kind === 'artist'
							? `Approving adds the artist to the Library with ${what(picked)} kept up to date. HolyArrs does not look for music Releases yet.`
							: 'Approving adds the movie to the Library and starts looking for it.'}
				</p>
			{/if}
			{#if dialogError}<p class="form-error" role="alert">{dialogError}</p>{/if}
			<div class="decide-actions">
				<Button variant="plain" type="button" onclick={() => (decideOpen = false)}>Cancel</Button>
				<Button variant={mode === 'approve' ? 'primary' : 'destructive'} type="submit" {busy}>
					{mode === 'approve' ? 'Approve and add' : 'Turn down'}
				</Button>
			</div>
		</form>
	{/if}
</Sheet>

<style>
	.limit { margin: 0 0 16px; padding: 12px 16px; border-radius: var(--r-card); background: var(--surface-2); color: var(--text-muted); font: var(--t-callout); }
	.toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 10px 16px; margin-bottom: 16px; }
	.chips { display: flex; flex-wrap: wrap; gap: 8px; }
	.count { margin-left: auto; color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.workspace { display: grid; grid-template-columns: minmax(0, 1fr) minmax(300px, 360px); gap: 24px; align-items: start; }
	.main-col, .side-col { min-width: 0; }
	.side-col { position: sticky; top: calc(var(--topbar) + 16px); display: flex; flex-direction: column; gap: 16px; }
	.asks { border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); overflow: hidden; }
	.ask { display: flex; gap: 14px; padding: 14px 16px; transition: background var(--dur-fast) var(--ease-out); }
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
	.review-actions { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; margin-top: 14px; }
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
	.decide-actions { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: 8px; margin-top: 6px; }
	@media (pointer: coarse) { .inspect, summary { min-height: 44px; } }
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
		.actions > :global(*) { flex: 1 1 auto; }
		.review-actions { grid-template-columns: 1fr; }
	}
</style>
