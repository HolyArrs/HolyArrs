import { describe, expect, it } from 'vitest';
import { collectionBody, collectionResultLine, collectionStart } from './collections.ts';

describe('collection removal choices', () => {
	it('keeps files and permits outside-list re-addition by default', () => {
		expect(collectionStart()).toEqual({ delete_files: false, never_add: false, acknowledge_protection: false });
	});

	it('sorts the exact reviewed selection and carries its token', () => {
		expect(collectionBody([8, 2], { delete_files: true, never_add: true, acknowledge_protection: true }, 'review')).toEqual({
			title_ids: [2, 8], delete_files: true, never_add: true, acknowledge_protection: true, review_token: 'review'
		});
	});

	it('explains partial outcomes', () => {
		expect(collectionResultLine({ title_id: 1, name: 'One', status: 'skipped', error: 'protected copy' })).toContain('stayed');
		expect(collectionResultLine({ title_id: 2, name: 'Two', status: 'failed', error: 'busy' })).toContain('busy');
	});
});
