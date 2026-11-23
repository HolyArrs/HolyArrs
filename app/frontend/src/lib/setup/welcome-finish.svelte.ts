// The later first-run steps: Folders' Hands-off button, the Found lines on Sources and Downloads, Subtitles' languages, Access's address and Done's quality line.
import { api, message, ApiError } from '#lib/core/api.ts';
import { to } from '#lib/core/paths.ts';
import { kindWords } from '#lib/downloads/index.ts';
import { wishProblem, type Language, type SubtitleSettings } from '#lib/subtitles/index.ts';
import { qualityValue } from '#lib/mediafiles/index.ts';
import type { TitleDefaults } from '#lib/mediafiles/index.ts';
import type { PickRulesView } from '#lib/core/types.ts';
import { nextStep } from './welcome-steps.ts';

export function createWelcomeFinish(ctx: any) {
	// Folders: "Don't let HolyArrs touch my files" turns Hands-off on; a build without that setting yet (404 or 405) still moves on.
	async function handsOffOn() {
		if (ctx.busy) return;
		ctx.error = '';
		ctx.busy = true;
		try {
			await api.put('/settings/hands-off', { on: true });
		} catch (err) {
			if (!(err instanceof ApiError && (err.status === 404 || err.status === 405))) {
				ctx.busy = false;
				return void (ctx.error = message(err));
			}
		}
		ctx.handsOff = true;
		ctx.busy = false;
		ctx.go(nextStep('folders', ctx.plan));
	}

	// Sources and Downloads: what an import brought shows as one "Found" line, with Change to open the full step.
	let bridgeNames = $state<string[]>([]);
	let bridgeKinds = $state<string[]>([]);
	async function loadBridges() {
		await ctx.sourcesStep.loadSources();
		const names: Record<string, string> = { prowlarr: 'Prowlarr', jackett: 'Jackett' };
		bridgeKinds = await api.get<{ bridges: { kind: string }[] }>('/bridges').then((r) => (r.bridges ?? []).map((b) => b.kind), () => []);
		bridgeNames = bridgeKinds.map((k) => names[k] ?? k);
	}
	let srcChange = $state(false);
	let dlChange = $state(false);
	const foundSrc = $derived(ctx.bring.didImport && !srcChange ? bridgeNames : []);
	// Plain sources an import brought with no bridge: a count line instead of an empty form.
	const foundCount = $derived(ctx.bring.didImport && !srcChange && !bridgeNames.length ? ctx.sourcesStep.sources : 0);
	// Only downloaders that answered fold into the Found line; one that failed or needs its key keeps the full form.
	const foundDl = $derived(ctx.bring.didImport && !dlChange && !ctx.downloaderStep.dlForm && (ctx.downloaderStep.dlList as { id: number }[]).every((d) => ctx.downloaderStep.dlTests[d.id]?.ok) ? [...new Set((ctx.downloaderStep.dlList as { kind: string; name: string }[]).map((d) => (kindWords as Record<string, string>)[d.kind] ?? d.name))] : []);

	// Subtitles: the languages to look for (Settings › Subtitles has the rest).
	let subs = $state<SubtitleSettings | null>(null);
	let subLangs = $state<Language[]>([]);
	let subsFailed = $state(false);
	async function loadSubs() {
		subsFailed = false;
		try {
			const [s, l] = await Promise.all([api.get<SubtitleSettings>('/settings/subtitles'), api.get<{ languages: Language[] }>('/subtitles/languages')]);
			subLangs = l.languages ?? [];
			subs = s;
		} catch (err) {
			subsFailed = true;
			ctx.error = message(err);
		}
	}
	async function saveSubs(e: SubmitEvent) {
		e.preventDefault();
		if (!subs || ctx.busy) return;
		const problem = wishProblem(subs.languages);
		if (problem) return void (ctx.error = problem);
		ctx.error = '';
		ctx.busy = true;
		try {
			subs = await api.put<SubtitleSettings>('/settings/subtitles', subs);
			ctx.busy = false;
			ctx.go(nextStep('subtitles', ctx.plan));
		} catch (err) {
			ctx.busy = false;
			ctx.error = message(err);
		}
	}

	// Access: the address people use to reach this app; invite links carry it.
	const appAddress = typeof location === 'undefined' ? '' : location.origin + to('/');

	// Done: the download quality new titles get, with Change.
	let quality = $state('');
	async function loadQuality() {
		try {
			const [saved, rules] = await Promise.all([api.get<TitleDefaults>('/settings/title-defaults'), api.get<{ pick_rules: PickRulesView[] }>('/pick-rules')]);
			quality = qualityValue({ kind: 'movie', default_pick_rules_id: null }, rules.pick_rules ?? [], saved).text;
		} catch {
			quality = '';
		}
	}

	return {
		get srcChange() { return srcChange; },
		set srcChange(v) { srcChange = v; },
		get dlChange() { return dlChange; },
		set dlChange(v) { dlChange = v; },
		get foundSrc() { return foundSrc; },
		get foundDl() { return foundDl; },
		get foundCount() { return foundCount; },
		get subsFailed() { return subsFailed; },
		get subs() { return subs; },
		get subLangs() { return subLangs; },
		get quality() { return quality; },
		appAddress,
		handsOffOn,
		loadBridges,
		get bridges() { return bridgeKinds; },
		loadSubs,
		saveSubs,
		loadQuality,
	};
}
