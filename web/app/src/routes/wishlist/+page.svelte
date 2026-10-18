<script lang="ts">
	import { to } from '#lib/paths.ts';
	import { onMount, tick } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import EmptyState from '#lib/EmptyState.svelte';
	import Icon from '#lib/Icon.svelte';
	import Poster from '#lib/Poster.svelte';
	import { ago, dateTime } from '#lib/format.ts';
	import { live } from '#lib/live.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { askTone, askedBy, decided, limitLines, limitKinds } from '#lib/wishlist.ts';
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
	let dialog: HTMLDialogElement | undefined = $state();
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
		dialog?.showModal();
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
			dialog?.close();
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

<svelte:head><title>Wishlist · HolyArrs</title></svelte:head>

<header class="page-head">
	<div>
		<p class="section-kicker">Requests</p>
		<h1>Wishlist</h1>
		<p>{approver ? 'What people asked for. Approve to add, or turn down with a note.' : 'The movies, shows and music you asked for, and where each one is.'}</p>
	</div>
	{#if session.can('ask')}
		<div class="row head-actions">
			<a class="btn btn-red" href={to(`/search`)}><Icon name="film" size={18} /> Ask for a movie</a>
			<a class="btn btn-ghost" href={to(`/search?kind=show`)}><Icon name="tv" size={18} /> Ask for a show</a>
			<a class="btn btn-ghost" href={to(`/search?kind=music`)}><Icon name="music" size={18} /> Ask for music</a>
		</div>
	{/if}
</header>

{#if inbox.messages.length}
	<section class="card messages" aria-labelledby="h-msg">
		<div class="card-head">
			<h2 id="h-msg" class="h3">Messages{#if inbox.unread}{' '}<span class="chip red">{inbox.unread} new</span>{/if}</h2>
			{#if inbox.unread}<button class="btn btn-quiet" type="button" disabled={markingRead} aria-busy={markingRead} onclick={() => void readAllMessages()}>Mark all read</button>{/if}
		</div>
		<ul class="msg-list">
			{#each inbox.messages.slice(0, 5) as m (m.id)}
				<li class:unread={!m.read_at} data-testid="message">
					<span class="chip {m.kind === 'ready' ? 'ok' : m.kind === 'turned_down' ? 'fail' : ''}">{m.kind === 'ready' ? 'Ready' : m.kind === 'turned_down' ? 'Turned down' : 'Approved'}</span>
					<span class="grow small">{m.text}</span>
					<time class="xs muted" datetime={m.created_at} title={dateTime(m.created_at)}>{ago(m.created_at)}</time>
				</li>
			{/each}
		</ul>
	</section>
{/if}

{#if limitLines(limit, onKinds).length}
	<ul class="small muted limit" data-testid="ask-limit">
		{#each limitLines(limit, onKinds) as l (l.kind)}<li>{l.text}</li>{/each}
	</ul>
{/if}

{#if bothKinds || asks.length}
<div class="wishlist-toolbar">
{#if bothKinds}
	<div class="pills" role="group" aria-label="Kind">
		<button type="button" class="pill" aria-pressed={kind === 'all'} onclick={() => (kind = 'all')}>Everything</button>
		<button type="button" class="pill" aria-pressed={kind === 'movie'} onclick={() => (kind = 'movie')}>Movies</button>
		<button type="button" class="pill" aria-pressed={kind === 'show'} onclick={() => (kind = 'show')}>Shows</button>
	<button type="button" class="pill" aria-pressed={kind === 'artist'} onclick={() => (kind = 'artist')}>Music</button>
	</div>
{/if}

{#if approver && asks.length}
	<div class="pills" role="group" aria-label="Show">
		<button type="button" class="pill" aria-pressed={show === 'waiting'} onclick={() => (show = 'waiting')}>Waiting <span class="n">{waiting}</span></button>
		<button type="button" class="pill" aria-pressed={show === 'all'} onclick={() => (show = 'all')}>Everyone's <span class="n">{asks.length}</span></button>
		<button type="button" class="pill" aria-pressed={show === 'mine'} onclick={() => (show = 'mine')}>Mine</button>
	</div>
{/if}
	{#if asks.length}<p class="queue-count">{shown.length} {shown.length === 1 ? 'request' : 'requests'}</p>{/if}
</div>
{/if}

{#if error}
	<div class="notice bad load-error" role="alert">
		<div><b>Wishlist could not be loaded.</b><br /><span class="small">{error}</span></div>
		<button class="btn btn-ghost" type="button" onclick={load}>Try again</button>
	</div>
{:else if !loaded}
	<div class="loading-grid" aria-label="Loading Wishlist"><div class="sk" style="height: 320px"></div><div class="sk" style="height: 240px"></div></div>
{:else if asks.length === 0}
	<EmptyState icon="wish" title="Nothing on the Wishlist yet">
		<p>
			{session.can('ask')
				? 'Find a movie, a show or an artist in Search and press Ask. It shows up here, and you hear when it is approved and when it is ready.'
				: 'Your account may not ask for movies, shows or music yet; ask the owner.'}
		</p>
		{#snippet actions()}<a class="btn btn-red" href={to(`/search`)}>Search</a>{/snippet}
	</EmptyState>
{:else if shown.length === 0}
	<p class="quiet card">{show === 'waiting' ? 'Nothing waits for approval.' : 'Nothing here.'}</p>
{:else}
	<div class="wishlist-workspace">
		<ul class="asks" aria-label="Wishlist requests">
			{#each shown as a, i (a.id)}
				<li class="ask rise" class:selected={selected?.id === a.id} style="--i: {i}" data-testid="ask">
					<div class="art"><Poster src={a.poster} name={a.name} /></div>
					<div class="grow body">
						<div class="top">
							<h2 class="h3">
								{#if a.title_id}<a href={to(`/library/${a.title_id}`)}>{a.name}<span class="muted">{year(a)}</span></a>
								{:else}{a.name}<span class="muted">{year(a)}</span>{/if}
							</h2>
							<span class="chip {askTone(a.state)}" data-testid="ask-state">{a.state_label}</span>
							{#if a.kind === 'show'}<span class="chip" data-testid="ask-kind">Show</span>{/if}
							{#if a.kind === 'artist'}<span class="chip" data-testid="ask-kind">Music</span>{/if}
						</div>
						{#if what(a)}<p class="small" data-testid={a.kind === 'artist' ? 'ask-albums' : 'ask-seasons'}>Asked for {what(a)}</p>{/if}
						<p class="small muted" data-testid="asked-by">
							{askedBy(a)} · <time datetime={a.asked_at} title={dateTime(a.asked_at)}>{ago(a.asked_at)}</time>
						</p>
						{#if decided(a)}
							<p class="small muted" data-testid="decided-by">
								{decided(a)}{#if a.decided_at}{' '}· <time datetime={a.decided_at} title={dateTime(a.decided_at)}>{ago(a.decided_at)}</time>{/if}
							</p>
						{/if}
						{#if a.note}<p class="small note">“{a.note}”</p>{/if}
						{#if a.state === 'approved'}<p class="xs muted">{approvedLine(a)}</p>{/if}
						<div class="row actions">
							<button
								class="btn btn-quiet inspect"
								class:selected={selected?.id === a.id}
								type="button"
								aria-label="Review {a.name}"
								aria-pressed={selected?.id === a.id}
								aria-expanded={compactReview ? reviewOpen && selected?.id === a.id : undefined}
								aria-controls="wishlist-review"
								onclick={(event) => void selectAsk(a.id, event.currentTarget)}>Review</button
							>
							{#if approver && a.state === 'asked'}
								<button class="btn btn-red" type="button" onclick={() => open(a, 'approve')}><Icon name="check" size={18} /> Approve</button>
								<button class="btn btn-ghost" type="button" onclick={() => open(a, 'turn_down')}><Icon name="x" size={18} /> Turn down</button>
							{/if}
							{#if a.mine && a.state === 'asked'}
								<button class="btn btn-quiet" type="button" disabled={takingBack.has(a.id)} aria-busy={takingBack.has(a.id)} onclick={() => takeBack(a)}>Take back</button>
							{/if}
						</div>
					</div>
				</li>
			{/each}
		</ul>

		<aside id="wishlist-review" class="review" class:mobile-open={reviewOpen} aria-label="Selected Wishlist item">
			{#if selected}
				<button class="review-close btn btn-quiet" type="button" aria-label="Close review" onclick={() => void closeReview()}><Icon name="x" size={18} /> Close</button>
				<div class="review-art"><Poster src={selected.poster} name={selected.name} /></div>
				<div class="review-head">
					<div>
						<p class="eyebrow">Wishlist review</p>
						<h2 class="h2 review-title" bind:this={reviewHeading} tabindex="-1">{selected.name}<span class="muted">{year(selected)}</span></h2>
					</div>
					<span class="chip {askTone(selected.state)}">{selected.state_label}</span>
				</div>
				<dl class="review-facts">
					<dt>Asked by</dt><dd>{askedBy(selected)}</dd>
					<dt>Asked</dt><dd>{ago(selected.asked_at)}</dd>
					{#if what(selected)}<dt>For</dt><dd>{what(selected)}</dd>{/if}
					{#if decided(selected)}<dt>Decision</dt><dd>{decided(selected)}</dd>{/if}
				</dl>
				{#if selected.note}<p class="review-note">“{selected.note}”</p>{/if}
				{#if selected.state === 'approved'}<p class="small muted">{approvedLine(selected)}</p>{/if}
				{#if approver && selected.state === 'asked'}
					<div class="review-actions">
						<button class="btn btn-red" type="button" onclick={() => open(selected, 'approve')}><Icon name="check" size={18} /> Approve</button>
						<button class="btn btn-ghost" type="button" onclick={() => open(selected, 'turn_down')}><Icon name="x" size={18} /> Turn down</button>
					</div>
				{/if}
			{/if}
		</aside>
	</div>
{/if}

<dialog bind:this={dialog} aria-labelledby="decide-title" class="modal">
	{#if picked}
		<form onsubmit={decide}>
			<h2 id="decide-title" class="h2">{mode === 'approve' ? 'Approve' : 'Turn down'} {picked.name}{year(picked)}</h2>
			<p class="small muted" style="margin-top: var(--s-2)">{askedBy(picked)}{what(picked) ? `, for ${what(picked)}` : ''}.</p>
			{#if mode === 'approve' && kindFolders.length > 0}
				<div class="field" style="margin-top: var(--s-5)">
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
			<div class="field" style="margin-top: var(--s-4)">
				<label for="ap-note">{mode === 'approve' ? 'Note (optional)' : 'Why (sent to the people who asked)'}</label>
				<textarea id="ap-note" class="input" rows="3" maxlength="500" bind:value={note}></textarea>
			</div>
			{#if mode === 'approve'}
				<p class="hint" style="margin-top: var(--s-2)">
					{picked.kind === 'show'
						? `Approving adds the show to the Library and keeps ${what(picked)} up to date; HolyArrs looks for the episodes at the next feed check and each night.`
						: picked.kind === 'artist'
							? `Approving adds the artist to the Library with ${what(picked)} kept up to date. HolyArrs does not look for music Releases yet.`
							: 'Approving adds the movie to the Library and starts looking for it.'}
				</p>
			{/if}
			{#if dialogError}<p class="form-error" role="alert">{dialogError}</p>{/if}
			<div class="row modal-actions">
				<button class="btn btn-quiet" type="button" onclick={() => dialog?.close()}>Cancel</button>
				<button class="btn {mode === 'approve' ? 'btn-red' : 'btn-danger'}" type="submit" disabled={busy} aria-busy={busy}>
					{mode === 'approve' ? 'Approve and add' : 'Turn down'}
				</button>
			</div>
		</form>
	{/if}
</dialog>

<style>
	.head-actions {
		gap: var(--s-2);
		flex-wrap: wrap;
	}
	.section-kicker {
		margin-bottom: 5px;
		color: var(--text-muted);
		font-size: var(--text-sm);
		font-weight: var(--weight-medium);
	}
	.messages {
		margin-bottom: var(--s-5);
	}
	.msg-list {
		display: flex;
		flex-direction: column;
		gap: var(--s-2);
		margin-top: var(--s-3);
	}
	.msg-list li {
		display: flex;
		align-items: baseline;
		gap: var(--s-3);
		flex-wrap: wrap;
		padding: var(--s-2) 0;
	}
	.msg-list li.unread .small {
		color: var(--text);
		font-weight: var(--weight-medium);
	}
	.limit {
		margin: 0 0 var(--s-4);
		padding: var(--s-3) var(--s-4);
		border: 1px solid var(--line);
		border-radius: 12px;
		background: var(--surface-2);
	}
	.wishlist-toolbar {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		min-height: 58px;
		margin-bottom: var(--s-4);
		padding: 9px 0;
		border-block: 1px solid var(--line);
	}
	.pills {
		display: inline-flex;
		flex-wrap: wrap;
		gap: 3px;
		margin: 0;
		padding: 3px;
		border: 1px solid var(--line);
		border-radius: 11px;
		background: var(--surface-2);
	}
	.pill {
		min-height: 34px;
		padding: 0 var(--s-3);
		border: 0;
		border-radius: 8px;
		background: transparent;
		color: var(--text-muted);
		font-weight: var(--weight-medium);
		cursor: pointer;
	}
	.pill[aria-pressed='true'] {
		background: var(--surface-1);
		color: var(--text);
		box-shadow: 0 1px 2px rgb(20 24 18 / 10%);
	}
	.pill .n {
		margin-left: 4px;
		color: var(--text-faint);
	}
	.queue-count {
		margin-left: auto;
		color: var(--text-muted);
		font-size: var(--text-xs);
		font-weight: var(--weight-semibold);
		font-variant-numeric: tabular-nums;
	}
	.loading-grid,
	.wishlist-workspace {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(320px, 380px);
		gap: 0;
		align-items: start;
		border: 1px solid var(--line);
		border-radius: 14px;
		overflow: clip;
	}
	.asks {
		display: flex;
		flex-direction: column;
		gap: 0;
		min-width: 0;
		border: 0;
		border-radius: 0;
		background: var(--surface-1);
		overflow: hidden;
	}
	.ask {
		display: flex;
		gap: var(--s-3);
		align-items: center;
		padding: 12px 14px;
		border-left: 3px solid transparent;
		transition: background var(--dur) var(--ease-out), border-color var(--dur) var(--ease-out);
	}
	.ask + .ask {
		border-top: 1px solid var(--line);
	}
	.ask.selected {
		background: color-mix(in srgb, var(--info) 7%, var(--surface-1));
		border-left-color: color-mix(in srgb, var(--info) 65%, var(--line));
	}
	.art {
		width: 58px;
		flex: none;
	}
	.body {
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.top {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		flex-wrap: wrap;
	}
	.note {
		color: var(--text);
	}
	.actions {
		margin-top: 4px;
		flex-wrap: wrap;
	}
	.actions:empty {
		display: none;
	}
	.inspect.selected {
		background: var(--surface-3);
		color: var(--text);
	}
	.review {
		position: sticky;
		top: calc(var(--topbar) + var(--s-4));
		min-height: 100%;
		padding: var(--s-5);
		border-left: 1px solid var(--line);
		background: var(--surface-1);
		overflow: hidden;
	}
	.review-close {
		display: none;
	}
	.review-art {
		width: min(180px, 62%);
		margin: 0 auto var(--s-5);
		max-height: 260px;
		border-radius: 12px;
		overflow: hidden;
		background: var(--surface-2);
	}
	.review-art :global(img) {
		width: 100%;
		height: 100%;
		object-fit: cover;
		object-position: center 28%;
	}
	.review-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: var(--s-3);
	}
	.review-head .h2 {
		margin-top: 3px;
		overflow-wrap: anywhere;
	}
	.review-title {
		scroll-margin-top: calc(var(--topbar) + var(--s-4));
	}
	.review-title:focus-visible {
		outline: 2px solid var(--info);
		outline-offset: 4px;
		border-radius: var(--r-sm);
	}
	.eyebrow {
		margin: 0;
		font-size: var(--text-sm);
		font-weight: var(--weight-medium);
		color: var(--text-muted);
	}
	.review-facts {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr);
		gap: var(--s-2) var(--s-3);
		margin-top: var(--s-5);
		padding-top: var(--s-4);
		border-top: 1px solid var(--line);
		font-size: var(--text-sm);
	}
	.review-facts dt {
		color: var(--text-muted);
	}
	.review-facts dd {
		margin: 0;
		text-align: right;
		overflow-wrap: anywhere;
	}
	.review-note {
		margin-top: var(--s-4);
		padding: var(--s-3);
		border: 1px solid var(--line);
		border-left: 3px solid var(--info);
		border-radius: 10px;
		background: var(--surface-2);
		font-size: var(--text-sm);
		overflow-wrap: anywhere;
	}
	.review-actions {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--s-2);
		margin-top: var(--s-5);
	}
	.load-error {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--s-4);
	}
	.quiet {
		color: var(--text-muted);
	}
	.modal {
		width: min(480px, calc(100vw - 32px));
		max-height: calc(100dvh - 32px);
		overflow: auto;
		padding: var(--s-6);
		border: 0;
		border-radius: 16px;
		background: var(--surface-1);
		color: var(--text);
		box-shadow: var(--shadow-modal);
	}
	.modal::backdrop {
		background: var(--scrim);
	}
	.modal-actions {
		justify-content: flex-end;
		margin-top: var(--s-6);
	}
	textarea.input {
		height: auto;
		padding-top: 10px;
		resize: vertical;
	}
	@media (max-width: 600px) {
		.head-actions {
			display: grid;
			grid-template-columns: minmax(0, 1fr);
			width: 100%;
		}
		.head-actions .btn {
			width: 100%;
			justify-content: center;
		}
		.head-actions .btn:last-child:nth-child(odd) {
			grid-column: 1 / -1;
		}
		.messages .card-head {
			gap: var(--s-3);
		}
		.messages .card-head .h3 {
			display: flex;
			align-items: center;
			flex-wrap: wrap;
			gap: var(--s-2);
		}
		.review:not(.mobile-open) {
			display: none;
		}
		.review.mobile-open {
			display: block;
			padding: var(--s-4);
		}
		.review-close {
			display: inline-flex;
			margin: 0 0 var(--s-4) auto;
		}
		.review-art {
			width: min(132px, 44%);
			margin-bottom: var(--s-4);
		}
		.review-head {
			flex-direction: column;
			gap: var(--s-2);
		}
		.review-facts {
			grid-template-columns: minmax(88px, auto) minmax(0, 1fr);
		}
		.msg-list {
			gap: 0;
		}
		.msg-list li {
			display: grid;
			grid-template-columns: minmax(0, 1fr) auto;
			align-items: center;
			gap: var(--s-2) var(--s-3);
			padding: var(--s-3) 0;
		}
		.msg-list li + li {
			border-top: 1px solid var(--line);
		}
		.msg-list li > .chip {
			justify-self: start;
		}
		.msg-list li > .grow {
			grid-column: 1 / -1;
			grid-row: 2;
			min-width: 0;
			line-height: 1.45;
			overflow-wrap: anywhere;
		}
		.msg-list li > time {
			grid-column: 2;
			grid-row: 1;
			white-space: nowrap;
		}
		.art {
			width: 44px;
			align-self: start;
		}
		.ask {
			align-items: flex-start;
			gap: var(--s-3);
		}
		.top .h3 {
			flex: 1 0 100%;
			min-width: 0;
			line-height: 1.3;
			overflow-wrap: anywhere;
		}
		.ask .chip,
		.review .chip {
			max-width: 100%;
			height: auto;
			min-height: 26px;
			padding-block: 6px;
			line-height: 1.25;
			white-space: normal;
			overflow-wrap: anywhere;
		}
		.ask .chip::before,
		.review .chip::before { flex: none; }
		.body > .small,
		.body > .xs {
			line-height: 1.45;
			overflow-wrap: anywhere;
		}
		.actions {
			display: grid;
			grid-template-columns: minmax(0, 1fr);
			width: 100%;
		}
		.actions :global(.btn) {
			width: 100%;
			min-width: 0;
			min-height: 44px;
			white-space: nowrap;
		}
		.pill {
			min-height: 44px;
		}
		.wishlist-toolbar {
			align-items: flex-start;
			flex-direction: column;
		}
		.queue-count { order: -1; margin-left: 0; }
		.ask {
			padding: var(--s-3);
		}
		.load-error {
			align-items: stretch;
			flex-direction: column;
		}
		.review-actions {
			grid-template-columns: 1fr;
		}
	}
	@media (max-width: 980px) {
		.loading-grid,
		.wishlist-workspace {
			grid-template-columns: 1fr;
		}
		.review {
			position: static;
			grid-row: 1;
			border-left: 0;
			border-bottom: 1px solid var(--line);
		}
	}
</style>
