<script lang="ts">
	// The choices for a download that needs a decision, shown in its details
	// sheet on Downloads: why it waits, and Search again, Choose a release by
	// hand, another Version, Turned down or Stop looking (a whole season pack
	// or album at once, or one episode or track). The same choices and calls
	// the separate Needs a decision page had.
	import { to } from '../paths.ts';
	import { api, message } from '../api.ts';
	import { toasts } from '../toasts.svelte.ts';
	import { ago } from '../format.ts';
	import { decisionName, groupDecisions, manyWords, whyWords, type Decision, type DecisionGroup } from '../downloaders.ts';
	import { refreshDecisions } from '../decisions.svelte.ts';
	import { session } from '../session.svelte.ts';
	import { plainReason } from '../plain.ts';
	import Button from '../ui/Button.svelte';
	import EditionDecision from '../EditionDecision.svelte';

	// `compact`: the Download details views put the choices in one wrapping row
	// of small buttons instead of a full-width stack.
	let { items, ondone, compact = false }: { items: Decision[]; ondone?: () => void; compact?: boolean } = $props();
	const sz = $derived(compact ? 'sm' : 'md');
	const groups = $derived(groupDecisions(items));
	const admin = $derived(session.can('settings'));

	type Action = 'search' | 'turn_down' | 'stop';
	let busy = $state('');
	async function act(d: Decision, action: Action, pack = false, key = `slot-${d.slot_id}`) {
		const name = decisionName(d);
		const w = manyWords(d.kind);
		const whole = d.kind === 'artist' ? 'artist' : 'show';
		const episodes = pack ? `these ${w?.many}` : w ? name : d.title;
		const ask = {
			search: '',
			turn_down: pack
				? `Add ${d.release_name ?? `the ${w?.pack}`} to Turned down for every ${w?.one} it holds and search again for these ${w?.many}?`
				: w
					? `Add ${d.release_name ?? 'its last Release'} to Turned down for ${name} and search again? The Release stays usable for other ${w.many}.`
					: `Add ${d.release_name ?? 'its last Release'} to Turned down and search again?`,
			stop: w
				? `Stop looking for ${episodes}? Keep up to date is turned off for ${pack ? 'each of them' : `this ${w.one}`}; the rest of the ${whole} goes on, and files on disk are not touched.`
				: `Stop looking for ${name} (${d.version})? Keep up to date is turned off for this Version; files on disk are not touched.`
		}[action];
		if (ask && !confirm(ask)) return;
		busy = key;
		try {
			await api.post(`/needs-decision/${d.slot_id}/decide`, pack ? { action, pack: true } : { action });
			toasts.push({ search: `Searching again for ${episodes}.`, turn_down: `Turned down. Searching again for ${episodes}.`, stop: `Stopped looking for ${episodes}.` }[action], 'ok');
			await refreshDecisions();
			ondone?.();
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = '';
		}
	}

	function packLine(g: DecisionGroup): string {
		const all = g.lead.pack ?? [];
		const w = manyWords(g.lead.kind);
		return `${g.items.length} of the ${all.length} ${w ? w.many : 'episodes'} in this download need a decision`;
	}
</script>

{#snippet cause(d: Decision, bold: boolean)}
	{@const r = d.reason ? plainReason(d.reason) : null}
	<div class="cause">
		<p class="why" data-testid="decision-why">{#if bold}<b>{whyWords(d)}</b>{:else}{whyWords(d)}{/if}</p>
		{#if r}<p class="reason" data-testid="decision-reason">{r.text}</p>{/if}
		{#if admin && (r?.detail || d.release_name)}
			<details class="raw">
				<summary>Details</summary>
				{#if d.release_name}<p>Last Release: {d.release_name}</p>{/if}
				{#if r?.detail}<p>{r.detail}</p>{/if}
			</details>
		{/if}
	</div>
{/snippet}

{#snippet choices(d: Decision, key: string)}
	{#if d.kind === 'movie' && d.download_id}<EditionDecision downloadId={d.download_id} onchange={refreshDecisions} />{/if}
	<div class="acts">
		<Button size={sz} variant="primary" onclick={() => act(d, 'search', false, key)} disabled={busy === key} busy={busy === key}>Search again</Button>
		<Button size={sz} variant="secondary" href={to(`/library/${d.title_id}#h-rel`)}>Choose a release by hand</Button>
		{#if !manyWords(d.kind)}
			<Button size={sz} variant="secondary" href={to(`/library/${d.title_id}#h-versions`)}>{d.versions > 1 ? 'Try another Version' : 'Add another Version'}</Button>
		{/if}
		{#if d.release_id}
			<Button size={sz} variant="plain" onclick={() => act(d, 'turn_down', false, key)} disabled={busy === key}>Add to Turned down</Button>
		{/if}
		<Button size={sz} variant="plain" onclick={() => act(d, 'stop', false, key)} disabled={busy === key}>Stop looking</Button>
	</div>
{/snippet}

<section class="decide" class:compact aria-label="Needs a decision" data-testid="decision-panel">
	{#each groups as g (g.key)}
		{@const d = g.lead}
		{#if g.pack}
			{@const w = manyWords(d.kind)}
			<div class="dec" data-testid="decision-pack">
				<p class="sub">{d.kind === 'artist' ? 'Album' : 'Season pack'} · {packLine(g)} · waiting {ago(d.since)}</p>
				{#if admin && d.release_name}<details class="raw"><summary>Details</summary><p>{d.kind === 'artist' ? 'Album' : 'Pack'}: {d.release_name}</p></details>{/if}
				<div class="acts">
					<Button size={sz} variant="primary" onclick={() => act(d, 'search', true, g.key)} disabled={busy === g.key} busy={busy === g.key}>Search again for these {w?.many}</Button>
					{#if d.release_id}
						<Button size={sz} variant="plain" onclick={() => act(d, 'turn_down', true, g.key)} disabled={busy === g.key}>Add the {w?.pack === 'album' ? 'album' : 'pack'} to Turned down</Button>
					{/if}
					<Button size={sz} variant="plain" onclick={() => act(d, 'stop', true, g.key)} disabled={busy === g.key}>Stop looking for these {w?.many}</Button>
				</div>
				<ul class="subs">
					{#each g.items as one (one.slot_id)}
						<li data-testid="decision">
							<p class="one">{decisionName(one)}</p>
							{@render cause(one, false)}
							{@render choices(one, `slot-${one.slot_id}`)}
						</li>
					{/each}
				</ul>
			</div>
		{:else}
			<div class="dec" data-testid="decision">
				<!-- In the compact details the title is already in the header above. -->
				{#if !compact}<a class="one" href={to(`/library/${d.title_id}`)}>{decisionName(d)}</a>{/if}
				<p class="sub">{d.version} Version · waiting {ago(d.since)}{#if d.has_copy} · a copy is already in the Library{/if}</p>
				{@render cause(d, true)}
				{@render choices(d, g.key)}
			</div>
		{/if}
	{/each}
</section>

<style>
	.decide { display: flex; flex-direction: column; gap: 14px; padding-top: 12px; border-top: 1px solid var(--line); }
	.dec { display: flex; flex-direction: column; gap: 10px; min-width: 0; }
	.sub { color: var(--text-muted); font: var(--t-subhead); font-weight: 400; }
	.cause { padding: 12px 14px; border-radius: var(--r-btn); background: var(--surface-2); box-shadow: inset 3px 0 0 var(--wait); display: flex; flex-direction: column; gap: 4px; overflow-wrap: anywhere; }
	.why { font: var(--t-callout); }
	.reason { color: var(--text-muted); font: var(--t-callout); }
	.raw { color: var(--text-muted); font: var(--t-footnote); }
	.raw summary { width: max-content; min-height: 24px; cursor: pointer; border-radius: 6px; }
	.raw summary:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.raw p { margin-top: 4px; font-family: var(--font-mono); overflow-wrap: anywhere; }
	/* In a sheet the buttons stack full width. */
	.acts { display: grid; gap: 8px; }
	.compact { gap: 10px; padding-top: 0; border-top: 0; }
	.compact .dec { gap: 8px; }
	.compact .acts { display: flex; flex-wrap: wrap; gap: 6px; }
	.compact .cause { padding: 8px 12px; }
	.compact .why, .compact .reason { font: var(--t-subhead); }
	.subs { list-style: none; padding: 0; margin: 0; display: grid; gap: 14px; }
	.subs > li { display: flex; flex-direction: column; gap: 8px; padding-top: 14px; border-top: 1px solid var(--line); min-width: 0; }
	.one { color: var(--text); font: var(--t-headline); font-size: 15px; overflow-wrap: anywhere; }
	a.one:hover { text-decoration: underline; text-underline-offset: 3px; }
</style>
