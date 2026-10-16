<script lang="ts">
	import { to } from '#lib/paths.ts';
	import { onMount, onDestroy } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { dateTime, bytes } from '#lib/format.ts';
	import Icon from '#lib/Icon.svelte';
	import {
		apps,
		appName,
		addSuggestions,
		cleanRules,
		groupLabels,
		linesByGroup,
		notMovedByGroup,
		resultWords,
		ruleProblem,
		running,
		sameName,
		setChoice,
		summary,
		tone,
		type AppKind,
		type Move,
		type PathRule,
		type PersonChoice,
		type Plan,
		type Report,
		type Snapshot
	} from '#lib/move.ts';

	// Step 1: read the old apps.
	let kind = $state<AppKind>('radarr');
	let way = $state<'api' | 'backup'>('api');
	let address = $state('');
	let key = $state('');
	let username = $state('');
	let signWord = $state('');
	let backupPath = $state('');
	let local = $state(true);
	// Old-app keys saved from the secrets file (owner only); never the key.
	let savedKeys = $state<{ kind: string; address: string; allow_local_network: boolean }[]>([]);
	let useSaved = $state(true);
	let reading = $state(false);
	let readError = $state('');
	let snapshots = $state<Snapshot[]>([]);
	let picked = $state<number[]>([]);
	let loaded = $state(false);
	let loadError = $state('');

	// Step 2: the plan and the owner's choices.
	let rules = $state<PathRule[]>([]);
	let suggested = $state<PathRule[]>([]);
	let skip = $state<string[]>([]);
	// The owner's word per person: owner, admin, merge or skip.
	let people = $state<Record<string, PersonChoice>>({});
	let plan = $state<Plan | null>(null);
	let planning = $state(false);
	let planError = $state('');

	// Step 3: the dry run. Step 4: the move.
	let dry = $state<Report | null>(null);
	// The move starts only from the plan this dry run showed.
	let planHash = $state('');
	let trying = $state(false);
	let moving = $state(false);
	let current = $state<Move | null>(null);
	let progress = $state(0);
	let progressNote = $state('');
	let moves = $state<Move[]>([]);
	let timer: ReturnType<typeof setTimeout> | null = null;

	const qbit = $derived(kind === 'qbittorrent');
	const saved = $derived(savedKeys.find((x) => x.kind === kind));
	const withSaved = $derived(way === 'api' && !!saved && useSaved);
	const ruleErrors = $derived(cleanRules(rules).map(ruleProblem));
	const rulesOk = $derived(ruleErrors.every((e) => e === ''));

	async function load() {
		try {
			const [s, m] = await Promise.all([api.get<{ snapshots: Snapshot[] }>('/migrate/snapshots'), api.get<{ moves: Move[] }>('/migrate/moves')]);
			snapshots = s.snapshots;
			moves = m.moves;
			loadError = '';
			const busy = m.moves.find(running);
			if (busy) watch(busy.id);
			savedKeys = await api
				.get<{ saved: { kind: string; address: string; allow_local_network: boolean }[] }>('/migrate/saved-keys')
				.then((r) => r.saved)
				.catch(() => []); // only the owner sees them
		} catch (e) {
			loadError = message(e);
		} finally {
			loaded = true;
		}
	}
	onMount(load);
	onDestroy(() => {
		if (timer) clearTimeout(timer);
	});

	function stale() {
		plan = null;
		dry = null;
		planHash = '';
	}

	async function read(e: SubmitEvent) {
		e.preventDefault();
		reading = true;
		readError = '';
		const body: Record<string, unknown> = { kind, allow_local_network: local };
		if (way === 'backup') body.path = backupPath.trim();
		else if (withSaved) {
			body.use_saved_key = true;
			if (address.trim()) body.url = address.trim();
		} else {
			body.url = address.trim();
			if (qbit) {
				body.username = username;
				body.password = signWord;
			}
			if (key) body.key = key;
		}
		try {
			const r = await api.post<{ snapshot: Snapshot; needs_reentry?: string[] }>('/migrate/read', body);
			snapshots = [r.snapshot, ...snapshots];
			picked = [...picked, r.snapshot.id];
			if (body.use_saved_key) savedKeys = savedKeys.filter((x) => x.kind !== kind);
			key = '';
			signWord = '';
			toasts.push(`${appName(kind)} read. Keys never come over; ${r.needs_reentry?.length ?? 0} to type again after the move.`, 'ok');
			stale();
		} catch (err) {
			readError = message(err);
		} finally {
			reading = false;
			key = '';
			signWord = '';
		}
	}

	async function forget(s: Snapshot) {
		if (!confirm(`Forget what was read from ${appName(s.kind)}? Nothing in HolyArrs or ${appName(s.kind)} changes.`)) return;
		try {
			await api.del(`/migrate/snapshots/${s.id}`);
			snapshots = snapshots.filter((x) => x.id !== s.id);
			picked = picked.filter((x) => x !== s.id);
			stale();
		} catch (err) {
			toasts.push(message(err), 'fail');
		}
	}

	function togglePick(id: number, on: boolean) {
		picked = on ? [...picked, id] : picked.filter((x) => x !== id);
		stale();
	}

	function choices() {
		return { snapshots: picked, path_rules: cleanRules(rules), skip, people };
	}

	function choose(key: string, c: PersonChoice | null) {
		people = setChoice(people, key, c);
		makePlan();
	}

	async function makePlan() {
		planning = true;
		planError = '';
		dry = null;
		planHash = '';
		try {
			const r = await api.post<{ plan: Plan; suggested_path_rules: PathRule[] }>('/migrate/plan', choices());
			plan = r.plan;
			suggested = r.suggested_path_rules ?? [];
		} catch (err) {
			planError = message(err);
			plan = null;
		} finally {
			planning = false;
		}
	}

	function useSuggestions() {
		rules = addSuggestions(rules, suggested);
		makePlan();
	}

	function toggleGroup(name: string, on: boolean) {
		skip = on ? skip.filter((x) => x !== name) : [...skip, name];
		makePlan();
	}

	async function tryIt() {
		trying = true;
		try {
			const r = await api.post<{ report: Report; plan_hash: string }>('/migrate/dry-run', choices());
			dry = r.report;
			planHash = r.plan_hash;
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			trying = false;
		}
	}

	async function approve() {
		if (!confirm('Move now? HolyArrs adds what the dry run listed. No media file is moved or renamed, and the old apps are not touched.')) return;
		moving = true;
		try {
			const r = await api.post<{ move: Move }>('/migrate/moves', { ...choices(), plan_hash: planHash });
			current = r.move;
			watch(r.move.id);
		} catch (err) {
			toasts.push(message(err), 'fail');
			moving = false;
		}
	}

	function watch(id: number) {
		moving = true;
		const tick = async () => {
			try {
				const r = await api.get<{ move: Move; progress: number; progress_note: string }>(`/migrate/moves/${id}`);
				current = r.move;
				progress = r.progress;
				progressNote = r.progress_note;
				if (!running(r.move)) {
					moving = false;
					moves = [r.move, ...moves.filter((x) => x.id !== id)];
					if (r.move.state === 'undone') toasts.push(`Undone: ${summary(r.move.report?.counts ?? {}) || 'nothing to remove'}.`, 'ok');
					else if (r.move.undo_state === 'failed') toasts.push('The undo stopped before it finished. Try it again.', 'fail');
					else if (r.move.state === 'done') toasts.push('The move is done. Its report is below.', 'ok');
					else toasts.push('The move stopped. Its report says how far it got.', 'fail');
					return;
				}
			} catch (err) {
				toasts.push(message(err), 'fail');
			}
			timer = setTimeout(tick, 1000);
		};
		tick();
	}

	async function undo(m: Move) {
		if (!confirm('Undo this move? Only what it added is removed. Titles leave the Library and their files stay where they are.')) return;
		try {
			// The undo runs in the background like the move; follow it.
			const r = await api.post<{ move: Move }>(`/migrate/moves/${m.id}/undo`);
			current = r.move;
			watch(m.id);
		} catch (err) {
			toasts.push(message(err), 'fail');
		}
	}

	async function openMove(m: Move) {
		try {
			const r = await api.get<{ move: Move }>(`/migrate/moves/${m.id}`);
			current = r.move;
		} catch (err) {
			toasts.push(message(err), 'fail');
		}
	}

	function countWords(c: Record<string, number>): string {
		return Object.entries(c)
			.map(([k, n]) => `${n} ${k.replaceAll('_', ' ')}`)
			.join(' · ');
	}
</script>

<svelte:head><title>Move from other apps · HolyArrs</title></svelte:head>

<header class="page-head">
	<div>
		<h1>Move from other apps</h1>
		<p>Bring a setup over from the apps HolyArrs replaces. Read them, look at the plan, try it, then move. Media files are never moved or renamed, and the old apps are never changed.</p>
	</div>
</header>

{#if !loaded}
	<div class="sk" style="height: 200px"></div>
{:else if loadError}
	<p class="notice bad" role="alert">{loadError}</p>
{:else}
	<ol class="steps">
		<li>
			<section class="card" aria-labelledby="h-read">
				<h2 id="h-read" class="h2"><span class="num" aria-hidden="true">1</span> Read your apps</h2>
				<form onsubmit={read} class="read">
					<div class="two">
						<div class="field">
							<label for="mv-app">App</label>
							<select id="mv-app" class="input" bind:value={kind}>
								{#each apps as a (a.kind)}<option value={a.kind}>{a.name} ({a.what})</option>{/each}
							</select>
						</div>
						<fieldset class="field way">
							<legend class="label">How</legend>
							<label class="checkline"><input type="radio" name="way" value="api" bind:group={way} /> Through the app (address and key)</label>
							<label class="checkline"><input type="radio" name="way" value="backup" bind:group={way} /> From a backup file on this server</label>
						</fieldset>
					</div>
					{#if way === 'api'}
						{#if saved}
							<label class="checkline" data-testid="use-saved-key">
								<input type="checkbox" bind:checked={useSaved} />
								<span
									><b>Use the saved key for {appName(kind)}</b><br /><span class="hint"
										>Saved from your secrets file for {saved.address}. Used for one read, then forgotten. It is never shown.</span
									></span
								>
							</label>
						{/if}
						<div class="field">
							<label for="mv-url">Address</label>
							<input id="mv-url" class="input mono" type="url" bind:value={address} placeholder={withSaved ? saved?.address : 'http://10.0.0.5:7878'} required={!withSaved} />
						</div>
						{#if withSaved}
							<!-- the saved key is used on the server; nothing to type -->
						{:else if qbit}
							<div class="two">
								<div class="field">
									<label for="mv-user">User name</label>
									<input id="mv-user" class="input" bind:value={username} autocomplete="off" />
								</div>
								<div class="field">
									<label for="mv-pass">Password</label>
									<input id="mv-pass" class="input" type="password" bind:value={signWord} autocomplete="new-password" />
								</div>
							</div>
						{/if}
						{#if !withSaved}
							<div class="field">
								<label for="mv-key">{qbit ? 'API key (qBittorrent 5.2 and later, instead of a password)' : 'API key'}</label>
								<input id="mv-key" class="input mono" type="password" bind:value={key} autocomplete="new-password" spellcheck="false" required={!qbit} />
								<span class="hint">Used for this one read, then forgotten: never saved, logged or shown.</span>
							</div>
						{/if}
						<label class="checkline">
							<input type="checkbox" bind:checked={local} />
							<span><b>The app is on my home network</b><br /><span class="hint">Lets this read reach local addresses.</span></span>
						</label>
					{:else}
						<div class="field">
							<label for="mv-path">Backup path on this server</label>
							<input id="mv-path" class="input mono" bind:value={backupPath} placeholder="/backups/radarr_backup.zip" required />
							<span class="hint">A backup zip, the app's config folder, or one database or settings file.</span>
						</div>
					{/if}
					{#if readError}<p class="form-error" role="alert">{readError}</p>{/if}
					<div class="row" style="margin-top: var(--s-4)">
						<button class="btn btn-red" type="submit" disabled={reading} aria-busy={reading}><Icon name="download" size={18} /> Read {appName(kind)}</button>
					</div>
				</form>

				{#if snapshots.length > 0}
					<h3 class="h3 sub">Read so far</h3>
					<ul class="snaps" aria-label="Read apps">
						{#each snapshots as s (s.id)}
							<li>
								<label class="checkline grow">
									<input type="checkbox" checked={picked.includes(s.id)} onchange={(e) => togglePick(s.id, e.currentTarget.checked)} />
									<span>
										<b>{appName(s.kind)}</b>
										{s.app_version}
										<span class="xs muted">· {s.via === 'api' ? 'through the app' : 'from a backup'} · {dateTime(s.created_at)} · {bytes(s.size)}</span>
										<br /><span class="xs muted">{countWords(s.counts)}</span>
									</span>
								</label>
								<button class="btn btn-quiet" type="button" onclick={() => forget(s)} aria-label="Forget {appName(s.kind)} read {s.id}">Forget</button>
							</li>
						{/each}
					</ul>
				{/if}
			</section>
		</li>

		<li>
			<section class="card" aria-labelledby="h-plan">
				<div class="card-head">
					<h2 id="h-plan" class="h2"><span class="num" aria-hidden="true">2</span> See the plan</h2>
					<button class="btn btn-ghost" type="button" onclick={makePlan} disabled={picked.length === 0 || planning || !rulesOk} aria-busy={planning}>
						<Icon name="refresh" size={18} /> {plan ? 'Plan again' : 'Make the plan'}
					</button>
				</div>
				{#if picked.length === 0}<p class="hint">Pick at least one read app above.</p>{/if}

				<fieldset class="rules">
					<legend class="label">Path rules</legend>
					<p class="hint">When the old app saw a folder under another path (another container), say both paths.</p>
					{#each rules as r, i (i)}
						<div class="rule">
							<div class="field">
								<label for="rule-from-{i}">Old app's path</label>
								<input id="rule-from-{i}" class="input mono" bind:value={r.from} placeholder="/movies" aria-invalid={ruleErrors[i] ? 'true' : undefined} oninput={stale} />
							</div>
							<div class="field">
								<label for="rule-to-{i}">Path here</label>
								<input id="rule-to-{i}" class="input mono" bind:value={r.to} placeholder="/data/movies" aria-invalid={ruleErrors[i] ? 'true' : undefined} oninput={stale} />
							</div>
							<button class="icon-btn" type="button" aria-label="Remove path rule {i + 1}" onclick={() => ((rules = rules.filter((_, j) => j !== i)), stale())}><Icon name="x" size={18} /></button>
							{#if ruleErrors[i]}<p class="form-error rule-err" role="alert">{ruleErrors[i]}</p>{/if}
						</div>
					{/each}
					<div class="row">
						<button class="btn btn-quiet" type="button" onclick={() => (rules = [...rules, { from: '', to: '' }])}><Icon name="plus" size={18} /> Add a path rule</button>
						{#if suggested.length > 0}
							<button class="btn btn-quiet" type="button" onclick={useSuggestions}>Use {suggested.length} suggested {suggested.length === 1 ? 'rule' : 'rules'}</button>
						{/if}
					</div>
					{#if suggested.length > 0}
						<ul class="xs muted sugg">
							{#each suggested as s (s.from)}<li><span class="mono">{s.from}</span> → <span class="mono">{s.to}</span></li>{/each}
						</ul>
					{/if}
				</fieldset>

				{#if planError}<p class="notice bad" role="alert">{planError}</p>{/if}
				{#if plan}
					<div class="groups" data-testid="plan">
						{#each plan.groups as g (g.name)}
							{#if g.items.length > 0 || skip.includes(g.name)}
								<details class="group">
									<summary>
										<span class="grow"><b>{g.label}</b> <span class="chip" data-testid="group-count">{g.items.length}</span></span>
									</summary>
									<label class="checkline keep">
										<input type="checkbox" checked={!skip.includes(g.name)} onchange={(e) => toggleGroup(g.name, e.currentTarget.checked)} />
										Move {g.label.toLowerCase()}
									</label>
									<ul class="items">
										{#each g.items as it (it.key)}
											<li>
												<b>{it.name}</b>
												{#if it.detail}<span class="small muted"> · {it.detail}</span>{/if}
												{#if it.why}<br /><span class="xs muted">Why: {it.why}</span>{/if}
												{#if it.needs}<br /><span class="chip wait">Needs {it.needs}</span>{/if}
												{#if it.person?.was_owner}
													<label class="checkline confirm">
														<input type="checkbox" checked={people[it.key] === 'owner'} onchange={(e) => choose(it.key, e.currentTarget.checked ? 'owner' : null)} />
														This is me (their asks become mine)
													</label>
												{:else if it.person?.was_admin}
													<label class="checkline confirm">
														<input type="checkbox" checked={people[it.key] === 'admin'} onchange={(e) => choose(it.key, e.currentTarget.checked ? 'admin' : null)} />
														Keep {it.name} an admin here
													</label>
												{/if}
											</li>
										{/each}
									</ul>
								</details>
							{/if}
						{/each}
					</div>
					{#if plan.not_moved.length > 0}
						<details class="group notmoved">
							<summary><span class="grow"><b>Not moved</b> <span class="chip" data-testid="not-moved-count">{plan.not_moved.length}</span></span></summary>
							<p class="hint">No HolyArrs match. They stay in the old apps.</p>
							{#each notMovedByGroup(plan.not_moved, groupLabels) as g (g.label)}
								<h3 class="h3 sub">{g.label}</h3>
								<ul class="items">
									{#each g.items as n, i (n.from + i)}<li><b>{n.what}</b> <span class="small muted">· {n.why}</span></li>{/each}
								</ul>
							{/each}
						</details>
					{/if}
					{#if plan.needs_input.length > 0}
						<details class="group">
							<summary><span class="grow"><b>Needs your input afterwards</b> <span class="chip wait">{plan.needs_input.length}</span></span></summary>
							<p class="hint">Keys and passwords never come over. Type them again after the move.</p>
							<ul class="items">{#each plan.needs_input as n (n)}<li class="small">{n}</li>{/each}</ul>
						</details>
					{/if}
				{/if}
			</section>
		</li>

		<li>
			<section class="card" aria-labelledby="h-try">
				<div class="card-head">
					<h2 id="h-try" class="h2"><span class="num" aria-hidden="true">3</span> Try it (dry run)</h2>
					<button class="btn btn-ghost" type="button" onclick={tryIt} disabled={!plan || trying} aria-busy={trying}>Try it</button>
				</div>
				<p class="hint">Shows exactly what would be added, changed or left out on this server. Nothing changes.</p>
				{#if dry}
					<p class="small" role="status" data-testid="dry-summary"><b>{summary(dry.counts)}</b></p>
					{#each linesByGroup(dry) as g (g.group)}
						<details class="group">
							<summary><span class="grow"><b>{groupLabels[g.group] ?? g.group}</b> <span class="xs muted">{summary(g.counts)}</span></span></summary>
							<ul class="items">
								{#each g.lines as l (l.key)}
									<li>
										<span class="chip {tone(l.result) === 'ok' ? 'ok' : tone(l.result) === 'warn' ? 'wait' : tone(l.result) === 'bad' ? 'fail' : ''}">{resultWords[l.result] ?? l.result}</span> <b>{l.name}</b>{#if l.detail}<span class="small muted"> · {l.detail}</span>{/if}
										{#if sameName(l)}
											<div class="row pick">
												<button class="btn btn-quiet" type="button" onclick={() => choose(l.key, 'merge')} aria-label="Merge {l.name} with the person of this name here">Same person: merge</button>
												<button class="btn btn-quiet" type="button" onclick={() => choose(l.key, 'skip')} aria-label="Leave {l.name} out">Leave out</button>
											</div>
										{/if}
									</li>
								{/each}
							</ul>
						</details>
					{/each}
					{#if dry.next.length > 0}
						<ul class="next small">{#each dry.next as n (n)}<li>{n}</li>{/each}</ul>
					{/if}
				{/if}
			</section>
		</li>

		<li>
			<section class="card" aria-labelledby="h-move">
				<div class="card-head">
					<h2 id="h-move" class="h2"><span class="num" aria-hidden="true">4</span> Move</h2>
					<button class="btn btn-red" type="button" onclick={approve} disabled={!dry || !planHash || moving} aria-busy={moving}>Approve and move</button>
				</div>
				<p class="hint">Runs in the background, from exactly the plan the dry run showed (change anything and try it again). Moving again later adds nothing twice, and a move can be undone.</p>
				<p class="hint">Sources and Downloaders always come over paused and cannot reach your home network. After the move, check each one, turn on "reach the local network" only for one that lives on your network, and resume it.</p>
				{#if moving}
					<div role="status" aria-live="polite">
						<div class="bar" style="margin-top: var(--s-3)"><i style="transform: scaleX({Math.max(progress, 0.02)})"></i></div>
						<p class="xs muted">{progressNote || 'Starting the move…'}</p>
					</div>
				{/if}
				{#if current?.report && !moving}
					<div class="report" data-testid="move-report" role="status">
						<p><b>Move {current.id}: {current.state === 'undone' ? 'undone' : current.state}{current.undo_state === 'failed' ? ' (its undo stopped; try it again)' : ''}.</b> {summary(current.report.counts)}</p>
						<h3 class="h3 sub">What to do next</h3>
						<ul class="next small">{#each current.report.next as n (n)}<li>{n}</li>{/each}</ul>
						{#if current.report.needs_input.length > 0}
							<h3 class="h3 sub">Needs your input</h3>
							<ul class="items">{#each current.report.needs_input as n (n)}<li class="small">{n}</li>{/each}</ul>
						{/if}
						{#each linesByGroup(current.report) as g (g.group)}
							<details class="group">
								<summary><span class="grow"><b>{groupLabels[g.group] ?? g.group}</b> <span class="xs muted">{summary(g.counts)}</span></span></summary>
								<ul class="items">
									{#each g.lines as l (l.key)}<li><span class="chip {tone(l.result) === 'ok' ? 'ok' : tone(l.result) === 'warn' ? 'wait' : tone(l.result) === 'bad' ? 'fail' : ''}">{resultWords[l.result] ?? l.result}</span> <b>{l.name}</b>{#if l.detail}<span class="small muted"> · {l.detail}</span>{/if}</li>{/each}
								</ul>
							</details>
						{/each}
						<div class="row" style="margin-top: var(--s-4)">
							<a class="btn btn-ghost" href={to(`/api/v1/migrate/moves/${current.id}/report`)} download><Icon name="download" size={18} /> Download the report</a>
							{#if current.state === 'done' || current.state === 'failed'}
								<button class="btn btn-quiet" type="button" onclick={() => current && undo(current)}>Undo this move</button>
							{/if}
						</div>
					</div>
				{/if}
			</section>
		</li>
	</ol>

	{#if moves.length > 0}
		<section class="card" aria-labelledby="h-moves">
			<h2 id="h-moves" class="h2">Earlier moves</h2>
			<ul class="snaps">
				{#each moves as m (m.id)}
					<li>
						<span class="grow small"><b>Move {m.id}</b> · {dateTime(m.created_at)} · {m.state}{#if m.report} · {summary(m.report.counts)}{/if}</span>
						<button class="btn btn-quiet" type="button" onclick={() => openMove(m)}>Open</button>
						{#if m.state === 'done' || m.state === 'failed'}
							<button class="btn btn-quiet" type="button" onclick={() => undo(m)} aria-label="Undo move {m.id}">Undo</button>
						{/if}
					</li>
				{/each}
			</ul>
		</section>
	{/if}
{/if}

<style>
	.steps {
		list-style: none;
		padding: 0;
		margin: 0;
		display: grid;
		gap: var(--s-4);
	}
	.num {
		display: inline-grid;
		place-items: center;
		width: 26px;
		height: 26px;
		margin-right: var(--s-2);
		border-radius: 50%;
		background: var(--surface-3);
		font-size: var(--text-sm);
	}
	.two {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
		gap: var(--s-4);
		margin-bottom: var(--s-4);
	}
	.way {
		border: 0;
		padding: 0;
		margin: 0;
	}
	.read .field + .field,
	.read .checkline {
		margin-top: var(--s-4);
	}
	.sub {
		margin-top: var(--s-5);
		margin-bottom: var(--s-2);
	}
	.snaps {
		list-style: none;
		padding: 0;
		margin: 0;
		display: grid;
		gap: var(--s-2);
	}
	.snaps li {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		flex-wrap: wrap;
		padding: var(--s-2) 0;
		border-top: 1px solid var(--line);
	}
	.rules {
		border: 0;
		padding: 0;
		margin: var(--s-4) 0;
	}
	.rule {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) auto;
		gap: var(--s-3);
		align-items: end;
		margin-bottom: var(--s-3);
	}
	.rule-err {
		grid-column: 1 / -1;
	}
	.sugg {
		margin: var(--s-2) 0 0;
		padding-left: var(--s-5);
	}
	.groups {
		display: grid;
		gap: var(--s-2);
		margin-top: var(--s-4);
	}
	.group {
		border: 1px solid var(--line);
		border-radius: var(--r-btn);
		padding: var(--s-2) var(--s-3);
		margin-top: var(--s-2);
	}
	.group summary {
		display: flex;
		align-items: center;
		cursor: pointer;
		min-height: 36px;
	}
	.keep {
		margin: var(--s-2) 0;
	}
	.confirm {
		margin-top: var(--s-1);
	}
	.pick {
		margin-top: var(--s-1);
		gap: var(--s-2);
		flex-wrap: wrap;
	}
	.items {
		list-style: none;
		padding: 0;
		margin: var(--s-2) 0 0;
		display: grid;
		gap: var(--s-2);
		max-height: 420px;
		overflow: auto;
		overflow-wrap: anywhere;
	}
	.next {
		margin: var(--s-2) 0 0;
		padding-left: var(--s-5);
	}
	.report {
		margin-top: var(--s-4);
	}
	@media (max-width: 640px) {
		.rule {
			grid-template-columns: minmax(0, 1fr) auto;
		}
		.rule .field:first-child {
			grid-column: 1 / -1;
		}
	}
</style>
