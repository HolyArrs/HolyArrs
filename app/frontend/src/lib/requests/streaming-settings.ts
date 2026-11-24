// My streaming services: the countries to pick from and the service list helpers.

import { appLocale } from '#lib/core/i18n/i18n.svelte.ts';

/** Countries streaming services are listed for, as two-letter codes. */
export const STREAMING_COUNTRIES = [
	'AR', 'AT', 'AU', 'BE', 'BG', 'BR', 'CA', 'CH', 'CL', 'CO', 'CZ', 'DE', 'DK', 'EE', 'EG', 'ES', 'FI', 'FR', 'GB', 'GR', 'HK', 'HR', 'HU', 'ID', 'IE', 'IL',
	'IN', 'IS', 'IT', 'JP', 'KR', 'LT', 'LV', 'MX', 'MY', 'NL', 'NO', 'NZ', 'PE', 'PH', 'PL', 'PT', 'RO', 'RS', 'RU', 'SA', 'SE', 'SG', 'SK', 'TH', 'TR',
	'TW', 'UA', 'US', 'VN', 'ZA'
] as const;

/** Most services a person can pick (the server's limit). */
export const MAX_MY_SERVICES = 30;

/** Adds or removes one service, keeping the picked order; never past the limit. */
export function toggleService(picked: readonly number[], id: number, on: boolean): number[] {
	const rest = picked.filter((p) => p !== id);
	if (!on) return rest;
	return rest.length >= MAX_MY_SERVICES ? [...picked] : [...rest, id];
}

/** A country's name in the app language, its code when the browser has no name for it. */
export function countryName(code: string, locale: string = appLocale() ?? 'en'): string {
	try {
		return new Intl.DisplayNames([locale], { type: 'region' }).of(code) ?? code;
	} catch {
		return code;
	}
}

/** The countries sorted by their shown name, with the saved one kept even when not listed. */
export function countryOptions(saved: string, locale: string = appLocale() ?? 'en'): { code: string; name: string }[] {
	const codes: string[] = [...STREAMING_COUNTRIES];
	if (/^[A-Z]{2}$/.test(saved) && !codes.includes(saved)) codes.push(saved);
	return codes.map((code) => ({ code, name: countryName(code, locale) })).sort((a, b) => a.name.localeCompare(b.name, locale));
}
