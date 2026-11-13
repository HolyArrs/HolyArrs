// The quality choice on a Library folder: which sets fit its kind, and what a change sends.
import type { PickRulesView } from '../core/types.ts';

const fits = (r: PickRulesView, folderKind: string) =>
	folderKind === 'music' ? r.kinds.includes('music') || r.kinds.includes('artist') : r.kinds.includes(folderKind || 'movie');

// Sets that fit the folder's kind. A set the folder already has stays listed so the choice never lies.
export function qualityFor(rules: PickRulesView[], folderKind: string, current?: number | null): PickRulesView[] {
	return rules.filter((r) => fits(r, folderKind) || (!!current && r.id === current));
}

// The server reads 0 as "no quality of its own".
export const qualityBody = (value: string | number | null): { default_pick_rules_id: number } => ({ default_pick_rules_id: Number(value) || 0 });
