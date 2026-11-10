// Starting choices for a new title: built-in defaults, saved defaults and what is on offer.
import type { LibraryFolder, PickRulesView } from '../core/types.ts';
import type { TitleDefaults } from './types.ts';
import { t } from '../core/i18n/i18n.svelte.ts';

export type TitleKind = 'movie' | 'show';

export interface StartChoices {
	folderId: number;
	rulesId: number;
	keep: boolean;
	searchOnAdd: boolean;
}

export interface DefaultsLoader {
	// Loads once and reuses the answer; a failed load is not kept.
	load(): Promise<TitleDefaults | null>;
	// Drops the kept answer so the next load asks again.
	forget(): void;
}

// Id 0 means nothing picked yet; startChoices fills it from what exists.
export const builtInDefaults: Readonly<TitleDefaults> = Object.freeze({
	movie: Object.freeze({ library_folder_id: 0, pick_rules_id: 0, keep_up_to_date: true, search_on_add: true }),
	show: Object.freeze({ library_folder_id: 0, pick_rules_id: 0, keep_up_to_date: true, search_on_add: false })
});

// Choices are [value, name, help text]. The name and help read their words when used, so they follow the language.
function gate(value: string, name: string, help: string): [string, string, string] {
	const row = [value, '', ''] as [string, string, string];
	Object.defineProperty(row, 1, { get: () => t(name), enumerable: true });
	Object.defineProperty(row, 2, { get: () => t(help), enumerable: true });
	return row;
}

export const gateChoices: [string, string, string][] = [
	gate('announced', 'mediafiles.gate.announced', 'mediafiles.gate.announcedHelp'),
	gate('in_cinemas', 'mediafiles.gate.inCinemas', 'mediafiles.gate.inCinemasHelp'),
	gate('released', 'mediafiles.gate.released', 'mediafiles.gate.releasedHelp')
];

const offered = (id: number | null | undefined, list: { id: number }[]) => !!id && list.some((x) => x.id === id);

export function foldersFor(folders: LibraryFolder[], kind: TitleKind): LibraryFolder[] {
	return folders.filter((f) => f.kind === kind);
}

// Rules with no kinds apply to everything.
export function rulesFor(rules: PickRulesView[], kind: TitleKind): PickRulesView[] {
	return rules.filter((r) => !r.kinds.length || r.kinds.includes(kind));
}

// A saved default only counts while it is still on offer; otherwise use a healthy folder and that folder's own rules.
export function startChoices(defaults: TitleDefaults | null | undefined, folders: LibraryFolder[], rules: PickRulesView[], kind: TitleKind): StartChoices {
	const d = defaults?.[kind] ?? builtInDefaults[kind];
	const fs = foldersFor(folders, kind);
	const rs = rulesFor(rules, kind);
	const folderId = offered(d.library_folder_id, fs) ? d.library_folder_id : (fs.find((f) => f.health === 'ok')?.id ?? fs[0]?.id ?? 0);
	const own = fs.find((f) => f.id === folderId)?.default_pick_rules_id;
	const rulesId = offered(d.pick_rules_id, rs) ? d.pick_rules_id : offered(own, rs) ? (own as number) : (rs[0]?.id ?? 0);
	return { folderId, rulesId, keep: d.keep_up_to_date, searchOnAdd: d.search_on_add };
}

// Keeps the promise itself so many callers share one request.
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

// Names of the fields whose value differs from the loaded copy.
export function changedParts<T extends object>(loaded: T, now: T): (keyof T & string)[] {
	const was = loaded as Record<string, unknown>;
	const is = now as Record<string, unknown>;
	return Object.keys(is).filter((k) => JSON.stringify(was[k]) !== JSON.stringify(is[k])) as (keyof T & string)[];
}
