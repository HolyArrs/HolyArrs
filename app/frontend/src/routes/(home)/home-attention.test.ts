import { afterEach, describe, expect, it } from 'vitest';
import { createHomeAttention } from './home-attention.svelte.ts';
import { partsNow, type PartsAnswer } from '#lib/mediafiles/index.ts';

// Home with one Library folder and one working Downloader, no Source, set up and read.
function home(sources: { name: string; state: string }[] = []) {
	return createHomeAttention({
		checks: { checks: [] },
		health: { sources, downloaders: [{ name: 'SABnzbd', state: 'working' }] },
		settled: true,
		uncheckedParts: false,
		folders: [{ id: 1 }],
		owner: true,
		rows: [],
		admin: true,
		downloads: [],
		manager: true,
		uncheckedDl: false,
		uncheckedHealth: false,
		home: null,
		asks: []
	} as never);
}

function sourcesSwitch(on: boolean): PartsAnswer {
	return { parts: [{ part: 'sources', name: 'Sources', on, state: on ? 'running' : 'off' }], screens: {}, safe_mode: false };
}

afterEach(() => {
	partsNow.answer = null;
});

describe('Home and the Sources part', () => {
	it('asks for a Source while Sources is on', () => {
		partsNow.answer = sourcesSwitch(true);
		const v = home();
		expect(v.steps.map((s) => s.href)).toContain('/settings/sources');
		expect(v.fixes.map((f) => f.key)).toContain('no-sources');
	});

	it('asks for no Source when Sources is off: no setup step, no warning, even for one that is failing', () => {
		partsNow.answer = sourcesSwitch(false);
		let v = home();
		expect(v.steps.map((s) => s.href)).not.toContain('/settings/sources');
		expect(v.nextStep.href).not.toBe('/settings/sources');
		expect(v.fixes.filter((f) => f.href === '/settings/sources')).toEqual([]);
		v = home([{ name: 'Lab', state: 'unreachable' }]);
		expect(v.fixes.filter((f) => f.href === '/settings/sources')).toEqual([]);
	});
});
