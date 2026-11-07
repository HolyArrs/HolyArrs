// Log viewer helpers: turn filters into a query and levels into words.

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

export const levelWord: Record<LogLevel, string> = { info: 'Info', warning: 'Warning', error: 'Error' };

export const emptyFilter = (limit = 100): LogFilter => ({ level: '', q: '', from: '', to: '', offset: 0, limit });

// Empty fields are left out so the URL stays short and the server uses its defaults.
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

export function levelTone(l: LogLevel): string {
	return l === 'error' ? 'fail' : l === 'warning' ? 'wait' : '';
}

// Offset is zero based, so the first shown line is offset + 1.
export function pageWords(p: Pick<LogPage, 'total' | 'offset'>, shown: number): string {
	if (p.total === 0) return 'No lines match.';
	return `Lines ${p.offset + 1} to ${p.offset + shown} of ${p.total}, newest first.`;
}
