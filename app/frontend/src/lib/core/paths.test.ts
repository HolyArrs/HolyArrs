import { afterEach, describe, expect, it } from 'vitest';
import { appPath, getBase, setBase, to, toOwn } from './paths.ts';

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

describe('toOwn', () => {
	it('puts the base on its own picture addresses', () => {
		setBase('/holyarrs');
		expect(toOwn('/api/v1/artwork/a.jpg')).toBe('/holyarrs/api/v1/artwork/a.jpg');
		expect(toOwn('/api/v1/artwork/a.jpg?size=250')).toBe('/holyarrs/api/v1/artwork/a.jpg?size=250');
	});
	it('does it only once', () => {
		setBase('/holyarrs');
		expect(toOwn('/holyarrs/api/v1/artwork/a.jpg')).toBe('/holyarrs/api/v1/artwork/a.jpg');
		setBase('/api');
		expect(toOwn('/api/v1/artwork/a.jpg')).toBe('/api/api/v1/artwork/a.jpg');
		expect(toOwn('/api/api/v1/artwork/a.jpg')).toBe('/api/api/v1/artwork/a.jpg');
	});
	it('leaves other addresses alone', () => {
		setBase('/holyarrs');
		expect(toOwn('https://image.tmdb.org/t/p/w500/a.jpg')).toBe('https://image.tmdb.org/t/p/w500/a.jpg');
		expect(toOwn('//evil.example/api/v1/x')).toBe('//evil.example/api/v1/x');
		expect(toOwn('/brand/mark.svg')).toBe('/brand/mark.svg');
		expect(toOwn('')).toBe('');
	});
	it('changes nothing without a base', () => {
		expect(toOwn('/api/v1/artwork/a.jpg')).toBe('/api/v1/artwork/a.jpg');
	});
});
