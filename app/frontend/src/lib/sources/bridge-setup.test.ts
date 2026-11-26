import { describe, expect, it } from 'vitest';
import { bridgeKeyPage, joinAddress, splitAddress } from './bridge-setup.ts';

describe('bridge setup address', () => {
	it('joins and splits the three boxes', () => {
		expect(joinAddress(false, ' 192.168.1.10 ', '9696')).toBe('http://192.168.1.10:9696');
		expect(joinAddress(true, 'jackett.lan', '', '/jackett')).toBe('https://jackett.lan/jackett');
		expect(joinAddress(false, '', '9117')).toBe('');
		expect(splitAddress('http://10.0.0.5:9117/jackett/', '9117')).toEqual({ ssl: false, host: '10.0.0.5', port: '9117', basePath: '/jackett' });
		expect(splitAddress('10.0.0.5:9696', '9696')).toEqual({ ssl: false, host: '10.0.0.5', port: '9696', basePath: '' });
	});
	it('opens each app at its key page', () => {
		expect(bridgeKeyPage('prowlarr', false, '10.0.0.5', '9696')).toBe('http://10.0.0.5:9696/settings/general');
		expect(bridgeKeyPage('jackett', false, '10.0.0.5', '9117')).toBe('http://10.0.0.5:9117/UI/Dashboard');
		// A Docker name only HolyArrs can reach is not offered as a link.
		expect(bridgeKeyPage('jackett', false, 'jackett', '9117')).toBe('');
	});
});
