// A page that breaks in the browser says why, on the error page and in the
// console, so a screenshot is enough to find the cause ("Internal Error" alone
// says nothing). Not-found pages keep their plain words.
export const handleError = ({ error, status }: { error: unknown; status: number }) => {
	console.error(error);
	const e = error as { status?: number; message?: unknown } | null;
	if (status === 404 || e?.status === 404) return { message: 'Not Found' };
	const why = error instanceof Error ? error.message : typeof e?.message === 'string' ? e.message : String(error);
	return { message: `Internal Error: ${why}`.slice(0, 300) };
};
