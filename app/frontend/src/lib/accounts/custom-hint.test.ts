import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

const root = join(import.meta.dirname, '../core/i18n/messages');
const langs = readdirSync(root).filter((l) => l !== 'en');

describe('Custom hint names the button on screen', () => {
	for (const lang of langs) {
		it(lang, () => {
			const m = JSON.parse(readFileSync(join(root, lang, 'settings-users.json'), 'utf8'));
			expect(m['settings.users.customHint']).toContain(m['settings.users.custom']);
		});
	}
	it('sr stays in one script', () => {
		const m = JSON.parse(readFileSync(join(root, 'sr', 'settings-users.json'), 'utf8'));
		expect(m['settings.users.customHint']).not.toMatch(/[A-Za-z]/);
	});
});
