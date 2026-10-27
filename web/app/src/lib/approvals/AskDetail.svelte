<script lang="ts">
	// One ask in full: backdrop, poster, facts, what was asked, and the actions.
	// The review sheet uses it plain; the Inbox pane adds the options form
	// (Library folder, Pick rules and a note) that decides in place.
	import Poster from '../Poster.svelte';
	import Button from '../ui/Button.svelte';
	import StatusPill from '../ui/StatusPill.svelte';
	import Avatar from '../ui/Avatar.svelte';
	import KidMark from './KidMark.svelte';
	import { to } from '../paths.ts';
	import { ago, dateTime } from '../format.ts';
	import { kindWord, runtimeLine, type ApprovalsCtx } from './approvals.ts';
	import type { Ask } from '../types.ts';

	let { ctx, a, form = false, headingId }: { ctx: ApprovalsCtx; a: Ask; form?: boolean; headingId?: string } = $props();

	$effect(() => ctx.want(a));
	const d = $derived(ctx.details(a));
	const who = $derived(ctx.who(a));
	const art = $derived(d?.backdrop ?? a.poster ?? '');
	const facts = $derived([d?.year ? String(d.year) : a.year ? String(a.year) : '', kindWord(a.kind), runtimeLine(d?.runtime), d?.rating ?? '', ...(d?.genres ?? []).slice(0, 2)].filter(Boolean));

	// The options form, reset for each ask.
	let folderId = $state(0);
	let rulesId = $state(0);
	let note = $state('');
	let busy = $state<'' | 'approve' | 'turn_down'>('');
	let error = $state('');
	$effect(() => {
		void a.id;
		folderId = 0;
		rulesId = 0;
		note = '';
		error = '';
	});
	async function decide(mode: 'approve' | 'turn_down') {
		busy = mode;
		error = await ctx.decideNow(a, mode, note.trim(), folderId, rulesId);
		busy = '';
	}
	const folders = $derived(ctx.foldersFor(a));
	const rules = $derived(ctx.rulesFor(a));
	const waiting = $derived(ctx.approver && a.state === 'asked' && !ctx.isPending(a.id));
</script>

<article class="detail" data-testid="ask-detail">
	<div class="hero" class:music={a.kind === 'artist'}>
		{#if art}<div class="bd" style:background-image={`url("${to(art)}")`} aria-hidden="true"></div>{/if}
		<div class="hero-in">
			<div class="poster"><Poster src={a.poster} name={a.name} /></div>
			<div class="head">
				<h2 class="title" id={headingId} tabindex="-1">
					{#if ctx.href(a)}<a href={to(ctx.href(a))}>{a.name}</a>{:else}{a.name}{/if}
				</h2>
				{#if facts.length}<p class="facts">{facts.join(' · ')}</p>{/if}
				<StatusPill kind={ctx.pill(a).kind} label={ctx.pill(a).label} />
			</div>
		</div>
	</div>

	<div class="body">
		<div class="who">
			<Avatar name={who.name} src={who.avatar} size={36} />
			<div class="who-text">
				<p class="who-name">{who.name} <KidMark label={who.kid} size="sm" /></p>
				<p class="muted small-line">asked <time datetime={a.asked_at} title={dateTime(a.asked_at)}>{ago(a.asked_at)}</time>{#if a.people.length > 1}{' '}· with {a.people.length - 1} {a.people.length === 2 ? 'other' : 'others'}{/if}</p>
			</div>
		</div>

		{#if ctx.what(a)}
			<div class="asked-for" data-testid={a.kind === 'artist' ? 'ask-albums' : 'ask-seasons'}>
				<span class="label">Asked for</span>
				<span class="value">{ctx.what(a)}</span>
			</div>
		{/if}

		{#if d?.overview}<p class="overview">{d.overview}</p>{/if}
		{#if ctx.decided(a)}<p class="muted small-line" data-testid="decided-by">{ctx.decided(a)}{#if a.decided_at}{' '}· {ago(a.decided_at)}{/if}</p>{/if}
		{#if a.note}<p class="note">“{a.note}”</p>{/if}
		{#if a.state === 'approved'}<p class="muted small-line">{ctx.approvedLine(a)}</p>{/if}
		{#if ctx.isPending(a.id)}<p class="muted small-line">Sending in a moment…</p>{/if}

		{#if waiting && form}
			<form class="options" onsubmit={(e) => (e.preventDefault(), void decide('approve'))}>
				{#if folders.length || rules.length}
					<div class="two">
						{#if folders.length}
							<div class="field">
								<label for="ib-folder-{a.id}">Library folder</label>
								<select id="ib-folder-{a.id}" class="input" bind:value={folderId}>
									<option value={0}>The Wishlist default</option>
									{#each folders as f (f.id)}<option value={f.id}>{f.label || f.path}</option>{/each}
								</select>
							</div>
						{/if}
						{#if rules.length}
							<div class="field">
								<label for="ib-rules-{a.id}">Pick rules</label>
								<select id="ib-rules-{a.id}" class="input" bind:value={rulesId}>
									<option value={0}>The Wishlist default</option>
									{#each rules as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
								</select>
							</div>
						{/if}
					</div>
				{/if}
				<div class="field">
					<label for="ib-note-{a.id}">Note to {who.name} (optional)</label>
					<textarea id="ib-note-{a.id}" class="input" rows="2" maxlength="500" bind:value={note} placeholder="They see it with the answer"></textarea>
				</div>
				{#if error}<p class="form-error" role="alert">{error}</p>{/if}
				<div class="acts">
					<Button variant="primary" icon="check" type="submit" busy={busy === 'approve'} disabled={!!busy}>Approve and add</Button>
					<Button icon="x" type="button" busy={busy === 'turn_down'} disabled={!!busy} onclick={() => void decide('turn_down')}>Turn down</Button>
				</div>
			</form>
		{:else if waiting}
			<div class="acts">
				<Button variant="primary" icon="check" type="button" onclick={() => ctx.approve([a])}>Approve</Button>
				<Button icon="sparkle" type="button" onclick={() => ctx.withOptions(a)}>Approve with options</Button>
				<Button icon="x" type="button" onclick={() => ctx.turnDown([a])}>Turn down</Button>
			</div>
		{/if}
		{#if a.mine && a.state === 'asked'}
			<div class="acts"><Button variant="plain" icon="trash" type="button" onclick={() => ctx.takeBack(a)}>Take back</Button></div>
		{/if}
	</div>
</article>

<style>
	.detail { display: flex; flex-direction: column; min-width: 0; }
	.hero { position: relative; isolation: isolate; overflow: hidden; border-radius: var(--r-panel); background: var(--surface-2); min-height: 200px; display: flex; align-items: flex-end; }
	.bd { position: absolute; inset: 0; z-index: -2; background-size: cover; background-position: center 25%; }
	.music .bd { filter: blur(26px) saturate(1.3); transform: scale(1.2); opacity: 0.75; }
	.hero::after { content: ''; position: absolute; inset: 0; z-index: -1; background: linear-gradient(180deg, var(--on-art-scrim-none) 0%, var(--on-art-scrim-mid) 45%, var(--on-art-scrim) 100%); }
	.hero-in { display: flex; align-items: flex-end; gap: 16px; padding: 18px; width: 100%; color: var(--on-art); }
	.poster { width: 92px; flex: none; border-radius: 8px; overflow: hidden; box-shadow: 0 10px 30px rgb(0 0 0 / 40%); }
	.head { min-width: 0; display: flex; flex-direction: column; align-items: flex-start; gap: 6px; }
	.title { font: var(--t-title2); letter-spacing: var(--t-title2-track); overflow-wrap: anywhere; scroll-margin-top: calc(var(--topbar) + 16px); }
	.title a:hover { text-decoration: underline; text-underline-offset: 4px; }
	.title:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 6px; }
	.facts { color: var(--on-art-muted); font: var(--t-footnote); overflow-wrap: anywhere; }
	.body { display: flex; flex-direction: column; gap: 14px; padding: 16px 2px 2px; }
	.who { display: flex; align-items: center; gap: 10px; }
	.who-text { min-width: 0; }
	.who-name { font: var(--t-headline); display: flex; align-items: center; flex-wrap: wrap; gap: 6px; overflow-wrap: anywhere; }
	.small-line { font: var(--t-footnote); overflow-wrap: anywhere; }
	.asked-for { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; padding: 12px 14px; border-radius: var(--r-card); background: var(--surface-2); }
	.asked-for .label { color: var(--text-muted); font: var(--t-footnote); }
	.asked-for .value { font: var(--t-headline); text-align: right; overflow-wrap: anywhere; }
	.overview { font: var(--t-callout); color: var(--text-muted); display: -webkit-box; -webkit-line-clamp: 5; line-clamp: 5; -webkit-box-orient: vertical; overflow: hidden; }
	.note { padding: 10px 12px; border-radius: 10px; background: var(--surface-2); font: var(--t-callout); overflow-wrap: anywhere; }
	.options { display: flex; flex-direction: column; gap: 12px; padding-top: 12px; border-top: 1px solid var(--line); }
	.two { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 12px; }
	.options .field + .field { margin-top: 0; }
	textarea.input { height: auto; padding-top: 10px; resize: vertical; }
	.acts { display: flex; flex-wrap: wrap; gap: 8px; }
	.acts > :global(*) { flex: 1 1 auto; }
	@media (max-width: 600px) {
		.hero { min-height: 180px; }
		.poster { width: 76px; }
		.title { font: var(--t-title3); }
		.acts { flex-direction: column; }
	}
</style>
