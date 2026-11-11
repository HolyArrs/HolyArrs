// The move wizard's plan: the folder picks, the rules, the people and the trial run.
import { OTHER, setChoice, type PathRule, type PersonChoice, type Plan, type Report } from './move.ts';
import { api, ApiError } from '#lib/core/api.ts';
import { plainMessage as message } from '#lib/core/settings.ts';

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
			const r = await api.request<{ plan: Plan; suggested_path_rules?: PathRule[]; missing_paths?: Record<string, string>; seen_folders?: string[] }>('/migrate/plan', { method: 'POST', body: ctx.choices(), signal: ctx.planCtrl.signal });
			if (seq !== ctx.planSeq || ctx.destroyed) return;
			ctx.plan = r.plan;
			ctx.missingPaths = r.missing_paths ?? {};
			ctx.seenFolders = r.seen_folders ?? [];
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
			}
		} catch (err) {
			if (seq !== ctx.planSeq || ctx.quiet(err)) return;
			if (err instanceof ApiError && err.status === 404) ctx.gone = true;
			else ctx.planError = message(err);
		} finally {
			if (seq === ctx.planSeq) ctx.planning = false;
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
			const r = await api.request<{ report: Report; plan_hash: string }>('/migrate/dry-run', { method: 'POST', body: ctx.choices(), signal: ctx.tryCtrl.signal });
			if (seq !== ctx.trySeq || ctx.destroyed) return;
			ctx.dry = r.report;
			ctx.planHash = r.plan_hash;
			ctx.dryAt = at;
			const m = new Map(ctx.seen);
			for (const l of r.report.lines) if (l.choose) m.set(l.key, l);
			ctx.seen = m;
		} catch (err) {
			if (seq !== ctx.trySeq || ctx.quiet(err)) return;
			if (err instanceof ApiError && err.status === 404) ctx.gone = true;
			else {
				ctx.tryFailed = true;
				ctx.actionError = message(err);
			}
		} finally {
			if (seq === ctx.trySeq) ctx.trying = false;
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
