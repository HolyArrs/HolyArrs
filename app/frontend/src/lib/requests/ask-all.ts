// Request all: one normal request per movie of a series, sent one after the other, so the weekly limit, approval and age rules apply to each exactly as if the person had asked one by one.
import { ApiError, message } from '../core/api.ts';
import type { Ask, TitleView } from '../core/types.ts';
import { cardTmdb, type ExploreCard } from '../ui/title-cards.ts';

export type AskAllState = 'todo' | 'requested' | 'skipped';
/** Why a movie was skipped: already in the Library, already asked for by this person, the weekly limit, or the server's own plain reason (age rating, ...). */
export type AskAllWhy = 'library' | 'mine' | 'limit' | 'server';

export interface AskAllLine {
	card: ExploreCard;
	state: AskAllState;
	why?: AskAllWhy;
	/** The server's plain reason, for why 'limit' and 'server'. */
	reason?: string;
	ask?: Ask;
	/** The Library title, when the movie was added instead of requested. */
	title?: TitleView;
}

export type AskAllPost = (body: Record<string, unknown>) => Promise<{ ask?: Ask; title?: TitleView; joined?: boolean; already?: boolean }>;

/** What would be sent: movies in the Library, and (when requesting) movies this person has an open request for, are skipped up front. One that was turned down can be asked for again. Adding does not care who asked. */
export function planAskAll(cards: ExploreCard[], adding = false): AskAllLine[] {
	const seen = new Set<string>();
	const lines: AskAllLine[] = [];
	for (const card of cards) {
		const tmdb = cardTmdb(card);
		if (card.kind !== 'movie' || !tmdb || seen.has(tmdb)) continue;
		seen.add(tmdb);
		if (card.titleId !== undefined || card.status === 'in_library') lines.push({ card, state: 'skipped', why: 'library' });
		else if (card.mine && !card.declined && !adding) lines.push({ card, state: 'skipped', why: 'mine' });
		else lines.push({ card, state: 'todo' });
	}
	return lines;
}

/** The series read again past the browser's and the app's short-lived copies, so requests made since the page loaded count. */
export { freshPath } from '../core/paths.ts';

/** How many of the lines would be sent. */
export const toSend = (lines: AskAllLine[]): number => lines.filter((l) => l.state === 'todo').length;

/** A movie someone else has an open request for: asking joins that request, and the weekly limit does not count a join. */
export const joins = (l: AskAllLine): boolean => l.state === 'todo' && !!l.card.ask && !l.card.mine;

/** Sends the todo lines one at a time. One that fails is skipped with the server's reason and the rest go on, except at the weekly limit (429): every later new request would be refused too, so those are skipped with the same reason, while movies that join someone else's request are still sent. With no asks left from the start (`noRoom`) the new requests are skipped without being sent at all. Each finished line is handed to `onLine`. */
export async function runAskAll(lines: AskAllLine[], post: AskAllPost, onLine: (l: AskAllLine, i: number) => void = () => {}, noRoom = false): Promise<AskAllLine[]> {
	const out = lines.map((l) => ({ ...l }));
	let stopped: string | null = noRoom ? '' : null;
	for (let i = 0; i < out.length; i++) {
		const line = out[i];
		if (line.state !== 'todo') continue;
		if (stopped !== null && !joins(line)) {
			out[i] = { ...line, state: 'skipped', why: 'limit', reason: stopped || undefined };
			onLine(out[i], i);
			continue;
		}
		try {
			const r = await post({ id: `tmdb:${cardTmdb(line.card)}`, follow: true });
			out[i] = r.already ? { ...line, state: 'skipped', why: 'mine', ask: r.ask } : { ...line, state: 'requested', ask: r.ask, title: r.title };
		} catch (err) {
			if (err instanceof ApiError && err.status === 409 && typeof err.body.title_id === 'number') out[i] = { ...line, state: 'skipped', why: 'library', card: { ...line.card, titleId: err.body.title_id } };
			else {
				const reason = message(err);
				const limit = err instanceof ApiError && err.status === 429;
				if (limit) stopped = reason;
				out[i] = { ...line, state: 'skipped', why: limit ? 'limit' : 'server', reason };
			}
		}
		onLine(out[i], i);
	}
	return out;
}

/** The server's own words about the weekly limit, once, for the summary. */
export const limitReason = (lines: AskAllLine[]): string => lines.find((l) => l.why === 'limit')?.reason ?? '';

export const countOf = (lines: AskAllLine[], state: AskAllState): number => lines.filter((l) => l.state === state).length;

/** The words for one line, as a message key; null when the line shows the server's own reason. With no asks left this week (`noRoom`) a movie that would have been requested says so, since it cannot be sent; one that joins someone else's request still can. */
export function lineWords(l: AskAllLine, adding: boolean, noRoom: boolean): string | null {
	if (l.state === 'requested') return adding ? 'requests.addAll.added' : 'requests.askAll.requested';
	if (l.state === 'todo') return adding ? 'requests.addAll.willAdd' : noRoom && !joins(l) ? 'requests.askAll.limit' : 'requests.askAll.willRequest';
	if (l.why === 'library') return 'requests.askAll.inLibrary';
	if (l.why === 'mine') return 'requests.askAll.mine';
	if (l.why === 'limit') return 'requests.askAll.limit';
	return null;
}

/** What a finished line changes for the movie: its Library title (added, or found there) or its ask. Null when nothing changed. */
export function lineNews(l: AskAllLine): { tmdb: string; titleId?: number; ask?: Ask } | null {
	const tmdb = cardTmdb(l.card);
	if (l.state === 'requested' && l.title) return { tmdb, titleId: l.title.id };
	if (l.state === 'requested' && l.ask) return { tmdb, ask: l.ask };
	if (l.why === 'library' && l.card.titleId !== undefined) return { tmdb, titleId: l.card.titleId };
	return null;
}

/** Runs one load of the add choices at a time for the error flag: only the newest load may set or clear it, so a slow older one cannot raise an error over choices that did load. */
export function loadChoices(setFailed: (failed: boolean) => void): (loads: Promise<unknown>[]) => void {
	let latest = 0;
	return (loads) => {
		const mine = ++latest;
		setFailed(false);
		for (const p of loads) p.catch(() => { if (mine === latest) setFailed(true); });
	};
}
