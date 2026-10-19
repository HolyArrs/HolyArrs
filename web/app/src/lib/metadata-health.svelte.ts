import { api } from '#lib/api.ts';

/** The notice shown while the metadata server is not answering. */
export const METADATA_DOWN = "Metadata server is down — searches and new artwork will return when it's back";

const TTL = 60_000;

/** metadataHealth is the shared, cached answer of GET /metadata/health. */
export const metadataHealth = $state<{ up: boolean; checked: number }>({ up: true, checked: 0 });

let pending: Promise<boolean> | null = null;

/** checkMetadata asks the server at most once a minute; a failed ask keeps the last answer. */
export function checkMetadata(force = false): Promise<boolean> {
	if (!force && metadataHealth.checked && Date.now() - metadataHealth.checked < TTL) return Promise.resolve(metadataHealth.up);
	pending ??= api
		.get<{ up: boolean }>('/metadata/health')
		.then((r) => {
			metadataHealth.up = r.up !== false;
			return metadataHealth.up;
		})
		.catch(() => metadataHealth.up)
		.finally(() => {
			metadataHealth.checked = Date.now();
			pending = null;
		});
	return pending;
}
