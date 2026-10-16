export type PersonRole = 'all' | 'actor' | 'director';

export type PersonSummary = {
	id: string;
	name: string;
	department?: string;
	profile?: string;
	known_for: PersonMovie[];
};

export type Person = PersonSummary & {
	biography?: string;
	birthday?: string;
	deathday?: string;
	birthplace?: string;
};

export type PersonMovie = {
	name: string;
	year: number;
	overview: string;
	poster?: string;
	ids: Record<string, string>;
	roles?: string[];
	characters?: string[];
	title_id?: number;
	never_add?: boolean;
	ask?: { id: number; state: string; state_label: string; mine: boolean; people: number };
};

export type PersonSearchPage = {
	people: PersonSummary[];
	page: number;
	total_pages: number;
	total_results: number;
	attribution: string;
};

export type FilmographyPage = {
	person: Person;
	movies: PersonMovie[];
	page: number;
	page_size: number;
	total_pages: number;
	total_results: number;
	complete: boolean;
	provider_pages: number;
	attribution: string;
};

export type ReviewedResult = { tmdb_id: string; ok: boolean; error?: string };

export function normalizeSelection(ids: string[]): string[] {
	const seen = new Set<string>();
	const out: string[] = [];
	for (const raw of ids) {
		const id = raw.trim();
		if (!/^\d{1,12}$/.test(id) || seen.has(id)) continue;
		seen.add(id);
		out.push(id);
	}
	return out;
}

export function actionableSelection(movies: PersonMovie[]): string[] {
	return normalizeSelection(movies.filter((movie) => !movie.title_id && !movie.never_add && !movie.ask).map((movie) => movie.ids.tmdb ?? ''));
}

function errorText(err: unknown): string {
	if (err instanceof Error && err.message.trim()) return err.message;
	return 'This movie could not be submitted.';
}

// runReviewed intentionally uses the existing one-movie routes. Each item
// keeps its own result, so one refusal or weekly-limit answer does not hide
// what happened to the other reviewed movies.
export async function runReviewed(
	post: (path: string, body: Record<string, unknown>) => Promise<unknown>,
	action: 'add' | 'ask',
	ids: string[]
): Promise<ReviewedResult[]> {
	const results: ReviewedResult[] = [];
	for (const tmdb_id of normalizeSelection(ids)) {
		try {
			await post(action === 'add' ? '/titles' : '/asks', { id: `tmdb:${tmdb_id}` });
			results.push({ tmdb_id, ok: true });
		} catch (err) {
			results.push({ tmdb_id, ok: false, error: errorText(err) });
		}
	}
	return results;
}
