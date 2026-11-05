// Health checks: words for their kinds and results.
import type { HealthCheck, StageState } from './types.ts';

const kinds: Record<HealthCheck['kind'], string> = {
	library_folder: 'Library folder',
	disk_space: 'Free space',
	source: 'Source',
	downloader: 'Downloader',
	backups: 'Backups',
	update: 'Update',
	config_disk: 'Config folder space',
	config_folder: 'Config folder',
	waiting_downloads: 'Downloads waiting for files'
};

/** checkLabel names what a check looks at. */
export function checkLabel(kind: string): string {
	return kinds[kind as HealthCheck['kind']] ?? kind;
}

/** stateWord says a result in a word. */
export function stateWord(state: string): string {
	return state === 'ok' ? 'Fine' : state === 'warning' ? 'Warning' : state === 'error' ? 'Broken' : state;
}

/** problems keeps the checks that are not fine, worst first (as the server sends them). */
export function problems(cs: HealthCheck[] | null | undefined): HealthCheck[] {
	return (cs ?? []).filter((c) => c.state !== 'ok');
}

const linkKinds: Record<StageState['kind'], string> = {
	stage: 'Stage',
	part: 'Part',
	source: 'Source',
	downloader: 'Downloader',
	subtitle_site: 'Subtitle site',
	media_server: 'Media server'
};

/** linkLabel names the kind of a stage or linked app. */
export function linkLabel(kind: string): string {
	return linkKinds[kind as StageState['kind']] ?? kind;
}

/** runWord says a stage state in a word; a stuck stage says Stuck. */
export function runWord(s: Pick<StageState, 'state' | 'stuck'>): string {
	if (s.stuck) return 'Stuck';
	return { off: 'Off', starting: 'Starting', running: 'Running', retrying: 'Retrying', failed: 'Failed' }[s.state] ?? s.state;
}

/** runTone is the chip colour for a stage state. */
export function runTone(state: StageState['state']): string {
	return state === 'running' ? 'ok' : state === 'failed' ? 'fail' : state === 'off' ? '' : 'wait';
}

/** splitStates puts stages first and linked apps second, as the server orders them; parts are left out (the controls screen comes later). */
export function splitStates(ss: StageState[] | null | undefined): { stages: StageState[]; links: StageState[] } {
	const all = ss ?? [];
	return { stages: all.filter((s) => s.kind === 'stage'), links: all.filter((s) => s.kind !== 'stage' && s.kind !== 'part') };
}
