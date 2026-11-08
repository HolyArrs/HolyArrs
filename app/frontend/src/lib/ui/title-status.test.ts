import { describe, expect, it } from 'vitest';
import { audienceOf, audienceStatus, factsOf, statusLabel, titleStatus } from './title-status.ts';
import type { SlotView } from '../core/types.ts';

const slot = (s: Partial<SlotView>): SlotView => ({ id: 1, version: 'Main', version_id: 1, state: 'wanted', state_label: '', ...s });

describe('title status words', () => {
	it('uses the member words', () => {
		expect(titleStatus({}, 'member')).toEqual({ kind: 'missing', label: 'Get it' });
		expect(titleStatus({ ask: 'asked' }, 'member')).toEqual({ kind: 'requested', label: 'Requested' });
		expect(titleStatus({ downloading: true }, 'member')).toEqual({ kind: 'downloading', label: 'Downloading' });
		expect(titleStatus({ inLibrary: true }, 'member')).toEqual({ kind: 'in_library', label: 'Ready to watch' });
	});

	it('says ready to listen for music', () => {
		expect(titleStatus({ inLibrary: true, titleKind: 'artist' }, 'member').label).toBe('Ready to listen');
	});

	it('uses the more precise staff words', () => {
		expect(titleStatus({}, 'staff')).toEqual({ kind: 'missing', label: 'Missing' });
		expect(titleStatus({ ask: 'asked' }, 'staff')).toEqual({ kind: 'requested', label: 'Waiting' });
		expect(titleStatus({ inLibrary: true }, 'staff')).toEqual({ kind: 'in_library', label: 'In your Library' });
		expect(titleStatus({ failed: true }, 'staff')).toEqual({ kind: 'failed', label: 'Failed' });
		expect(titleStatus({ comingSoon: true }, 'staff')).toEqual({ kind: 'coming_soon', label: 'Coming soon' });
	});

	it('adds the percent while downloading', () => {
		expect(titleStatus({ downloading: true, percent: 41.6 }, 'member').label).toBe('Downloading 42%');
	});

	it('follows an ask through approval and turning down', () => {
		expect(titleStatus({ ask: 'approved' }, 'member')).toEqual({ kind: 'approved', label: 'Approved' });
		expect(titleStatus({ ask: 'turned_down' }, 'member')).toEqual({ kind: 'turned_down', label: 'Declined' });
		expect(titleStatus({ ask: 'available' }, 'member')).toEqual({ kind: 'in_library', label: 'Ready to watch' });
	});

	it('ranks downloading over the Library over an ask', () => {
		expect(titleStatus({ downloading: true, inLibrary: true, ask: 'asked' }, 'staff').kind).toBe('downloading');
		expect(titleStatus({ inLibrary: true, ask: 'asked' }, 'staff').kind).toBe('in_library');
	});

	it('never shows a failed download to members', () => {
		expect(titleStatus({ failed: true, ask: 'approved' }, 'member')).toEqual({ kind: 'approved', label: 'Approved' });
		expect(titleStatus({ failed: true }, 'member').label).toBe('Get it');
	});

	it('tells members a Library title without a file is already on its way', () => {
		expect(titleStatus({ tracked: true }, 'member')).toEqual({ kind: 'requested', label: 'Requested' });
		expect(titleStatus({ tracked: true }, 'staff')).toEqual({ kind: 'missing', label: 'Missing' });
	});

	it('says when the ask is the person’s own', () => {
		expect(titleStatus({ ask: 'asked', mine: true }, 'member')).toEqual({ kind: 'requested', label: 'You asked · Requested' });
		expect(titleStatus({ ask: 'asked', mine: true }, 'staff')).toEqual({ kind: 'requested', label: 'You asked · Waiting' });
		expect(titleStatus({ ask: 'approved', mine: true }, 'member').label).toBe('You asked · Approved');
		expect(titleStatus({ ask: 'asked', mine: false }, 'member').label).toBe('Requested');
		expect(titleStatus({ ask: 'available', mine: true, inLibrary: true }, 'member').label).toBe('Ready to watch');
		expect(titleStatus({ ask: 'asked', mine: true, downloading: true }, 'member').label).toBe('Downloading');
	});

	it('names the Never-add list', () => {
		expect(titleStatus({ neverAdd: true }, 'staff')).toEqual({ kind: 'turned_down', label: 'On the Never-add list' });
	});

	it('says a known status kind for the audience', () => {
		expect(audienceStatus('failed', 'member', { tracked: true })).toEqual({ kind: 'requested', label: 'Requested' });
		expect(audienceStatus('failed', 'staff', { tracked: true })).toEqual({ kind: 'failed', label: 'Failed' });
		expect(audienceStatus('missing', 'member', { tracked: true })).toEqual({ kind: 'requested', label: 'Requested' });
		expect(audienceStatus('coming_soon', 'member')).toEqual({ kind: 'coming_soon', label: 'Coming soon' });
	});

	it('gives one word per kind and audience', () => {
		expect(statusLabel('requested', 'member')).toBe('Requested');
		expect(statusLabel('in_library', 'staff')).toBe('In your Library');
	});
});

describe('title status facts', () => {
	it('reads slots and asks', () => {
		const f = factsOf({ slots: [slot({ state: 'downloading', download: { active: true, progress: 0.5 } as SlotView['download'] })], asks: [{ state: 'asked' }] });
		expect(f).toMatchObject({ downloading: true, percent: 50, ask: 'asked', inLibrary: false });
		expect(factsOf({ slots: [slot({ state: 'better_wanted' })] }).inLibrary).toBe(true);
		expect(factsOf({ slots: [slot({ state: 'not_out_yet' })] }).comingSoon).toBe(true);
		expect(factsOf({ slots: [slot({ state: 'failed' })] }).failed).toBe(true);
	});

	it('keeps the open ask over a turned down one', () => {
		expect(factsOf({ asks: [{ state: 'turned_down' }, { state: 'approved' }] }).ask).toBe('approved');
		expect(factsOf({ asks: [{ state: 'turned_down' }] }).ask).toBe('turned_down');
	});
});

describe('title status audience', () => {
	it('gives staff words to people who manage the Library or approve', () => {
		expect(audienceOf((p) => p === 'manage_library')).toBe('staff');
		expect(audienceOf((p) => p === 'approve')).toBe('staff');
		expect(audienceOf(() => false)).toBe('member');
	});
});
