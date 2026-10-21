// The viewer's country, read from the browser languages.

/** defaultRegion is the country of the first browser language that names one, else global. */
export function defaultRegion(langs: readonly string[] = typeof navigator === 'undefined' ? [] : navigator.languages): string {
	for (const l of langs) {
		const m = /^[a-z]{2,3}-([A-Za-z]{2})$/.exec(l);
		if (m) return m[1].toUpperCase();
	}
	return 'global';
}
