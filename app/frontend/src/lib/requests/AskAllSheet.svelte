<script lang="ts">
	// Request all: the movies of one series asked for one after the other, with the same rules as asking for each by hand, then a short summary of what was requested and what was skipped and why. For people who manage the Library it is Add all: the same list, each movie added with the folder and Pick rules chosen once here.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Words from '#lib/ui/Words.svelte';
	import { to } from '../core/paths.ts';
	import type { ExploreCard } from '#lib/ui/title-cards.ts';
	import { api } from '../core/api.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import { refreshInbox } from '../core/inbox.svelte.ts';
	import { gateChoices, loadChoices, needsFolder, savedDefaults, startChoices } from '../mediafiles/index.ts';
	import { session } from '../core/session.svelte.ts';
	import type { LibraryFolder, PickRulesView, TitleView } from '../core/types.ts';
	import type { AskLimit } from './types.ts';
	import { countOf, joins, limitReason, lineWords, planAskAll, runAskAll, toSend, type AskAllLine, type AskAllPost } from './ask-all.ts';

	let { online, folders = [], rules = [], gateDefault = 'released', loaded = true, failed = false, onretry }: { online?: (l: AskAllLine) => void; folders?: LibraryFolder[]; rules?: PickRulesView[]; gateDefault?: string; /** False until the Library folders have answered, so the sheet does not say there is none while they load. */ loaded?: boolean; /** The Library folders or Pick rules did not load. */ failed?: boolean; onretry?: () => void } = $props();

	let sheetOpen = $state(false);
	let name = $state('');
	let lines = $state<AskAllLine[]>([]);
	let busy = $state(false);
	let reading = $state(false);
	let finished = $state(false);
	let limit = $state<AskLimit | null>(null);
	let adding = $state(false);
	let folderId = $state(0);
	let rulesId = $state(0);
	let keep = true;
	let searchOnAdd = true;
	let opened = 0;

	const movieFolders = $derived(folders.filter((f) => f.kind === 'movie'));
	// Hands-off mode needs no Library folder: each movie only goes to the Downloader.
	const noFolder = $derived(needsFolder(movieFolders.length, session.s.handsOff));
	const movieRules = $derived(rules.filter((r) => !r.kinds.length || r.kinds.includes('movie')));
	const gateOf = () => {
		const last = loadChoices().release_gate;
		return last && gateChoices.some(([g]) => g === last) ? last : gateDefault;
	};
	const post: AskAllPost = (body) => adding
		? api.post<{ title: TitleView }>('/titles', { ...body, library_folder_id: folderId, pick_rules_id: rulesId, release_gate: gateOf(), keep_up_to_date: keep, search_on_add: searchOnAdd, more_versions: [] })
		: api.post('/asks', body);
	const requested = $derived(countOf(lines, 'requested'));
	const skipped = $derived(countOf(lines, 'skipped'));
	const summary = $derived(adding ? (skipped ? t('requests.addAll.summary', { added: requested, skipped }) : t('requests.addAll.summaryClean', { added: requested })) : skipped ? t('requests.askAll.summary', { requested, skipped }) : t('requests.askAll.summaryClean', { requested }));
	const limited = $derived(!adding && limit && !limit.no_limit && !limit.movies.no_limit ? limit.movies : null);
	const noRoom = $derived(!!limited && limited.left <= 0);
	/** Nothing can be sent: no asks left and no movie that would join someone else's request. */
	const stuck = $derived(noRoom && !lines.some(joins));

	/** Shows the plan for a series' movies: asking for them, or (add) adding them to the Library. `fresh` is the same movies read again from the server: when it answers before anything is sent, the plan follows it, so a request made or turned down since the page loaded counts. */
	export function open(seriesName: string, cards: ExploreCard[], add = false, fresh?: Promise<ExploreCard[]>) {
		if (busy) {
			sheetOpen = true;
			return;
		}
		name = seriesName;
		adding = add;
		lines = planAskAll(cards, add);
		finished = false;
		limit = null;
		sheetOpen = true;
		const mine = ++opened;
		// Sending waits for the fresh read: a request someone else made since the page loaded is joined, not skipped.
		reading = !!fresh;
		fresh?.then((now) => { if (mine === opened && !busy && !finished) lines = planAskAll(now, add); }).catch(() => {}).finally(() => { if (mine === opened) reading = false; });
		if (add) {
			folderId = 0;
			rulesId = 0;
			void chooseStart(mine);
		}
		else void api.get<AskLimit>('/asks/limit').then((l) => { if (mine === opened && !busy) limit = l; }).catch(() => {});
	}

	/** The folder, Pick rules and switches an add starts from: the saved defaults, as the single add sheet does. A folder or Pick rules already chosen is kept. */
	async function chooseStart(mine: number) {
		const d = await savedDefaults.load();
		if (mine !== opened || busy) return;
		const start = startChoices(d, folders, rules, 'movie');
		if (!folderId) folderId = start.folderId;
		if (!rulesId) rulesId = start.rulesId;
		keep = start.keep;
		searchOnAdd = start.searchOnAdd;
	}

	async function go() {
		if (reading || busy || finished || !toSend(lines)) return;
		busy = true;
		try {
			lines = await runAskAll(lines, post, (l, i) => {
				lines[i] = l;
				online?.(l);
			}, noRoom);
		} finally {
			busy = false;
			finished = true;
			void refreshInbox();
		}
		// Closed meanwhile: the summary still arrives.
		if (!sheetOpen) toasts.push(summary, requested ? 'ok' : 'info');
	}

	function wordsOf(l: AskAllLine): string {
		const key = lineWords(l, adding, noRoom);
		return key ? t(key) : (l.reason ?? '');
	}

	// The folders and Pick rules can arrive after the sheet opens: the starting choices are made then.
	$effect(() => {
		if (sheetOpen && adding && !busy && !finished && (!folderId || !rulesId) && !noFolder && movieRules.length) void chooseStart(opened);
	});
</script>

<Sheet bind:open={sheetOpen} size="md" title={t(adding ? 'requests.addAll.title' : 'requests.askAll.title', { name })}>
	<div class="ask-all">
		<p class="summary" role="status" aria-live="polite">{#if finished}{summary}{/if}</p>
		{#if finished}
			{#if limitReason(lines)}<p class="hint">{limitReason(lines)}</p>{:else if limited && noRoom}<p class="hint">{t('discover.people.movieAsksThisRolling', { used: limited.used, left: limited.left })}</p>{/if}
		{:else}
			<p class="hint">{t(adding ? 'requests.addAll.intro' : 'requests.askAll.intro')}</p>
			{#if adding}
				{#if failed}
					<p class="hint failed" role="alert"><span>{t('library.import.foldersFailed')}</span><Button size="sm" onclick={() => onretry?.()}>{t('discover.titleLibrary.tryAgain')}</Button></p>
				{:else if !loaded}
					<!-- The Library folders have not answered yet: nothing is said about them. -->
				{:else if noFolder}
					<p class="hint"><Words key="requests.add.noFolder.movie" links={{ a: to('/settings/library-folders') }} linkClass="ul" /></p>
				{:else}
					{#if movieFolders.length}<div class="field">
						<label for="all-folder">{t('requests.add.folder')}</label>
						<select id="all-folder" class="input" bind:value={folderId}>
							{#each movieFolders as f (f.id)}<option value={f.id}>{f.label || f.path} ({f.path})</option>{/each}
						</select>
					</div>{:else}<p class="hint" data-testid="hands-off-hint">{t('requests.add.handsOff')}</p>{/if}
					<div class="field">
						<label for="all-rules">{t('requests.add.rules')}</label>
						<select id="all-rules" class="input" bind:value={rulesId}>
							{#each movieRules as r (r.id)}<option value={r.id}>{r.name}</option>{/each}
						</select>
					</div>
				{/if}
			{/if}
		{/if}
		<ul class="films">
			{#each lines as l (l.card.key)}
				<li class:server={l.why === 'server'} class:done={l.state === 'requested'}>
					<span class="film">{l.card.name}{#if l.card.year}{' '}<span class="year">({l.card.year})</span>{/if}</span>
					<span class="why">{wordsOf(l)}</span>
				</li>
			{/each}
		</ul>
		{#if limited && !finished}<p class="hint">{t('discover.people.movieAsksThisRolling', { used: limited.used, left: limited.left })}</p>{/if}
	</div>
	{#snippet footer()}
		{#if finished}
			<Button variant="primary" onclick={() => (sheetOpen = false)}>{t('ui.sheet.close')}</Button>
		{:else}
			<Button variant="plain" onclick={() => (sheetOpen = false)}>{t('requests.add.cancel')}</Button>
			<Button variant="primary" {busy} disabled={reading || !toSend(lines) || stuck || (adding && ((!folderId && movieFolders.length > 0) || noFolder || !rulesId))} onclick={go}>{t(adding ? 'requests.addAll.button' : 'requests.askAll.button')}</Button>
		{/if}
	{/snippet}
</Sheet>

<style>
	.ask-all { display: flex; flex-direction: column; gap: 12px; min-width: 0; }
	.hint { margin: 0; color: var(--text-muted); font: var(--t-callout); }
	.failed { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 12px; color: var(--wait-text); }
	.films { display: flex; flex-direction: column; margin: 0; padding: 0; list-style: none; border-radius: var(--r-btn); background: var(--surface-2); }
	.films li { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: baseline; gap: 2px 16px; padding: 10px 14px; }
	@media (max-width: 520px) { .films li { grid-template-columns: minmax(0, 1fr); } }
	.films li + li { border-top: 1px solid var(--line); }
	.film { min-width: 0; font: var(--t-headline); font-size: 15px; }
	.year { color: var(--text-muted); font-weight: 400; }
	.why { max-width: 22em; color: var(--text-muted); font: var(--t-callout); text-align: end; }
	@media (max-width: 520px) { .why { max-width: none; text-align: start; } }
	.server .why { color: var(--wait-text); }
	.done .why { color: var(--ok-text); }
	.summary { margin: 0; font: var(--t-headline); font-size: 15px; }
	.summary:empty { display: none; }
</style>
