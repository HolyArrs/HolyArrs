// Plain words for health check kinds, results and stage states.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import type { HealthCheck } from '../core/types.ts';
import type { StageState } from './types.ts';

// Server kind to the label users see.
// Getters, so each word follows the language picked when it is read.
const kinds: Record<string, string> = {
	get library_folder() { return t('health.kind.libraryFolder'); },
	get disk_space() { return t('health.kind.diskSpace'); },
	get source() { return t('health.kind.source'); },
	get downloader() { return t('health.kind.downloader'); },
	get backups() { return t('health.kind.backups'); },
	get update() { return t('health.kind.update'); },
	get config_disk() { return t('health.kind.configDisk'); },
	get config_folder() { return t('health.kind.configFolder'); },
	get waiting_downloads() { return t('health.kind.waitingDownloads'); },
	get trailers() { return t('health.kind.trailers'); }
};

// Stage or linked app kind to its label.
const linkKinds: Record<StageState['kind'], string> = {
	get stage() { return t('health.link.stage'); },
	get part() { return t('health.link.part'); },
	get source() { return t('health.link.source'); },
	get downloader() { return t('health.link.downloader'); },
	get subtitle_site() { return t('health.link.subtitleSite'); },
	get media_server() { return t('health.link.mediaServer'); }
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
	return state === 'ok' ? t('health.state.fine') : state === 'warning' ? t('health.state.warning') : state === 'error' ? t('health.state.broken') : state;
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
	if (s.stuck) return t('health.run.stuck');
	const words: Record<string, string> = { off: t('health.run.off'), starting: t('health.run.starting'), running: t('health.run.running'), retrying: t('health.run.retrying'), failed: t('health.run.failed') };
	return words[s.state] ?? s.state;
}

export function runTone(state: StageState['state']): string {
	return state === 'running' ? 'ok' : state === 'failed' ? 'fail' : state === 'off' ? '' : 'wait';
}

// Parts are left out here; they belong to the controls screen.
export function splitStates(ss: StageState[] | null | undefined): { stages: StageState[]; links: StageState[] } {
	const all = ss ?? [];
	return { stages: all.filter((s) => s.kind === 'stage'), links: all.filter((s) => s.kind !== 'stage' && s.kind !== 'part') };
}
