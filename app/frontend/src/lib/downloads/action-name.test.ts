import { actionName } from './action-name.ts';
import { describe, expect, it } from 'vitest';

describe('actionName', () => {
	it('is the label when there is no hint or the hint repeats it', () => {
		expect(actionName({ label: 'Pause', hint: '' })).toBe('Pause');
		expect(actionName({ label: 'Pause', hint: 'Pause' })).toBe('Pause');
	});

	it('is the hint when the hint already holds the label', () => {
		expect(actionName({ label: 'Start', hint: 'Start (select a download)' })).toBe('Start (select a download)');
		expect(actionName({ label: 'Speed limit', hint: 'Speed limit: 42 MB/s' })).toBe('Speed limit: 42 MB/s');
	});

	it('joins a longer hint to the label with a colon, not a dash', () => {
		const name = actionName({ label: 'Remove', hint: 'Take the selected downloads off the list' });
		expect(name).toBe('Remove: Take the selected downloads off the list');
		expect(name).not.toContain('\u2014');
	});
});
