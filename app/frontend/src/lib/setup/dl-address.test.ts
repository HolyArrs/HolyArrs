import { describe, expect, it } from 'vitest';
import { joinDlAddress, splitDlAddress, sabKeyPage } from './dl-address.ts';

describe('downloader address boxes', () => {
	it('splits and joins an address', () => {
		expect(splitDlAddress('https://nas.lan:8443/sab', '8080')).toEqual({ ssl: true, host: 'nas.lan', port: '8443', basePath: '/sab' });
		expect(splitDlAddress('192.168.1.5:8080', '9091')).toMatchObject({ ssl: false, host: '192.168.1.5', port: '8080' });
		expect(joinDlAddress(false, '192.168.1.5', '8080')).toBe('http://192.168.1.5:8080');
		expect(joinDlAddress(true, ' ', '8080')).toBe('');
	});
	it('keeps the default port with no host', () => {
		expect(splitDlAddress('http://', '6789')).toEqual({ ssl: false, host: '', port: '6789', basePath: '' });
	});
	it('gives SABnzbd key page only for a reachable-looking host', () => {
		expect(sabKeyPage(false, '192.168.1.5', '8080')).toBe('http://192.168.1.5:8080/config/general/');
		expect(sabKeyPage(false, 'sabnzbd', '8080')).toBe('');
	});
});
