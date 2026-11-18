import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const read = (p: string) => readFileSync(fileURLToPath(new URL(p, import.meta.url)), 'utf8');

describe('the import confirm while shows are still checked', () => {
	it('says the import can go ahead, not that a list updates (the confirm has no list)', () => {
		const page = read('../../routes/settings/move/+page.svelte');
		const line = page.split('\n').find((l) => l.includes('data-testid="confirm-checking"')) ?? '';
		const key = /t\('(migrate\.[\w.]+)'/.exec(line)?.[1] ?? '';
		const en = JSON.parse(read('../core/i18n/messages/en/migrate.json')) as Record<string, string>;
		expect(en[key]).toBe('Shows still being checked with the metadata server: {n}. You can import now; they are checked during the import.');
	});
});
