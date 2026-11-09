import { afterEach, describe, expect, it } from 'vitest';
import { appPath, getBase, setBase, to } from './paths.ts';

afterEach(() => setBase(''));

describe('paths', () => {
	it('leaves everything alone without a base', () => {
		expect(to('/library')).toBe('/library');
		expect(appPath('/library/1')).toBe('/library/1');
	});
	it('puts the base on app paths only', () => {
		setBase('/holyarrs/');
		expect(getBase()).toBe('/holyarrs');
		expect(to('/library?x=1')).toBe('/holyarrs/library?x=1');
		expect(to('/api/v1/logs/download')).toBe('/holyarrs/api/v1/logs/download');
		expect(to('https://example.com/a')).toBe('https://example.com/a');
		expect(to('//evil.example/x')).toBe('//evil.example/x');
		expect(to('#top')).toBe('#top');
	});
	it('takes the base off address paths', () => {
		setBase('/holyarrs');
		expect(appPath('/holyarrs')).toBe('/');
		expect(appPath('/holyarrs/settings/logs')).toBe('/settings/logs');
		expect(appPath('/holyarrsx/a')).toBe('/holyarrsx/a');
	});
});
