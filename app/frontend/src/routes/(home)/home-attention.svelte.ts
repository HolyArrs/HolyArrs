// Home's setup steps and what needs attention.
import { decisions } from '#lib/downloads/index.ts';
import { attention, groupAttention, homeFailures, libraryCards, moreLink, n, partCount, shownState, type Attention, type AttentionGroup } from './home.ts';
import { metadataDownText, metadataOutage } from '#lib/metadata/index.ts';
import type { Tint } from '#lib/core/settings.ts';

import type { HomeContext } from './home-context.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export function createHomeAttention(ctx: Pick<HomeContext, 'checks' | 'health' | 'settled' | 'uncheckedParts' | 'folders' | 'owner' | 'rows' | 'admin' | 'downloads' | 'manager' | 'uncheckedDl' | 'uncheckedHealth' | 'home' | 'asks'>) {
	const failingChecks = (kind: string) => (ctx.checks?.checks ?? []).filter((c) => c.kind === kind && shownState(c) !== 'ok');
	const srcs = $derived(partCount(ctx.health?.sources, failingChecks('source').map((c) => c.subject)));
	const dls = $derived(partCount(ctx.health?.downloaders, failingChecks('downloader').map((c) => c.subject)));
	const partsRead = $derived(ctx.settled && !!ctx.health && !ctx.uncheckedParts);
	const steps = $derived([
		{ done: ctx.folders.length > 0, text: t('home.chooseALibraryFolder'), detail: t('home.tellHolyarrsWhereFinished'), href: '/settings/library-folders', cta: t('home.chooseAFolder') },
		{ done: srcs.total > 0, text: t('home.addASource'), detail: t('home.connectAPlaceWhere'), href: '/settings/sources', cta: t('home.addASource') },
		{ done: dls.total > 0, text: t('home.addADownloader'), detail: t('home.connectTheAppThat'), href: '/settings/downloaders', cta: t('home.addADownloader') },
		{ done: false, text: t('home.addYourFirstTitle'), detail: t('home.searchForSomethingNew'), href: '/search', cta: t('home.findATitle') }
	]);
	const doneCount = $derived(steps.filter((s) => s.done).length);
	const nextStep = $derived(steps.find((s) => !s.done) ?? steps[3]);

	/** Which parts have a problem, in one short line: the name when it is one ("SABnzbd has a problem"), else how many. Needs attention names them all. */
	function troubled(kind: 'source' | 'downloader', parts: { name: string; state: string }[] | null | undefined): string {
		const names = new Set<string>();
		for (const p of parts ?? []) if (!['ok', 'working', 'enabled', 'disabled'].includes(p.state) && !/ \(removed \d+\)$/.test(p.name)) names.add(p.name);
		for (const c of failingChecks(kind)) if (c.subject) names.add(c.subject);
		const list = [...names];
		if (!list.length) return '';
		return list.length === 1 ? t('home.hasAProblem2', { name: list[0] }) : t('home.haveAProblem2', { n: n(list.length) });
	}
	const partLine = (p: { total: number; off: number }, trouble: string) => trouble || (p.off ? t('home.turnedOff', { n: n(p.off) }) : t('home.allWorking'));
	// Nothing can be found or fetched without a Source and a Downloader that are on. The health checks only cover the ones that exist, so Home adds these itself once setup is over (the Welcome card covers it before that).
	const setupGaps = $derived.by((): Attention[] => {
		if (!ctx.owner || !partsRead || ctx.rows.includes('welcome')) return [];
		const gap = (key: string, title: string, reason: string, path: string, action: string): Attention => ({ key, tone: 'wait', word: t('home.warning'), text: t('home.text', { title: title, reason: reason }), title, reason, cause: `setup:${key}`, href: ctx.admin ? path : '/health', action: ctx.admin ? action : t('home.look') });
		const out: Attention[] = [];
		if (!srcs.total) out.push(gap('no-sources', t('home.sources'), t('home.noneAddedYet'), '/settings/sources', t('home.add')));
		else if (!srcs.on) out.push(gap('sources-off', t('home.sources'), t('home.allTurnedOff'), '/settings/sources', t('home.fix')));
		if (!dls.total) out.push(gap('no-downloaders', t('home.downloaders'), t('home.noneAddedYet'), '/settings/downloaders', t('home.add')));
		else if (!dls.on) out.push(gap('downloaders-off', t('home.downloaders'), t('home.allTurnedOff'), '/settings/downloaders', t('home.fix')));
		// The health checks run every few minutes; /health is read now. A part it already reports as failing gets its row at once.
		if (srcs.failing && !failingChecks('source').length) out.push(gap('sources-trouble', srcs.failing === 1 ? t('home.source') : t('home.sources2', { n: n(srcs.failing) }), srcs.failing === 1 ? t('home.hasAProblem') : t('home.haveAProblem'), '/settings/sources', t('home.fix')));
		if (dls.failing && !failingChecks('downloader').length) out.push(gap('downloaders-trouble', dls.failing === 1 ? t('home.downloader') : t('home.downloaders2', { n: n(dls.failing) }), dls.failing === 1 ? t('home.hasAProblem') : t('home.haveAProblem'), '/settings/downloaders', t('home.fix')));
		return out;
	});
	// Needs attention is what needs you: failed downloads or imports (the same rule as Downloads), decisions, health problems and a metadata outage while it lasts. Decisions come from the list the layout keeps fresh.
	const fixes = $derived(
		ctx.owner
			? [
					...attention({
						failed: homeFailures(ctx.downloads),
						decisions: ctx.manager ? decisions.items : [],
						health: ctx.checks?.checks ?? [],
						metadata: metadataOutage.s.down ? metadataDownText() : undefined,
						admin: ctx.admin
					}),
					...setupGaps
				]
			: []
	);
	const unchecked = $derived(ctx.owner && (ctx.uncheckedDl || ctx.uncheckedHealth || ctx.uncheckedParts));
	// How bad it is, for the colour: a failure is red, a warning (an update, a low disk) is amber.
	const worst = $derived(fixes.some((f) => f.tone === 'fail') ? 'fail' : fixes.length || unchecked ? 'wait' : 'ok');
	const attTint = $derived<Tint>(!ctx.settled ? 'gray' : worst === 'fail' ? 'red' : worst === 'wait' ? 'amber' : 'green');
	// Items that share a cause are one row ("2 downloads failed").
	const fixRows = $derived(groupAttention(fixes));
	/** One row in a few words: "6 downloads failed", "2 Sources have a problem", "Downloader: SABnzbd not answering". */
	const rowLine = (g: AttentionGroup) => {
		const cause = g.items[0].cause;
		if (g.items.length > 1) {
			if (!/^(health|setup):/.test(cause)) return g.title;
			// The System row names the parts; the tile keeps "2 Sources have a problem".
			const names = g.items.map((i) => i.subject).filter(Boolean).join(', ');
			return `${g.title} ${names && g.reason.startsWith(`${names} `) ? g.reason.slice(names.length + 1) : g.reason}`;
		}
		// A folder's path is on the System row; the tile keeps the words.
		return `${g.title}: ${/^health:(library_folder|disk_space):/.test(cause) ? g.reason.split(': ')[0] : g.reason}`;
	};
	// The Home tile names the worst row and says how many more things there are, so the number adds up.
	const fixLine = $derived(fixRows.length ? rowLine(fixRows[0]) : '');
	const fixMore = $derived(fixRows.length ? fixes.length - fixRows[0].items.length : 0);
	// The Needs attention tile opens the page of the row it names (the worst one).
	const fixHref = $derived(fixRows[0]?.href ?? (ctx.admin ? '/settings/health' : '/health'));
	// System lists each cause as a row; four fit, the rest link to the page that holds most of them.
	const fixShown = $derived(fixRows.slice(0, 4));
	const fixHidden = $derived(moreLink(fixRows.slice(4), ctx.admin));
	// What passed, for the all-good state: each kind of check that has no problem, once.
	const checkWords = (): Record<string, string> => ({ library_folder: t('home.libraryFolders'), disk_space: t('home.diskSpace'), source: t('home.sources'), downloader: t('home.downloaders'), backups: t('home.backups'), update: t('home.updates'), config_disk: t('home.configFolder'), config_folder: t('home.configFolder'), waiting_downloads: t('home.downloads'), trailers: t('home.trailers') });
	const passedCount = $derived((ctx.checks?.checks ?? []).filter((c) => shownState(c) === 'ok').length);
	const passed = $derived.by(() => {
		const words = checkWords();
		const bad = new Set((ctx.checks?.checks ?? []).filter((c) => shownState(c) !== 'ok').map((c) => words[c.kind]));
		return [...new Set((ctx.checks?.checks ?? []).map((c) => words[c.kind]).filter((w) => w && !bad.has(w)))];
	});
	// Your Library: one card per kind that is on, and for owners what it all takes on disk.
	const libCards = $derived(libraryCards(ctx.home?.library, ctx.manager ? '/downloads' : undefined));
	const diskTotal = $derived(libCards.reduce((t, c) => t + c.size, 0));
	const showDisk = $derived(ctx.owner && libCards.length > 0);
	const waiting = $derived(ctx.asks.filter((a) => a.state === 'asked'));
	const mine = $derived(ctx.asks.filter((a) => a.mine && a.state !== 'available'));
	const mineShown = $derived(mine.slice(0, 20));

	return {
		get mine() { return mine; },
		get mineShown() { return mineShown; },
		get libCards() { return libCards; },
		get showDisk() { return showDisk; },
		get waiting() { return waiting; },
		get diskTotal() { return diskTotal; },
		get worst() { return worst; },
		get fixes() { return fixes; },
		get unchecked() { return unchecked; },
		partLine,
		get fixHref() { return fixHref; },
		get attTint() { return attTint; },
		get fixMore() { return fixMore; },
		get fixLine() { return fixLine; },
		get fixShown() { return fixShown; },
		get fixHidden() { return fixHidden; },
		get passedCount() { return passedCount; },
		get passed() { return passed; },
		get srcs() { return srcs; },
		troubled,
		get dls() { return dls; },
		get doneCount() { return doneCount; },
		get steps() { return steps; },
		get nextStep() { return nextStep; },
	};
}
