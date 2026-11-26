// Language packs: every language but English is a signed pack the server installs. A language someone picked installs
// by itself; a browser's own language installs by itself on an install that had every language built in before, and on
// a new install the browser asks once ("Use HolyArrs in Deutsch?").
import { api } from '#lib/core/api.ts';
import { lang, langPack, reloadLang, type Locale } from './i18n.svelte.ts';

export type LanguageState = 'built_in' | 'installed' | 'installing' | 'failed' | 'available';
export interface LanguageRow {
	code: Locale;
	state: LanguageState;
	version?: string;
	size?: number;
	error?: string;
}
export interface LanguageList {
	languages: LanguageRow[];
	auto_browser: boolean;
}

/** offer: the language to ask about; failed: the language that couldn't be downloaded (shown until dismissed). */
export const packs = $state<{ offer: Locale | null; failed: Locale | null; busy: Locale | null }>({ offer: null, failed: null, busy: null });

const ASKED = 'holyarrs-lang-asked';
// Languages fetchMissing already tried this page load: a failed download waits for Try again or the next load.
const tried = new Set<Locale>();

/** English and the installed packs, once the list was read (null until then): what the language picker lists. */
export const packList = $state<{ installed: ReadonlySet<string> | null }>({ installed: null });

export function installedCodes(rows: LanguageRow[]): Set<string> {
	return new Set(rows.filter((r) => r.state === 'built_in' || r.state === 'installed').map((r) => r.code));
}

/** Before sign-in: reads which languages this server can show, so the sign-in pickers list only those. */
export async function listInstalledPublic(): Promise<void> {
	try {
		const got = await api.get<{ installed: string[] }>('/public/languages');
		packList.installed = new Set(got.installed);
	} catch {
		// Unknown: the pickers keep listing every language.
	}
}

export async function listLanguages(): Promise<LanguageList> {
	const list = await api.get<LanguageList>('/languages');
	packList.installed = installedCodes(list.languages);
	return list;
}

/** Installs a language's pack and shows it when it is the one asked for. False when it couldn't be downloaded. */
export async function installLanguage(code: Locale): Promise<boolean> {
	packs.busy = code;
	try {
		await api.post(`/languages/${code}`);
		if (packs.failed === code) packs.failed = null;
		if (langPack.missing === code) await reloadLang();
		return true;
	} catch {
		packs.failed = code;
		return false;
	} finally {
		packs.busy = null;
	}
}

export async function removeLanguage(code: Locale): Promise<void> {
	await api.del(`/languages/${code}`);
}

/** Once signed in: gets the pack of the language being shown in English for want of one, or asks about it. */
export async function fetchMissing(): Promise<void> {
	const code = langPack.missing;
	if (!code || tried.has(code)) return;
	tried.add(code);
	if (lang.choice !== 'auto') {
		await installLanguage(code);
		return;
	}
	let auto = false;
	try {
		auto = (await listLanguages()).auto_browser;
	} catch {
		return;
	}
	if (auto) await installLanguage(code);
	else if (!asked()) packs.offer = code;
}

/** The answer to "Use HolyArrs in …?": install it, or keep English. Asked once per browser either way. */
export async function answerOffer(install: boolean): Promise<void> {
	const code = packs.offer;
	packs.offer = null;
	try {
		localStorage.setItem(ASKED, '1');
	} catch {
		// Without storage the question may come back next visit.
	}
	if (install && code) await installLanguage(code);
}

function asked(): boolean {
	try {
		return localStorage.getItem(ASKED) === '1';
	} catch {
		return false;
	}
}
