// The release's own page on its Source, only when it is a plain web link, so
// no script or data link can be opened from a release name.
export function releasePage(link: string | undefined): string | undefined {
	if (!link) return undefined;
	try {
		const u = new URL(link.trim());
		return (u.protocol === 'http:' || u.protocol === 'https:') && u.host ? u.href : undefined;
	} catch {
		return undefined;
	}
}
