
export interface HeldFile {
	id: number;
	path: string;
	held_path: string;
	folder: string;
	title_id: number;
	title: string;
	kind: string;
	title_deleted: boolean;
	size: number;
	deleted_at: string;
	expires_at?: string;
}

export interface RestoreResult {
	id: number;
	path?: string;
	result: string;
}

export function daysLeft(expires: string | undefined, now = Date.now()): number | null {
	if (!expires) return null;
	const t = Date.parse(expires);
	if (Number.isNaN(t)) return null;
	return Math.max(0, Math.ceil((t - now) / 86_400_000));
}

export function daysLeftWords(expires: string | undefined, now = Date.now()): string {
	const d = daysLeft(expires, now);
	if (d === null) return 'Kept until you empty it';
	if (d === 0) return 'Goes today';
	return d === 1 ? '1 day left' : `${d} days left`;
}

export function fileName(path: string): string {
	return path.slice(path.lastIndexOf('/') + 1);
}
