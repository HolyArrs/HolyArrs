<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// Choices for a download that needs a decision: one reason, one primary fix, the rest under More, and the plan behind Why. A season pack or album gets the same shape for the whole pack.
	import Button from '#lib/ui/Button.svelte';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import { decisionName, groupDecisions, manyWords, whyWords, type DecisionGroup } from './downloaders.ts';
	import { refreshDecisions } from './decisions.svelte.ts';
	import { editionDecisionBody } from '../mediafiles/index.ts';
	import { to } from '../core/paths.ts';
	import { api, message } from '../core/api.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import { ago } from '../core/format.ts';
	import { type Decision } from '../core/types.ts';
	import { session } from '../core/session.svelte.ts';
	import { plainReason } from '../core/plain.ts';

	// `compact` hides the title, which the Download details header already shows.
	let { items, ondone, compact = false }: { items: Decision[]; ondone?: () => void; compact?: boolean } = $props();
	const groups = $derived(groupDecisions(items));
	const admin = $derived(session.can('settings'));

	type Action = 'search' | 'turn_down' | 'stop';
	let busy = $state('');
	async function act(d: Decision, action: Action, pack = false, key = `slot-${d.slot_id}`) {
		const name = decisionName(d);
		const w = manyWords(d.kind);
		const whole = d.kind === 'artist' ? t('downloads.dp.artist') : t('downloads.noun.show');
		const episodes = pack ? t('downloads.dp.these', { many: w?.many ?? '' }) : w ? name : d.title;
		const ask = {
			search: '',
			turn_down: pack
				? t('downloads.dp.askPack', { release: d.release_name ?? t('downloads.dp.thePack', { pack: w?.pack ?? '' }), one: w?.one ?? '', many: w?.many ?? '' })
				: w
					? t('downloads.dp.askOne', { release: d.release_name ?? t('downloads.dp.lastRelease'), name, many: w.many })
					: t('downloads.dp.askPlain', { release: d.release_name ?? t('downloads.dp.lastRelease') }),
			stop: w
				? t('downloads.dp.stopMany', { episodes, each: pack ? t('downloads.dp.eachOfThem') : t('downloads.dp.thisOne', { one: w.one }), whole })
				: t('downloads.dp.stopPlain', { name, version: d.version })
		}[action];
		if (ask && !confirm(ask)) return;
		busy = key;
		try {
			await api.post(`/needs-decision/${d.slot_id}/decide`, pack ? { action, pack: true } : { action });
			toasts.push({ search: t('downloads.dp.searchingAgain', { episodes }), turn_down: t('downloads.dp.searchingAgain', { episodes }), stop: t('downloads.dp.stopped', { episodes }) }[action], 'ok', action === 'turn_down' ? { body: t('downloads.dp.turnedDown') } : {});
			await refreshDecisions();
			ondone?.();
		} catch (e) {
			toasts.push(t('downloads.dp.saveFailed'), 'fail', { body: message(e) });
		} finally {
			busy = '';
		}
	}

	type Fix = 'edition' | 'hand' | 'search';
	const isEdition = (d: Decision) => d.kind === 'movie' && !!d.download_id && (d.why === 'conflict' || /edition/i.test(d.reason ?? ''));
	function primary(d: Decision): Fix {
		if (isEdition(d)) return 'edition';
		if (d.why === 'conflict') return 'hand';
		return 'search';
	}
	function reasonLine(d: Decision): string {
		if (isEdition(d)) return t('downloads.dp.editionUnknown');
		const r = d.reason ? plainReason(d.reason).text : '';
		return r || whyWords(d);
	}
	function more(d: Decision, key: string, fix: Fix): MenuItem[] {
		const out: MenuItem[] = [];
		if (fix !== 'search') out.push({ label: t('downloads.dp.searchAgain'), onselect: () => act(d, 'search', false, key) });
		if (fix !== 'hand') out.push({ label: t('downloads.dp.byHand'), href: to(`/library/${d.title_id}?tab=library&pick=1#h-rel`) });
		if (!manyWords(d.kind)) out.push({ label: d.versions > 1 ? t('downloads.dp.anotherVersion') : t('downloads.dp.addVersion'), href: to(`/library/${d.title_id}?tab=library#h-versions`) });
		if (d.release_id) out.push({ label: t('downloads.dp.addTurnedDown'), onselect: () => act(d, 'turn_down', false, key) });
		out.push({ label: t('downloads.dp.stopLooking'), destructive: true, onselect: () => act(d, 'stop', false, key) });
		out.push({ kind: 'separator' }, { label: t('downloads.dp.openTitle'), href: to(`/library/${d.title_id}`) });
		return out;
	}
	function packMore(d: Decision, key: string): MenuItem[] {
		const w = manyWords(d.kind);
		return [
			...(d.release_id ? [{ label: t('downloads.dp.addPackTurnedDown', { kind: w?.pack === w?.pack && d.kind === 'artist' ? t('downloads.dp.albumWord') : t('downloads.dp.pack') }), onselect: () => act(d, 'turn_down', true, key) }] : []),
			{ label: t('downloads.dp.stopThese', { many: w?.many ?? '' }), destructive: true, onselect: () => act(d, 'stop', true, key) },
			{ kind: 'separator' },
			{ label: t('downloads.dp.openTitle'), href: to(`/library/${d.title_id}`) }
		];
	}

	let whyOpen = $state<Record<string, boolean>>({});
	let editOpen = $state<Record<string, boolean>>({});
	let edition = $state<Record<string, string>>({});
	let edError = $state<Record<string, string>>({});
	let done = $state<Record<string, string>>({});
	// The server has no undo for an edition, so the status line is the only confirmation.
	async function saveEdition(e: SubmitEvent, d: Decision, key: string) {
		e.preventDefault();
		const v = edition[key]?.trim();
		if (!v || !d.download_id) return;
		busy = key;
		edError[key] = '';
		try {
			await api.put(`/downloads/${d.download_id}/edition`, editionDecisionBody(v));
			done[key] = t('downloads.dp.editionSet');
			await refreshDecisions();
			ondone?.();
		} catch (err) {
			edError[key] = plainReason(message(err)).text;
		} finally {
			busy = '';
		}
	}

	function packLine(g: DecisionGroup): string {
		const all = g.lead.pack ?? [];
		const w = manyWords(g.lead.kind);
		return t('downloads.dp.packLine', { n: g.items.length, total: all.length, many: w ? w.many : t('downloads.many.episodes') });
	}
</script>

{#snippet whyBtn(key: string)}
	<button class="whyb" type="button" aria-expanded={!!whyOpen[key]} aria-controls="why-{key}" onclick={() => (whyOpen[key] = !whyOpen[key])} data-testid="decision-why">{t('downloads.dp.why')}</button>
{/snippet}

{#snippet why(d: Decision, key: string)}
	{@const r = d.reason ? plainReason(d.reason) : null}
	<div class="raw" id="why-{key}" hidden={!whyOpen[key]} data-testid="decision-why-box">
		<p>{whyWords(d)}</p>
		{#if r && r.text !== reasonLine(d)}<p>{r.text}</p>{/if}
		{#if d.has_copy}<p>{t('downloads.dp.hasCopy')}</p>{/if}
		{#if admin && d.release_name}<p class="mono">{t('downloads.dp.releaseColon', { name: d.release_name })}</p>{/if}
		{#if admin && r?.detail}<p class="mono">{r.detail}</p>{/if}
	</div>
{/snippet}

{#snippet one(d: Decision, key: string, showName: boolean)}
	{@const fix = primary(d)}
	<div class="dec" data-testid="decision">
		<p class="line">
			{#if showName}<span class="one">{decisionName(d)}</span>{/if}
			<span class="why" data-testid="decision-reason">{reasonLine(d)}</span>
			<span class="sub">· {d.version} · {ago(d.since)}</span>
		</p>
		{#if done[key]}
			<p class="state" role="status" data-testid="decision-done">{done[key]}</p>
		{:else}
			<div class="acts">
				{#if fix === 'edition'}
					<Button size="sm" variant="primary" aria-expanded={!!editOpen[key]} aria-controls="ed-form-{key}" onclick={() => (editOpen[key] = !editOpen[key])} data-testid="decision-fix">{t('downloads.dp.setEdition')}</Button>
				{:else if fix === 'hand'}
					<Button size="sm" variant="primary" href={to(`/library/${d.title_id}?tab=library&pick=1#h-rel`)} data-testid="decision-fix">{t('downloads.dp.byHand')}</Button>
				{:else}
					<Button size="sm" variant="primary" onclick={() => act(d, 'search', false, key)} disabled={busy === key} busy={busy === key} data-testid="decision-fix">{t('downloads.dp.searchAgain')}</Button>
				{/if}
				<Menu label={t('downloads.dp.moreChoices')} variant="button" text={t('downloads.dp.more')} icon="chevronDown" items={more(d, key, fix)} width={240} align="start" />
				{@render whyBtn(key)}
			</div>
			{#if fix === 'edition' && editOpen[key]}
				<form class="ed" id="ed-form-{key}" onsubmit={(e) => saveEdition(e, d, key)} data-testid="edition-decision">
					<label class="sr-only" for="ed-{key}">{t('downloads.dp.edition')}</label>
					<input id="ed-{key}" class="input" bind:value={edition[key]} maxlength="80" required placeholder={t('downloads.dp.editionPh')} />
					<Button size="sm" variant="primary" type="submit" disabled={!edition[key]?.trim() || busy === key} busy={busy === key}>{t('downloads.dp.saveTry')}</Button>
				</form>
				{#if edError[key]}<p class="err" role="alert">{edError[key]}</p>{/if}
			{/if}
		{/if}
		{@render why(d, key)}
	</div>
{/snippet}

<section class="decide" class:compact aria-label={t('downloads.status.review')} data-testid="decision-panel">
	{#each groups as g (g.key)}
		{@const d = g.lead}
		{#if g.pack}
			{@const w = manyWords(d.kind)}
			<div class="dec" data-testid="decision-pack">
				<p class="line"><span class="why">{packLine(g)}.</span> <span class="sub">· {d.kind === 'artist' ? t('downloads.hp.album') : t('downloads.dp.seasonPack')} · {ago(d.since)}</span></p>
				<div class="acts">
					<Button size="sm" variant="primary" onclick={() => act(d, 'search', true, g.key)} disabled={busy === g.key} busy={busy === g.key}>{t('downloads.dp.searchThese', { many: w?.many ?? '' })}</Button>
					<Menu label={t('downloads.dp.moreChoices')} variant="button" text={t('downloads.dp.more')} icon="chevronDown" items={packMore(d, g.key)} width={260} align="start" />
					{@render whyBtn(g.key)}
				</div>
				{@render why(d, g.key)}
				<ul class="subs">
					{#each g.items as x (x.slot_id)}<li>{@render one(x, `slot-${x.slot_id}`, true)}</li>{/each}
				</ul>
			</div>
		{:else}
			{@render one(d, g.key, !compact)}
		{/if}
	{/each}
</section>

<style>
	.decide { display: flex; flex-direction: column; gap: 12px; padding-top: 12px; border-top: 1px solid var(--line); }
	.compact { gap: 10px; padding-top: 0; border-top: 0; }
	.dec { display: flex; flex-direction: column; gap: 6px; min-width: 0; padding-left: 10px; box-shadow: inset 3px 0 0 var(--wait); }
	.line { font: var(--t-subhead); overflow-wrap: anywhere; }
	.one { font-weight: 600; margin-right: 6px; }
	.sub { color: var(--text-muted); }
	.acts { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
	.whyb { min-height: 32px; padding: 0 8px; border: 0; background: none; color: var(--text-muted); font: var(--t-footnote); cursor: pointer; border-radius: 6px; }
	.whyb:hover, .whyb[aria-expanded='true'] { color: var(--text); }
	.whyb:hover { text-decoration: underline; text-underline-offset: 3px; }
	.whyb:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.raw { display: grid; gap: 4px; padding: 8px 10px; border-radius: var(--r-btn); background: var(--surface-2); color: var(--text-muted); font: var(--t-footnote); overflow-wrap: anywhere; }
	.raw[hidden] { display: none; }
	.mono { font-family: var(--font-mono); }
	.ed { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; }
	.ed .input { flex: 1 1 180px; min-width: 0; height: 32px; }
	.err { color: var(--fail-text); font: var(--t-footnote); }
	.state { color: var(--text-muted); font: var(--t-subhead); font-weight: 600; }
	.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
	.subs { list-style: none; padding: 0; margin: 0; display: grid; gap: 10px; }
	.subs > li { padding-top: 10px; border-top: 1px solid var(--line); min-width: 0; }
	.subs .dec { box-shadow: none; padding-left: 0; }
</style>
