<script lang="ts">
	import { to } from '#lib/paths.ts';
	import { onMount } from 'svelte';
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
		const offs = [live.on('ask', () => void load()), live.on('ask_removed', () => void load())];
		return () => offs.forEach((off) => off());
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
		if (!confirm(`Take back your ask for ${a.name}?`)) return;
		try {
			await api.del(`/asks/${a.id}`);
			toasts.push(`Took back the ask for ${a.name}.`);
			await load();
		} catch (err) {
			toasts.push(message(err), 'fail');
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
			{#if inbox.unread}<button class="btn btn-quiet" type="button" onclick={() => void markRead()}>Mark all read</button>{/if}
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

{#if error}
	<p class="notice bad" role="alert">{error}</p>
{:else if !loaded}
	{#each Array(2) as _, i (i)}<div class="sk" style="height: 120px; margin-bottom: 12px"></div>{/each}
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
	<ul class="asks">
		{#each shown as a, i (a.id)}
			<li class="ask card rise" style="--i: {i}" data-testid="ask">
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
						{#if approver && a.state === 'asked'}
							<button class="btn btn-red" type="button" onclick={() => open(a, 'approve')}><Icon name="check" size={18} /> Approve</button>
							<button class="btn btn-ghost" type="button" onclick={() => open(a, 'turn_down')}><Icon name="x" size={18} /> Turn down</button>
						{/if}
						{#if a.mine && a.state === 'asked'}
							<button class="btn btn-quiet" type="button" onclick={() => takeBack(a)}>Take back</button>
						{/if}
					</div>
				</div>
			</li>
		{/each}
	</ul>
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
		margin-bottom: var(--s-4);
	}
	.pills {
		display: flex;
		flex-wrap: wrap;
		gap: var(--s-2);
		margin-bottom: var(--s-4);
	}
	.pill {
		min-height: 40px;
		padding: 0 var(--s-4);
		border-radius: var(--r-pill);
		border: 1px solid var(--line-strong);
		background: var(--surface-1);
		color: var(--text-muted);
		font-weight: var(--weight-medium);
		cursor: pointer;
	}
	.pill[aria-pressed='true'] {
		background: var(--ha-red-soft);
		border-color: var(--ha-red);
		color: var(--text);
	}
	.pill .n {
		margin-left: 4px;
		color: var(--text-faint);
	}
	.asks {
		display: flex;
		flex-direction: column;
		gap: var(--s-3);
		max-width: 900px;
	}
	.ask {
		display: flex;
		gap: var(--s-4);
		align-items: flex-start;
	}
	.ask + .ask {
		margin-top: 0;
	}
	.art {
		width: 72px;
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
		margin-top: var(--s-2);
		flex-wrap: wrap;
	}
	.actions:empty {
		display: none;
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
		border-radius: var(--r-modal);
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
		.art {
			width: 56px;
		}
		.actions :global(.btn) {
			flex: 1 1 auto;
		}
	}
</style>
