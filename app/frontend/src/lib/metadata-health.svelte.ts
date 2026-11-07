import { api } from '#lib/api.ts';
import { nextCheckIn, outageDismiss, outageShown, outageStart, outageStep, type OutageState } from '#lib/outage.ts';

/** The words of the notification shown while the metadata server is not answering. */
export const METADATA_DOWN_TITLE = 'Metadata offline';
export const METADATA_DOWN = "Search and new artwork pause until it's back. Your Library still works.";
export const METADATA_BACK = 'Metadata back online';

const TTL = 60_000;
/** Dev only: set localStorage `holyarrs.dev.metadataDown` to "1" to act as if checks fail. */
const DEV_FLAG = 'holyarrs.dev.metadataDown';

/** metadataHealth is the shared, cached answer of GET /metadata/health. */
export const metadataHealth = $state<{ up: boolean; checked: number }>({ up: true, checked: 0 });

/** metadataOutage is the debounced outage: what the notification follows. */
export const metadataOutage = $state<{ s: OutageState; back: number }>({ s: outageStart(), back: 0 });

let pending: Promise<boolean> | null = null;

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

/** checkMetadata asks the server at most once a minute; a failed ask counts toward an outage. */
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

/** watchMetadata keeps checking in the background, sooner while a check has failed. */
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
