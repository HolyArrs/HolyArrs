// Region code guessed from browser languages.
export function defaultRegion(langs: readonly string[] = typeof navigator === 'undefined' ? [] : navigator.languages): string {
	for (const l of langs) {
		const m = /^[a-z]{2,3}-([A-Za-z]{2})$/.exec(l);
		if (m) return m[1].toUpperCase();
	}
	return 'global';
}

// Service lookups need a two letter code, so 'global' falls back to US.
export function serviceRegion(r: string): string {
	return /^[A-Z]{2}$/.test(r) ? r : 'US';
}
