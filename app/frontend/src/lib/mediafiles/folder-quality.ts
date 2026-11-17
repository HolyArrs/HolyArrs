// The quality choice on a Library folder: which sets fit its kind, what it shows and what a change sends.
import type { LibraryFolder, PickRulesView } from '../core/types.ts';
import type { TitleDefaults } from './types.ts';
import { t } from '../core/i18n/i18n.svelte.ts';

const fits = (r: PickRulesView, folderKind: string) =>
	folderKind === 'music' ? r.kinds.includes('music') || r.kinds.includes('artist') : r.kinds.includes(folderKind || 'movie');

// Sets that fit the folder's kind. A set the folder already has stays listed so the choice never lies.
export function qualityFor(rules: PickRulesView[], folderKind: string, current?: number | null): PickRulesView[] {
	return rules.filter((r) => fits(r, folderKind) || (!!current && r.id === current));
}

// The server reads 0 as "no quality of its own".
export const qualityBody = (value: string | number | null): { default_pick_rules_id: number } => ({ default_pick_rules_id: Number(value) || 0 });

// The sets the server falls back to when nothing else is chosen; the same names the server looks up.
const builtInName: Record<string, string> = { movie: 'Standard', show: 'Standard', music: 'Lossless first' };

// The set a folder with no quality of its own hands its new titles: the saved Movies and TV default, else the built-in one.
export function defaultSet(rules: PickRulesView[], folderKind: string, saved?: TitleDefaults | null): PickRulesView | undefined {
	const k = folderKind || 'movie';
	const id = k === 'movie' || k === 'show' ? saved?.[k]?.pick_rules_id : 0;
	return (id ? rules.find((r) => r.id === id) : undefined) ?? rules.find((r) => r.name === builtInName[k]);
}

// What the folder's Quality row shows: its own set's name, or the default's name marked as the default.
export function qualityValue(f: Pick<LibraryFolder, 'kind' | 'default_pick_rules_id'>, rules: PickRulesView[], saved?: TitleDefaults | null): { text: string; own: boolean } {
	const own = f.default_pick_rules_id ? rules.find((r) => r.id === f.default_pick_rules_id) : undefined;
	if (own) return { text: own.name, own: true };
	const d = defaultSet(rules, f.kind, saved);
	return { text: d ? t('settings.libraryFolders.qualityDefaultValue', { name: d.name }) : t('settings.libraryFolders.qualityDefault'), own: false };
}

// The first choice in the menu and the add form: the default, named when it is known.
export function defaultChoice(rules: PickRulesView[], folderKind: string, saved?: TitleDefaults | null): string {
	const d = defaultSet(rules, folderKind, saved);
	return d ? t('settings.libraryFolders.qualityDefaultNamed', { name: d.name }) : t('settings.libraryFolders.qualityDefault');
}

// The menu: the default first, then the sets that fit; id 0 is the default. The current one is ticked.
export function qualityChoices(rules: PickRulesView[], folderKind: string, current: number | null | undefined, saved?: TitleDefaults | null): { id: number; label: string; checked: boolean }[] {
	const now = current ?? 0;
	return [{ id: 0, label: defaultChoice(rules, folderKind, saved), checked: !now }, ...qualityFor(rules, folderKind, current).map((r) => ({ id: r.id, label: r.name, checked: r.id === now }))];
}
