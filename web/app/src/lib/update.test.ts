import { describe, expect, it } from 'vitest';
import { releaseLink, updatePrivacy, updateWords } from './update.ts';

const base = { on: true, feed: 'f', newer: false, version: 'v1.0.0' };

describe('updateWords', () => {
	it('covers each state', () => {
		expect(updateWords({ ...base, on: false })).toContain('off');
		expect(updateWords(base)).toContain('Not checked yet');
		expect(updateWords({ ...base, checked_at: 'x', newer: true, latest: 'v2.0.0' })).toBe('HolyArrs v2.0.0 is out, newer than this one.');
		expect(updateWords({ ...base, checked_at: 'x', problem: 'the release feed did not answer' })).toContain('did not work');
		expect(updateWords({ ...base, checked_at: 'x', no_release: true })).toBe('No HolyArrs release is published yet.');
		expect(updateWords({ ...base, checked_at: 'x' })).toBe('This is the newest version.');
	});
});

describe('releaseLink', () => {
	it('keeps only the project release pages', () => {
		expect(releaseLink('https://github.com/CoderLuii/holyarrs/releases/tag/v1.2.0')).toBe('https://github.com/CoderLuii/holyarrs/releases/tag/v1.2.0');
		for (const bad of [
			undefined,
			'',
			'https://example.com/r/v1',
			'http://github.com/CoderLuii/holyarrs/releases/tag/v1',
			'https://github.com.evil.example/CoderLuii/holyarrs/releases/x',
			'https://github.com/CoderLuii/holyarrs/releasesX/tag/v1',
			'https://github.com/CoderLuii/holyarrs/releases/../../x',
			'https://github.com/CoderLuii/holyarrs/releases/tag/v1?x=1',
			'javascript:alert(1)'
		]) {
			expect(releaseLink(bad)).toBe('');
		}
	});
	it('says what the check sends', () => {
		expect(updatePrivacy('v1.0.0')).toContain('api.github.com');
		expect(updatePrivacy('v1.0.0')).toContain('HolyArrs/v1.0.0');
	});
});
