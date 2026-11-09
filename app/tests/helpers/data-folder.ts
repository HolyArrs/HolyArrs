import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

// The e2e data folder (app/tests/.tmp) and the .tmp-<name> folders of the side
// configs. setups/*-server.mjs make the same folders; keep both in step.
export const tmp = join(dirname(fileURLToPath(import.meta.url)), '..', '.tmp');

/** dataDir is app/tests/.tmp-<name>, a side config's own data folder. */
export function dataDir(name: string): string {
	return join(dirname(fileURLToPath(import.meta.url)), '..', `.tmp-${name}`);
}
