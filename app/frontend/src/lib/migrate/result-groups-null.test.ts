import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import ResultGroups from './ResultGroups.svelte';
import type { Report } from './move.ts';

describe('an earlier import report with missing lists', () => {
	it('opens when apps, not_moved, needs_input and next are null', () => {
		// A downloader-only import saved before the fix had these as null.
		const report = { move: 1, dry: false, apps: null, at: '', lines: [], counts: {}, not_moved: null, needs_input: null, next: null } as unknown as Report;
		expect(() => render(ResultGroups, { props: { report, groups: [], onshow: () => {} } })).not.toThrow();
	});
});
