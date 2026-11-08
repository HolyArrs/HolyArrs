// The folder and quality rules offered for each request, and the people who asked.
import { type Member } from './board.ts';
import type { Ask } from '#lib/core/types.ts';

import type { LibraryFolder, PickRulesView } from '#lib/core/types.ts';

export function createAskChoices(ctx: { rules: PickRulesView[]; folders: LibraryFolder[] }) {
	let members = $state<Record<number, Member>>({});
	let membersAsked = false;
	let askSettings = $state<Record<string, unknown>>({});
	const defaultRuleOf = (a: Ask) => {
		const k = a.kind === 'show' ? 'show_pick_rules_id' : a.kind === 'artist' ? 'music_pick_rules_id' : 'pick_rules_id';
		const id = Number(askSettings[k] ?? 0);
		return id ? (ctx.rules.find((r) => r.id === id)?.name ?? '') : '';
	};
	const foldersOf = (a: Ask) => {
		const k = a.kind === 'show' ? 'show' : a.kind === 'artist' ? 'music' : 'movie';
		return ctx.folders.filter((f) => f.kind === k);
	};
	const rulesOf = (a: Ask) => {
		const k = a.kind === 'show' ? 'show' : a.kind === 'artist' ? 'music' : 'movie';
		return ctx.rules.filter((r) => !r.kinds.length || r.kinds.includes(k));
	};

	return {
		get askSettings() { return askSettings; },
		set askSettings(v) { askSettings = v; },
		get membersAsked() { return membersAsked; },
		set membersAsked(v) { membersAsked = v; },
		get members() { return members; },
		set members(v) { members = v; },
		foldersOf,
		rulesOf,
		defaultRuleOf,
	};
}
