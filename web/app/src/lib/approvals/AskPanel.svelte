<script lang="ts">
	// One ask in full, for the Inbox panel, the phone's full screen and the
	// Stage arrangement: type tag and title, what it is for (with season
	// pills), not out yet, the rule and language, who asked and when, then
	// the decision: Rule and Language (starting from the person's picks),
	// an optional note, Approve and Turn down (both with a 6 s Undo).
	// `slim` drops the backdrop for a narrow panel; `stage` is the large one.
	import Poster from '../Poster.svelte';
	import Button from '../ui/Button.svelte';
	import StatusPill from '../ui/StatusPill.svelte';
	import Avatar from '../ui/Avatar.svelte';
	import KidMark from './KidMark.svelte';
	import AskSummary from './AskSummary.svelte';
	import { to } from '../paths.ts';
	import { ago, dateTime } from '../format.ts';
	import { kindWord, type ApprovalsCtx } from './approvals.ts';
	import { audioLanguages, languageName } from './inbox.ts';
	import type { Ask } from '../types.ts';

	let { ctx, a, variant = 'full', headingId }: { ctx: ApprovalsCtx; a: Ask; variant?: 'full' | 'slim' | 'stage'; headingId?: string } = $props();

	$effect(() => ctx.want(a));
	const d = $derived(ctx.details(a));
	const info = $derived(ctx.info(a));
	const who = $derived(ctx.who(a));
	const art = $derived(d?.backdrop ?? a.poster ?? '');
	const facts = $derived([d?.year ? String(d.year) : a.year ? String(a.year) : '', d?.rating ?? '', ...(d?.genres ?? []).slice(0, 2)].filter(Boolean));
	const rules = $derived(ctx.rulesFor(a));
	const defaultName = $derived(ctx.defaultRule(a));
	const music = $derived(a.kind === 'artist');
	// The decision, starting from the person's own picks (asks carry none yet: the defaults).
	let rulesId = $state(0);
	let lang = $state('');
	let note = $state('');
	$effect(() => {
		void a.id;
		rulesId = 0;
		lang = '';
		note = '';
	});
	const waiting = $derived(ctx.approver && a.state === 'asked' && !ctx.isPending(a.id));
	const original = $derived(info?.originalLanguage ?? '');
	const langList = $derived(original ? [{ code: original, name: `${languageName(original)} (original)` }, ...audioLanguages.filter((l) => l.code !== original)] : audioLanguages);
	const ruleWords = $derived(rulesId ? (rules.find((r) => r.id === rulesId)?.name ?? 'Default rule') : 'Default rule');
	const picks = $derived([ruleWords, lang ? languageName(lang) : ''].filter(Boolean).join(' · '));
	function approve(e: SubmitEvent) {
		e.preventDefault();
		ctx.approveWith(a, { rulesId, langs: lang && !music ? [lang] : [], note: note.trim() });
	}
</script>

<article class="panel {variant}" data-testid="ask-detail">
	{#if variant !== 'slim'}
		<div class="hero" class:square={music}>
			{#if art}<div class="bd" style:background-image={`url("${to(art)}")`} aria-hidden="true"></div>{/if}
			<div class="hero-in">
				<div class="poster"><Poster src={a.poster} name={a.name} /></div>
				<div class="head">
					<span class="tag on-art">{kindWord(a.kind)}</span>
					<h2 class="title" id={headingId} tabindex="-1">{#if ctx.href(a)}<a href={to(ctx.href(a))}>{a.name}</a>{:else}{a.name}{/if}</h2>
					{#if facts.length}<p class="facts">{facts.join(' · ')}</p>{/if}
				</div>
			</div>
		</div>
	{:else}
		<div class="slim-head">
			<div class="sposter"><Poster src={a.poster} name={a.name} /></div>
			<div class="head">
				<span class="tag">{kindWord(a.kind)}</span>
				<h2 class="title" id={headingId} tabindex="-1">{#if ctx.href(a)}<a href={to(ctx.href(a))}>{a.name}</a>{:else}{a.name}{/if}</h2>
				{#if facts.length}<p class="facts">{facts.join(' · ')}</p>{/if}
			</div>
		</div>
	{/if}

	<div class="body">
		<!-- The Posters arrangement shows the summary and pills in its rows already. -->
		{#if variant !== 'slim'}<AskSummary {ctx} {a} pills />{/if}
		<dl class="kv">
			<dt>Rule</dt><dd data-testid="ask-rule">Default rule{defaultName ? ` (${defaultName})` : ''}</dd>
			<dt>Asked by</dt>
			<dd class="who"><Avatar name={who.name} src={who.avatar} size={20} /><span>{who.name}</span><KidMark label={who.kid} size="sm" />{#if a.people.length > 1}<span class="muted">+{a.people.length - 1}</span>{/if}</dd>
			<dt>When</dt><dd><time datetime={a.asked_at} title={dateTime(a.asked_at)}>{ago(a.asked_at)}</time></dd>
			{#if ctx.decided(a)}<dt>Decision</dt><dd data-testid="decided-by">{ctx.decided(a)}{#if a.decided_at}{' '}· {ago(a.decided_at)}{/if}</dd>{/if}
		</dl>
		{#if a.state !== 'asked'}<div><StatusPill kind={ctx.pill(a).kind} label={ctx.pill(a).label} /></div>{/if}
		{#if a.note}<p class="note">“{a.note}”</p>{/if}
		{#if a.state === 'approved'}<p class="muted small">{ctx.approvedLine(a)}</p>{/if}
		{#if ctx.isPending(a.id)}<p class="muted small">Sending in a moment…</p>{/if}
		{#if variant !== 'slim' && d?.overview}<p class="overview">{d.overview}</p>{/if}

		{#if waiting}
			<form class="decide" onsubmit={approve}>
				<div class="two" class:one={music}>
					<div class="field">
						<label for="ap-rule-{a.id}">Rule</label>
						<select id="ap-rule-{a.id}" class="input" bind:value={rulesId} data-testid="ask-rule-pick">
							<option value={0}>Default rule{defaultName ? ` (${defaultName})` : ''}</option>
							{#each rules as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
						</select>
					</div>
					{#if !music}
						<div class="field">
							<label for="ap-lang-{a.id}">Language</label>
							<select id="ap-lang-{a.id}" class="input" bind:value={lang} data-testid="ask-lang-pick">
								<option value="">Any (the rule decides)</option>
								{#each langList as l (l.code)}<option value={l.code}>{l.name}</option>{/each}
							</select>
						</div>
					{/if}
				</div>
				<div class="field">
					<label for="ap-note-{a.id}">Note to {who.name} <span class="muted">(optional)</span></label>
					<textarea id="ap-note-{a.id}" class="input" rows="2" maxlength="500" bind:value={note} placeholder="They see it with the answer"></textarea>
				</div>
				<div class="acts">
					<Button variant="primary" icon="check" type="submit" data-testid="ask-approve">Approve</Button>
					<Button icon="x" type="button" data-testid="ask-decline" onclick={() => ctx.turnDownWith(a, note.trim())}>Turn down</Button>
				</div>
				<p class="muted small">Approves with {picks}. You can undo for 6 seconds.</p>
			</form>
		{/if}
		{#if a.mine && a.state === 'asked'}
			<div class="acts"><Button variant="plain" icon="trash" type="button" onclick={() => ctx.takeBack(a)}>Take back</Button></div>
		{/if}
	</div>
</article>

<style>
	.panel { display: flex; flex-direction: column; min-width: 0; }
	.hero { position: relative; isolation: isolate; overflow: hidden; border-radius: var(--r-panel); background: var(--surface-2); min-height: 180px; display: flex; align-items: flex-end; }
	.stage .hero { min-height: 340px; }
	.bd { position: absolute; inset: 0; z-index: -2; background-size: cover; background-position: center 25%; }
	.square .bd { filter: blur(26px) saturate(1.3); transform: scale(1.2); opacity: 0.75; }
	.hero::after { content: ''; position: absolute; inset: 0; z-index: -1; background: linear-gradient(180deg, var(--on-art-scrim-none) 0%, var(--on-art-scrim-mid) 45%, var(--on-art-scrim) 100%); }
	.hero-in { display: flex; align-items: flex-end; gap: 16px; padding: 16px; width: 100%; color: var(--on-art); }
	.poster { width: 84px; flex: none; border-radius: 8px; overflow: hidden; box-shadow: 0 10px 30px rgb(0 0 0 / 40%); }
	.stage .poster { width: 132px; }
	.square .poster { --poster-ratio: 1; }
	.head { min-width: 0; display: flex; flex-direction: column; align-items: flex-start; gap: 4px; }
	.tag { padding: 1px 8px; border-radius: var(--r-sm); background: var(--surface-2); color: var(--text-muted); font: var(--t-caption); font-weight: 600; }
	.tag.on-art { background: rgb(255 255 255 / 18%); color: var(--on-art); -webkit-backdrop-filter: blur(10px); backdrop-filter: blur(10px); }
	.title { font: var(--t-title3); letter-spacing: var(--t-title3-track); overflow-wrap: anywhere; scroll-margin-top: calc(var(--topbar) + 16px); }
	.stage .title { font: var(--t-title1); letter-spacing: var(--t-title1-track); }
	.title a:hover { text-decoration: underline; text-underline-offset: 4px; }
	.title:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 6px; }
	.facts { color: var(--on-art-muted); font: var(--t-footnote); overflow-wrap: anywhere; }
	.slim-head { display: flex; gap: 12px; align-items: flex-start; }
	.slim-head .facts { color: var(--text-muted); }
	.sposter { width: 56px; flex: none; border-radius: 6px; overflow: hidden; }
	.body { display: flex; flex-direction: column; gap: 12px; padding: 14px 2px 2px; }
	.kv { display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: 6px 14px; margin: 0; padding: 10px 12px; border-radius: var(--r-card); background: var(--surface-2); font: var(--t-footnote); }
	.kv dt { color: var(--text-muted); }
	.kv dd { margin: 0; min-width: 0; overflow-wrap: anywhere; }
	.kv .who { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
	.note { padding: 10px 12px; border-radius: 10px; background: var(--surface-2); font: var(--t-callout); overflow-wrap: anywhere; }
	.overview { font: var(--t-callout); color: var(--text-muted); display: -webkit-box; -webkit-line-clamp: 4; line-clamp: 4; -webkit-box-orient: vertical; overflow: hidden; }
	.decide { display: flex; flex-direction: column; gap: 12px; padding-top: 12px; border-top: 1px solid var(--line); }
	.two { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
	.two.one { grid-template-columns: 1fr; }
	.slim .two { grid-template-columns: 1fr; }
	.decide .field + .field { margin-top: 0; }
	.decide select { min-width: 0; width: 100%; }
	textarea.input { height: auto; padding-top: 10px; resize: vertical; }
	.acts { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; }
	.muted { color: var(--text-muted); }
	.small { font: var(--t-footnote); }
	@media (max-width: 420px) {
		.two { grid-template-columns: 1fr; }
		.poster { width: 72px; }
		.stage .poster { width: 96px; }
		.stage .title { font: var(--t-title2); }
	}
</style>
