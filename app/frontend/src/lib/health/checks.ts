// Plain words for health check kinds, results and stage states.
import type { HealthCheck } from '../core/types.ts';
import type { StageState } from './types.ts';

// Server kind to the label users see.
const kinds: Record<string, string> = {
	library_folder: 'Library folder',
	disk_space: 'Free space',
	source: 'Source',
	downloader: 'Downloader',
	backups: 'Backups',
	update: 'Update',
	config_disk: 'Config folder space',
	config_folder: 'Config folder',
	waiting_downloads: 'Downloads waiting for files',
	trailers: 'Trailer reader'
};

// Stage or linked app kind to its label.
const linkKinds: Record<StageState['kind'], string> = {
	stage: 'Stage',
	part: 'Part',
	source: 'Source',
	downloader: 'Downloader',
	subtitle_site: 'Subtitle site',
	media_server: 'Media server'
};

export function checkLabel(kind: string): string {
	return kinds[kind as HealthCheck['kind']] ?? kind;
}

// Avoids reading "Backups Backups" when subject and label overlap.
export function checkName(kind: string, subject: string): { label: string; subject: string } {
	const label = checkLabel(kind);
	const s = (subject ?? '').trim();
	if (s && s.toLowerCase().startsWith(label.toLowerCase())) return { label: s, subject: '' };
	if (s && label.toLowerCase().startsWith(s.toLowerCase())) return { label, subject: '' };
	return { label, subject: s };
}

export function stateWord(state: string): string {
	return state === 'ok' ? 'Fine' : state === 'warning' ? 'Warning' : state === 'error' ? 'Broken' : state;
}

// The server already sends the worst checks first, so order is kept.
export function problems(cs: HealthCheck[] | null | undefined): HealthCheck[] {
	return (cs ?? []).filter((c) => c.state !== 'ok');
}

export function linkLabel(kind: string): string {
	return linkKinds[kind as StageState['kind']] ?? kind;
}

// A stuck stage reads Stuck whatever its state says.
export function runWord(s: Pick<StageState, 'state' | 'stuck'>): string {
	if (s.stuck) return 'Stuck';
	return { off: 'Off', starting: 'Starting', running: 'Running', retrying: 'Retrying', failed: 'Failed' }[s.state] ?? s.state;
}

export function runTone(state: StageState['state']): string {
	return state === 'running' ? 'ok' : state === 'failed' ? 'fail' : state === 'off' ? '' : 'wait';
}

// Parts are left out here; they belong to the controls screen.
export function splitStates(ss: StageState[] | null | undefined): { stages: StageState[]; links: StageState[] } {
	const all = ss ?? [];
	return { stages: all.filter((s) => s.kind === 'stage'), links: all.filter((s) => s.kind !== 'stage' && s.kind !== 'part') };
}
