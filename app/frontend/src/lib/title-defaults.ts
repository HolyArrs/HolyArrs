// Movies and TV defaults: what a new movie or show gets, and where the add
// dialogs start. Kept apart from the pages so it can be tested.
import type { LibraryFolder, PickRulesView } from './core/types.ts';
import type { TitleDefaults } from './mediafiles/index.ts';

export type TitleKind = 'movie' | 'show';

/** builtInDefaults are what the server answers before an owner saves anything. */
export const builtInDefaults: Readonly<TitleDefaults> = Object.freeze({
	movie: Object.freeze({ library_folder_id: 0, pick_rules_id: 0, keep_up_to_date: true, search_on_add: true }),
	show: Object.freeze({ library_folder_id: 0, pick_rules_id: 0, keep_up_to_date: true, search_on_add: false })
});

/** gateChoices are when a movie counts as out, with a line about each. */
export const gateChoices: [string, string, string][] = [
	['announced', 'As soon as it is announced', 'Look even before it is in cinemas.'],
	['in_cinemas', 'Once it is in cinemas', 'Look from the cinema date.'],
	['released', 'Once it is out on digital or disc', 'Look from the first digital or disc date.']
];

/** foldersFor keeps the Library folders of one kind. */
export function foldersFor(folders: LibraryFolder[], kind: TitleKind): LibraryFolder[] {
	return folders.filter((f) => f.kind === kind);
}

/** rulesFor keeps the Pick rules a kind may use; rules without kinds fit every kind. */
export function rulesFor(rules: PickRulesView[], kind: TitleKind): PickRulesView[] {
	return rules.filter((r) => !r.kinds.length || r.kinds.includes(kind));
}

export interface StartChoices {
	folderId: number;
	rulesId: number;
	keep: boolean;
	searchOnAdd: boolean;
}

const offered = (id: number | null | undefined, list: { id: number }[]) => !!id && list.some((x) => x.id === id);

/**
 * startChoices is where an add dialog starts: the saved folder (or the first
 * healthy one of that kind), the saved Pick rules (or that folder's own, or
 * the first for the kind), and the saved switches. A saved choice that is gone
 * or of another kind falls back the same way.
 */
export function startChoices(defaults: TitleDefaults | null | undefined, folders: LibraryFolder[], rules: PickRulesView[], kind: TitleKind): StartChoices {
	const d = defaults?.[kind] ?? builtInDefaults[kind];
	const fs = foldersFor(folders, kind);
	const rs = rulesFor(rules, kind);
	const folderId = offered(d.library_folder_id, fs) ? d.library_folder_id : (fs.find((f) => f.health === 'ok')?.id ?? fs[0]?.id ?? 0);
	const own = fs.find((f) => f.id === folderId)?.default_pick_rules_id;
	const rulesId = offered(d.pick_rules_id, rs) ? d.pick_rules_id : offered(own, rs) ? (own as number) : (rs[0]?.id ?? 0);
	return { folderId, rulesId, keep: d.keep_up_to_date, searchOnAdd: d.search_on_add };
}

export interface DefaultsLoader {
	/** load answers the saved defaults, fetched once per visit; null (the built-in ones) when the server fails. */
	load(): Promise<TitleDefaults | null>;
	/** forget drops the kept answer, so the next load asks again (a new visit, or after a save). */
	forget(): void;
}

/** defaultsLoader keeps one answer of /settings/title-defaults for the add dialogs. A failure is not kept. */
export function defaultsLoader(get: (path: string) => Promise<TitleDefaults>): DefaultsLoader {
	let kept: Promise<TitleDefaults | null> | null = null;
	return {
		load() {
			if (!kept) {
				const p = get('/settings/title-defaults').catch(() => {
					if (kept === p) kept = null;
					return null;
				});
				kept = p;
			}
			return kept;
		},
		forget() {
			kept = null;
		}
	};
}

/** changedParts names the parts of a page's settings that differ from what was loaded, in order. */
export function changedParts<T extends object>(loaded: T, now: T): (keyof T & string)[] {
	const was = loaded as Record<string, unknown>;
	const is = now as Record<string, unknown>;
	return Object.keys(is).filter((k) => JSON.stringify(was[k]) !== JSON.stringify(is[k])) as (keyof T & string)[];
}
