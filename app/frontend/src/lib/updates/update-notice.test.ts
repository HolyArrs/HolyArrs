import { describe, expect, it } from 'vitest';
import { IMAGE, PROJECT_REPO, goBackBackup, imageTag, plainVersion, releaseLink, whatsNew, updatePrivacy } from './update-notice.ts';

const base = { on: true, feed: 'f', newer: false, version: 'v1.0.0' };

describe('releaseLink', () => {
	it('opens a release link only when it is a project release page', () => {
		expect(PROJECT_REPO).toBe('HolyArrs/HolyArrs');
		expect(releaseLink('https://github.com/HolyArrs/HolyArrs/releases/tag/v1.2.0')).toBe('https://github.com/HolyArrs/HolyArrs/releases/tag/v1.2.0');
		// Until the backend check moves to the new repo.
		expect(releaseLink('https://github.com/HolyArrs/HolyArrs/releases/tag/v1.2.0')).toBe('https://github.com/HolyArrs/HolyArrs/releases/tag/v1.2.0');
		for (const bad of [
			undefined,
			'',
			'https://example.com/r/v1',
			'http://github.com/HolyArrs/HolyArrs/releases/tag/v1',
			'https://github.com.evil.example/HolyArrs/HolyArrs/releases/x',
			'https://github.com/HolyArrs/HolyArrs/releasesX/tag/v1',
			'https://github.com/HolyArrs/HolyArrs-evil/releases/tag/v1',
			'https://github.com/HolyArrs/HolyArrs/releases/../../x',
			'https://github.com/HolyArrs/HolyArrs/releases/../../x',
			'https://github.com/HolyArrs/HolyArrs/releases/tag/v1?x=1',
			'javascript:alert(1)'
		]) {
			expect(releaseLink(bad)).toBe('');
		}
	});
	it('shows what the update check sends', () => {
		expect(updatePrivacy('v1.0.0')).toContain('api.github.com');
		expect(updatePrivacy('v1.0.0')).toContain('HolyArrs/v1.0.0');
	});
});

describe('goBackBackup', () => {
	const pre = (from: string, to: string, at: string) => ({ name: `holyarrs-backup-pre-update-${at}.zip`, kind: 'pre-update', created_at: at, from_version: from, to_version: to });
	it('offers the backup taken before the update to this version', () => {
		const list = [pre('1.1.0', '1.2.0', '3'), pre('1.0.0', '1.1.0', '2'), { name: 'm', kind: 'manual', created_at: '1' }];
		expect(goBackBackup(list, 'v1.2.0')?.from_version).toBe('1.1.0');
		expect(goBackBackup(list, '1.1.0')?.from_version).toBe('1.0.0');
		expect(goBackBackup(list, '1.3.0')).toBeNull();
		expect(goBackBackup([{ name: 'x', kind: 'pre-update', created_at: '1', detail: 'before updating the data (main 1 to 2)' }], '1.2.0')).toBeNull();
	});
	it('shows the image tag without a leading v', () => {
		expect(imageTag('v1.1.0')).toBe('1.1.0');
		expect(imageTag('1.1.0')).toBe('1.1.0');
		expect(plainVersion(' V2.0.0-rc.1')).toBe('2.0.0-rc.1');
		expect(`${IMAGE}:${imageTag('v1.1.0')}`).toBe('holyarrs/holyarrs:1.1.0');
	});
});

describe('whatsNew', () => {
	const u = { version: 'v1.0.0', newer: false, notes: 'Faster Explore.\n\n Fixed search. ', link: 'L' };
	it('lists this release with its note lines when it is the newest', () => {
		expect(whatsNew(null)).toEqual([]);
		expect(whatsNew(u)).toEqual([{ version: '1.0.0', lines: ['Faster Explore.', 'Fixed search.'], link: 'L' }]);
	});
	it('puts the newer release first and gives it the notes', () => {
		const got = whatsNew({ ...u, newer: true, latest: 'v1.1.0' });
		expect(got.map((r) => r.version)).toEqual(['1.1.0', '1.0.0']);
		expect(got[0].lines).toEqual(['Faster Explore.', 'Fixed search.']);
		expect(got[1]).toEqual({ version: '1.0.0', lines: [], link: undefined });
	});
});
