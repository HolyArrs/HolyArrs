// The folder and quality offered for each request, and the people who asked.
import { type Member } from './board.ts';
import type { Ask } from '#lib/core/types.ts';
import { ownerRow } from './quality-row.ts';
import type { QualityChoice } from './types.ts';

import type { LibraryFolder, PickRulesView } from '#lib/core/types.ts';

export function createAskChoices(ctx: { rules: PickRulesView[]; folders: LibraryFolder[] }) {
	let members = $state<Record<number, Member>>({});
	let membersAsked = false;
	// The approver's lines of what people can ask for, per kind (from /asks/limit).
	let lines = $state<{ movie: QualityChoice[]; show: QualityChoice[] }>({ movie: [], show: [] });
	const foldersOf = (a: Ask) => {
		const k = a.kind === 'show' ? 'show' : a.kind === 'artist' ? 'music' : 'movie';
		return ctx.folders.filter((f) => f.kind === k);
	};
	const rulesOf = (a: Ask) => {
		const k = a.kind === 'show' ? 'show' : a.kind === 'artist' ? 'music' : 'movie';
		return ctx.rules.filter((r) => !r.kinds.length || r.kinds.includes(k));
	};
	// Music has no Quality row; movies and shows show the lines with the other Pick rules between them.
	const qualityRowOf = (a: Ask) => (a.kind === 'artist' ? { pills: [], asked: '' } : ownerRow(a.kind === 'show' ? lines.show : lines.movie, rulesOf(a), a));

	return {
		get lines() { return lines; },
		set lines(v) { lines = v; },
		get membersAsked() { return membersAsked; },
		set membersAsked(v) { membersAsked = v; },
		get members() { return members; },
		set members(v) { members = v; },
		foldersOf,
		rulesOf,
		qualityRowOf,
	};
}
