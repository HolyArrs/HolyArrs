// The move wizard's plan: the folder picks, the rules, the people and the trial run.
import { OTHER, setChoice, type PathRule, type PersonChoice, type Plan, type Report } from './move.ts';
import { api, ApiError } from '#lib/core/api.ts';
import { plainMessage as message } from '#lib/core/settings.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

/** A plan or test run worked out on the server in the background. */
export interface CheckView {
	check_id?: string;
	state?: 'running' | 'done' | 'failed';
	step?: string;
	done?: number;
	total?: number;
	now?: string;
	counts?: Record<string, number>;
}

/** Short progress text for a running check: what it does, how far, problems so far. */
export function checkNote(v: CheckView | null): string {
	if (!v || v.state !== 'running') return '';
	if (v.step === 'titles' && (v.total ?? 0) > 0) {
		let s = t('migrate.checkedOf', { done: v.done ?? 0, total: v.total ?? 0 });
		if (v.now) s += ` · ${v.now}`;
		const issues = (v.counts?.not_found ?? 0) + (v.counts?.failed ?? 0);
		if (issues > 0) s += ` · ${t('migrate.checkIssuesSoFar', { n: issues })}`;
		return s;
	}
	return v.step === 'titles' ? t('migrate.checkTitles') : v.step === 'folders' ? t('migrate.checkFolders') : t('migrate.checkReading');
}

const pause = (ms: number, signal?: AbortSignal) =>
	new Promise<void>((done, fail) => {
		const id = setTimeout(done, ms);
		signal?.addEventListener('abort', () => {
			clearTimeout(id);
			fail(new DOMException('aborted', 'AbortError'));
		}, { once: true });
	});

/**
 * Starts a check and follows it until it is done: the POST answers at once and
 * GET /migrate/checks/{id} tells how far it is. A restart forgets checks (410):
 * it is then started again.
 */
export async function runCheck<T>(path: string, body: unknown, signal: AbortSignal, onprogress: (v: CheckView) => void): Promise<T> {
	for (let attempt = 0; ; attempt++) {
		let v = await api.request<CheckView & T>(path, { method: 'POST', body, signal });
		try {
			while (v.state === 'running') {
				onprogress(v);
				await pause(800, signal);
				v = await api.request<CheckView & T>(`/migrate/checks/${encodeURIComponent(v.check_id ?? '')}`, { signal });
			}
			return v;
		} catch (err) {
			if (attempt < 3 && err instanceof ApiError && err.status === 410) continue;
			throw err;
		}
	}
}

export function createMovePlan(ctx: { extra: PathRule[]; skip: string[]; [key: string]: any }) {
	function replan(wait: boolean) {
		clearTimeout(ctx.planTimer);
		if (wait) ctx.planTimer = setTimeout(() => void makePlan(), 400);
		else void makePlan();
	}
	async function makePlan() {
		clearTimeout(ctx.planTimer);
		if (ctx.picked.length === 0 || ctx.ruleErrors > 0) return;
		const seq = ++ctx.planSeq;
		ctx.planCtrl?.abort();
		ctx.planCtrl = new AbortController();
		ctx.planning = true;
		ctx.planError = '';
		try {
			const r = await runCheck<{ plan: Plan; suggested_path_rules?: PathRule[]; missing_paths?: Record<string, string>; seen_folders?: string[]; here_folders?: string[]; plan_hash?: string }>('/migrate/plan', { ...ctx.choices(), quick: !!ctx.quick }, ctx.planCtrl.signal, (v) => {
				if (seq === ctx.planSeq) ctx.checkNote = checkNote(v);
			});
			if (seq !== ctx.planSeq || ctx.destroyed) return;
			ctx.plan = r.plan;
			// Setup starts the import from this very plan (no test run first): the server checks the same hash.
			if (ctx.quick) ctx.planHash = r.plan_hash ?? '';
			ctx.missingPaths = r.missing_paths ?? {};
			ctx.seenFolders = r.seen_folders ?? [];
			ctx.hereFolders = r.here_folders ?? [];
			const sug: Record<string, string> = {};
			for (const s of r.suggested_path_rules ?? []) sug[s.from] = s.to;
			ctx.suggested = sug;
			// A suggestion fills a folder nobody set yet; the plan then runs with it.
			const next = { ...ctx.picks };
			let added = false;
			for (const [from, dest] of Object.entries(sug)) {
				if (!(from in next) && !ctx.extra.some((r) => r.from.trim() === from)) {
					next[from] = dest;
					added = true;
				}
			}
			if (added) {
				ctx.picks = next;
				ctx.changed();
				void makePlan();
			} else if (ctx.step === 'choose' && !ctx.quick && (!ctx.dry || ctx.dryAt !== ctx.changes)) {
				// Setup's Folders step skips this: Continue runs the test itself.
				// The test run marks what needs a look (a show to check) before Import is pressed.
				void runTry();
			}
		} catch (err) {
			if (seq !== ctx.planSeq || ctx.quiet(err)) return;
			if (err instanceof ApiError && err.status === 404) ctx.gone = true;
			else ctx.planError = message(err);
		} finally {
			if (seq === ctx.planSeq) {
				ctx.planning = false;
				ctx.checkNote = '';
			}
		}
	}
	function onfolder(from: string, value: string) {
		ctx.picks = { ...ctx.picks, [from]: value };
		ctx.changed();
		if (value !== OTHER) replan(true);
	}
	function onother(from: string, value: string) {
		ctx.others = { ...ctx.others, [from]: value };
		ctx.changed();
	}
	function onextra(rules: PathRule[]) {
		ctx.extra = rules;
		ctx.changed();
	}
	function ongroup(name: string, on: boolean) {
		ctx.skip = on ? ctx.skip.filter((x) => x !== name) : [...ctx.skip, name];
		ctx.changed();
		replan(true);
	}
	function onperson(key: string, c: PersonChoice | null) {
		ctx.people = setChoice(ctx.people, key, c);
		ctx.changed();
		replan(true);
	}

	// The test run. A choice runs it again in place.
	async function runTry() {
		if (ctx.picked.length === 0 || ctx.ruleErrors > 0) return;
		// No plan yet (a reload, a link): plan first, so the suggested folder matches are filled in before the test runs.
		if (!ctx.plan) {
			await makePlan();
			if (ctx.destroyed) return;
		}
		const seq = ++ctx.trySeq;
		ctx.tryCtrl?.abort();
		ctx.tryCtrl = new AbortController();
		const at = ctx.changes;
		ctx.trying = true;
		ctx.tryFailed = false;
		ctx.actionError = '';
		try {
			const r = await runCheck<{ report: Report; plan_hash: string }>('/migrate/dry-run', ctx.choices(), ctx.tryCtrl.signal, (v) => {
				if (seq === ctx.trySeq) ctx.checkNote = checkNote(v);
			});
			if (seq !== ctx.trySeq || ctx.destroyed) return;
			ctx.dry = r.report;
			ctx.planHash = r.plan_hash;
			ctx.dryAt = at;
			const m = new Map(ctx.seen);
			for (const l of r.report.lines) if (l.choose) m.set(l.key, l);
			ctx.seen = m;
			// Shows not looked up in time are looked up meanwhile: run again shortly to show them.
			if (r.report.checking) setTimeout(() => void (seq === ctx.trySeq && !ctx.destroyed && runTry()), 5000);
		} catch (err) {
			if (seq !== ctx.trySeq || ctx.quiet(err)) return;
			if (err instanceof ApiError && err.status === 404) ctx.gone = true;
			else {
				ctx.tryFailed = true;
				ctx.actionError = message(err);
			}
		} finally {
			if (seq === ctx.trySeq) {
				ctx.trying = false;
				ctx.checkNote = '';
			}
		}
	}
	function onchoose(key: string, c: PersonChoice) {
		ctx.people = setChoice(ctx.people, key, c);
		ctx.changed();
		void runTry();
	}

	/** Choose's Move: a quiet test run, then the move (after a confirm), or "Before you move" when the test found problems. */

	return {
		makePlan,
		runTry,
		onfolder,
		onother,
		onextra,
		ongroup,
		onperson,
		replan,
		onchoose,
	};
}
