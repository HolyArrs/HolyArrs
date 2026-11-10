import { api } from '#lib/core/api.ts';
import { t } from '../core/i18n/i18n.svelte.ts';
import { nextCheckIn, outageDismiss, outageShown, outageStart, outageStep, type OutageState } from './outage.ts';

// The same lines in the person's language; call these at render so they follow a language change.
export const metadataDownTitle = () => t('metadata.health.downTitle');
export const metadataDownText = () => t('metadata.health.down');
export const metadataBackTitle = () => t('metadata.health.back');

const TTL = 60_000;
// Dev only: set this localStorage key to "1" to make checks fail.
const DEV_FLAG = 'holyarrs.dev.metadataDown';

// Shared, cached answer of the last health check.
export const metadataHealth = $state<{ up: boolean; checked: number }>({ up: true, checked: 0 });

// Debounced outage state that the toast follows.
export const metadataOutage = $state<{ s: OutageState; back: number }>({ s: outageStart(), back: 0 });

let pending: Promise<boolean> | null = null;

export function checkMetadata(force = false): Promise<boolean> {
	if (!force && metadataHealth.checked && Date.now() - metadataHealth.checked < TTL) return Promise.resolve(metadataHealth.up);
	pending ??= (devDown() ? Promise.reject(new Error('dev flag')) : api.get<{ up: boolean }>('/metadata/health'))
		.then((r) => {
			metadataHealth.up = r.up !== false;
			record(metadataHealth.up);
			return metadataHealth.up;
		})
		.catch(() => {
			record(false);
			if (metadataOutage.s.down) metadataHealth.up = false;
			return metadataHealth.up;
		})
		.finally(() => {
			metadataHealth.checked = Date.now();
			pending = null;
		});
	return pending;
}

export function watchMetadata(): () => void {
	let timer: ReturnType<typeof setTimeout> | undefined;
	let stopped = false;
	const loop = async () => {
		await checkMetadata(true);
		if (!stopped) timer = setTimeout(loop, nextCheckIn(metadataOutage.s));
	};
	void loop();
	return () => {
		stopped = true;
		clearTimeout(timer);
	};
}

export const metadataDownShown = () => outageShown(metadataOutage.s);
export function dismissMetadataDown() {
	metadataOutage.s = outageDismiss(metadataOutage.s);
}
export function clearMetadataBack() {
	metadataOutage.back = 0;
}

function devDown(): boolean {
	if (!import.meta.env.DEV || typeof localStorage === 'undefined') return false;
	try {
		return localStorage.getItem(DEV_FLAG) === '1';
	} catch {
		return false;
	}
}

function record(ok: boolean) {
	const r = outageStep(metadataOutage.s, ok, Date.now());
	metadataOutage.s = r.state;
	if (r.event === 'up') metadataOutage.back = Date.now();
	if (r.event === 'down') metadataOutage.back = 0;
}
