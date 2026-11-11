import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// A small stand-in for <html> and localStorage, since the tests run in node.
function fakePage(stored: Record<string, string> = {}, dataset: Record<string, string> = {}) {
	const store = { ...stored };
	vi.stubGlobal('document', { documentElement: { dataset } });
	vi.stubGlobal('localStorage', {
		getItem: (k: string) => store[k] ?? null,
		setItem: (k: string, v: string) => void (store[k] = v),
		removeItem: (k: string) => void delete store[k]
	});
	return { store, dataset };
}

async function load() {
	vi.resetModules();
	return import('./look.svelte.ts');
}

describe('look', () => {
	beforeEach(() => vi.unstubAllGlobals());
	afterEach(() => vi.unstubAllGlobals());

	it('starts full when nothing is saved', async () => {
		fakePage();
		const m = await load();
		expect(m.look.value).toBe('full');
		expect(m.minimalLook()).toBe(false);
	});

	it('starts minimal when this browser saved it', async () => {
		fakePage({ 'holyarrs-look': 'minimal' });
		expect((await load()).minimalLook()).toBe(true);
	});

	it('starts minimal when the first paint already set it on <html>', async () => {
		fakePage({}, { look: 'minimal' });
		expect((await load()).minimalLook()).toBe(true);
	});

	it('minimal sets <html data-look> and saves it; full removes both', async () => {
		const { store, dataset } = fakePage();
		const m = await load();
		m.setLook('minimal');
		expect(dataset.look).toBe('minimal');
		expect(store['holyarrs-look']).toBe('minimal');
		expect(m.minimalLook()).toBe(true);
		m.setLook('full');
		expect(dataset.look).toBeUndefined();
		expect('holyarrs-look' in store).toBe(false);
		expect(m.minimalLook()).toBe(false);
	});

	it('still switches for this visit when storage refuses', async () => {
		const { dataset } = fakePage();
		vi.stubGlobal('localStorage', {
			getItem: () => null,
			setItem: () => {
				throw new Error('denied');
			},
			removeItem: () => {
				throw new Error('denied');
			}
		});
		const m = await load();
		m.setLook('minimal');
		expect(m.minimalLook()).toBe(true);
		expect(dataset.look).toBe('minimal');
	});

	it('is full with no document (server render)', async () => {
		const m = await load();
		expect(m.look.value).toBe('full');
	});
});
