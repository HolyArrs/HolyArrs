import { describe, expect, it } from 'vitest';
import { fakeIpHost } from './fake-ip.ts';

describe('fakeIpHost', () => {
	it('keeps a plain host name', () => {
		expect(fakeIpHost({ fakeIP: 'api.holyarrs.com' })).toBe('api.holyarrs.com');
	});
	it('drops anything that is not a host name', () => {
		expect(fakeIpHost({})).toBe('');
		expect(fakeIpHost(null)).toBe('');
		expect(fakeIpHost({ fakeIP: 42 })).toBe('');
		expect(fakeIpHost({ fakeIP: '<b>x</b>' })).toBe('');
		expect(fakeIpHost({ fakeIP: 'a'.repeat(300) })).toBe('');
	});
});
