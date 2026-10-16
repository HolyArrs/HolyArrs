// The log viewer: filters to a query, and words for levels.

export type LogLevel = 'info' | 'warning' | 'error';

export interface LogLine {
	at: string;
	level: LogLevel;
	text: string;
}

export interface LogPage {
	lines: LogLine[];
	total: number;
	offset: number;
	limit: number;
}

export interface LogFilter {
	level: '' | LogLevel;
	q: string;
	from: string;
	to: string;
	offset: number;
	limit: number;
}

export const emptyFilter = (limit = 100): LogFilter => ({ level: '', q: '', from: '', to: '', offset: 0, limit });

/** logQuery is the query string for GET /logs (empty fields left out). */
export function logQuery(f: LogFilter): string {
	const p = new URLSearchParams();
	if (f.level) p.set('level', f.level);
	if (f.q.trim()) p.set('q', f.q.trim());
	if (f.from) p.set('from', f.from);
	if (f.to) p.set('to', f.to);
	if (f.offset > 0) p.set('offset', String(f.offset));
	p.set('limit', String(f.limit));
	return p.toString();
}

export const levelWord: Record<LogLevel, string> = { info: 'Info', warning: 'Warning', error: 'Error' };

/** levelTone is the chip colour for a level. */
export function levelTone(l: LogLevel): string {
	return l === 'error' ? 'fail' : l === 'warning' ? 'wait' : '';
}

/** pageWords says which lines of how many are shown. */
export function pageWords(p: Pick<LogPage, 'total' | 'offset'>, shown: number): string {
	if (p.total === 0) return 'No lines match.';
	return `Lines ${p.offset + 1} to ${p.offset + shown} of ${p.total}, newest first.`;
}
