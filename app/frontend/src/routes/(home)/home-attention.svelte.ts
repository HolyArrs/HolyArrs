// Home's setup steps and what needs attention.
import { decisions } from '#lib/downloads/index.ts';
import { attention, groupAttention, homeFailures, libraryCards, moreLink, n, partCount, shownState, type Attention, type AttentionGroup } from './home.ts';
import { METADATA_DOWN, metadataOutage } from '#lib/metadata/index.ts';
import type { Tint } from '#lib/core/settings.ts';

import type { HomeContext } from './home-context.ts';

export function createHomeAttention(ctx: Pick<HomeContext, 'checks' | 'health' | 'settled' | 'uncheckedParts' | 'folders' | 'owner' | 'rows' | 'admin' | 'downloads' | 'manager' | 'uncheckedDl' | 'uncheckedHealth' | 'home' | 'asks'>) {
	const failingChecks = (kind: string) => (ctx.checks?.checks ?? []).filter((c) => c.kind === kind && shownState(c) !== 'ok');
	const srcs = $derived(partCount(ctx.health?.sources, failingChecks('source').map((c) => c.subject)));
	const dls = $derived(partCount(ctx.health?.downloaders, failingChecks('downloader').map((c) => c.subject)));
	const partsRead = $derived(ctx.settled && !!ctx.health && !ctx.uncheckedParts);
	const steps = $derived([
		{ done: ctx.folders.length > 0, text: 'Choose a Library folder', detail: 'Tell HolyArrs where finished media should live.', href: '/settings/library-folders', cta: 'Choose a folder' },
		{ done: srcs.total > 0, text: 'Add a Source', detail: 'Connect a place where HolyArrs can find Releases.', href: '/settings/sources', cta: 'Add a Source' },
		{ done: dls.total > 0, text: 'Add a Downloader', detail: 'Connect the app that fetches chosen Releases.', href: '/settings/downloaders', cta: 'Add a Downloader' },
		{ done: false, text: 'Add your first title', detail: 'Search for something new, or import folders you already have.', href: '/search', cta: 'Find a title' }
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
		return list.length === 1 ? `${list[0]} has a problem` : `${n(list.length)} have a problem`;
	}
	const partLine = (p: { total: number; off: number }, trouble: string) => trouble || (p.off ? `${n(p.off)} turned off` : 'All working');
	// Nothing can be found or fetched without a Source and a Downloader that are on. The health checks only cover the ones that exist, so Home adds these itself once setup is over (the Welcome card covers it before that).
	const setupGaps = $derived.by((): Attention[] => {
		if (!ctx.owner || !partsRead || ctx.rows.includes('welcome')) return [];
		const gap = (key: string, title: string, reason: string, path: string, action: string): Attention => ({ key, tone: 'wait', word: 'Warning', text: `${title}: ${reason}.`, title, reason, cause: `setup:${key}`, href: ctx.admin ? path : '/health', action: ctx.admin ? action : 'Look' });
		const out: Attention[] = [];
		if (!srcs.total) out.push(gap('no-sources', 'Sources', 'none added yet', '/settings/sources', 'Add'));
		else if (!srcs.on) out.push(gap('sources-off', 'Sources', 'all turned off', '/settings/sources', 'Fix'));
		if (!dls.total) out.push(gap('no-downloaders', 'Downloaders', 'none added yet', '/settings/downloaders', 'Add'));
		else if (!dls.on) out.push(gap('downloaders-off', 'Downloaders', 'all turned off', '/settings/downloaders', 'Fix'));
		// The health checks run every few minutes; /health is read now. A part it already reports as failing gets its row at once.
		if (srcs.failing && !failingChecks('source').length) out.push(gap('sources-trouble', srcs.failing === 1 ? 'Source' : `${n(srcs.failing)} Sources`, srcs.failing === 1 ? 'has a problem' : 'have a problem', '/settings/sources', 'Fix'));
		if (dls.failing && !failingChecks('downloader').length) out.push(gap('downloaders-trouble', dls.failing === 1 ? 'Downloader' : `${n(dls.failing)} Downloaders`, dls.failing === 1 ? 'has a problem' : 'have a problem', '/settings/downloaders', 'Fix'));
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
						metadata: metadataOutage.s.down ? METADATA_DOWN : undefined,
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
	const CHECK_WORDS: Record<string, string> = { library_folder: 'Library folders', disk_space: 'Disk space', source: 'Sources', downloader: 'Downloaders', backups: 'Backups', update: 'Updates', config_disk: 'Config folder', config_folder: 'Config folder', waiting_downloads: 'Downloads', trailers: 'Trailers' };
	const passedCount = $derived((ctx.checks?.checks ?? []).filter((c) => shownState(c) === 'ok').length);
	const passed = $derived.by(() => {
		const bad = new Set((ctx.checks?.checks ?? []).filter((c) => shownState(c) !== 'ok').map((c) => CHECK_WORDS[c.kind]));
		return [...new Set((ctx.checks?.checks ?? []).map((c) => CHECK_WORDS[c.kind]).filter((w) => w && !bad.has(w)))];
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
