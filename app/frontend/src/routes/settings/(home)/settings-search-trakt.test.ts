import { describe, expect, it, vi } from 'vitest';

// Every switch reads as held here, so only entries that no switch hides are found.
vi.mock('#lib/core/settings.ts', async (importOriginal) => ({ ...(await importOriginal<object>()), isHeld: (name?: string) => !!name }));

const { searchSettings } = await import('./settings-search.ts');

describe('Settings search: Trakt', () => {
	it('finds the Trakt key with no switch on', () => {
		const hits = searchSettings('trakt key', () => true).map((e) => e.anchor);
		expect(hits).toContain('trakt-card');
	});
});
