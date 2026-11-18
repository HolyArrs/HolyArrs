import { describe, expect, it } from 'vitest';
import { connectHelpUrl, testCount, testLabel, testVersion } from './test-tag.ts';

describe('test tag words', () => {
	it('keeps three parts of a version', () => {
		expect(testVersion('Radarr 6.0.4.10291')).toBe('v6.0.4');
		expect(testVersion('It answered: qbittorrent v5.1.2.')).toBe('v5.1.2');
		expect(testVersion('The Source answered.')).toBe('');
		expect(testVersion('reached http://192.168.1.10:8080')).toBe('');
	});
	it('names the app and adds a count', () => {
		expect(testLabel('It answered: qbittorrent 5.1.2.')).toBe('qBittorrent v5.1.2');
		expect(testLabel('It answered: rtorrent 0.16.25')).toBe('rTorrent v0.16.25');
		expect(testLabel('It answered: aria2 1.36.0.')).toBe('aria2 v1.36.0');
		expect(testLabel('Prowlarr 1.30.2.4939 answered.', 'Prowlarr', '3 indexers')).toBe('Prowlarr v1.30.2 · 3 indexers');
		expect(testLabel('The Source answered.')).toBe('Connected');
		expect(testLabel('The feed answered with 2 Releases.', '', testCount('The feed answered with 2 Releases.'))).toBe('Connected · 2 Releases');
	});
});

describe('connectHelpUrl', () => {
	const base = 'https://docs.holyarrs.com/start/connect/#if-it-does-not-work';
	it('links each name case to its row of the Connect page table', () => {
		const nf = (n: string) => `${n} → not found. HolyArrs can't find this name. Use the IP instead.`;
		expect(connectHelpUrl(nf('qbittorrent'))).toBe(`${base}:~:text=A%20container%20name%20like`);
		expect(connectHelpUrl(nf('nas.local'))).toBe(`${base}:~:text=A%20.local%20name%20like`);
		expect(connectHelpUrl(nf('nas.tail1234.ts.net'))).toBe(`${base}:~:text=A%20Tailscale%20name`);
		expect(connectHelpUrl(nf('nas.home'))).toBe(`${base}:~:text=A%20Pi%2Dhole%20or%20router%20name`);
		expect(connectHelpUrl('qbittorrent → 172.18.0.5 → refused on port 8080. The name works, but nothing answers on that port. Check the port.')).toBe(`${base}:~:text=Not%20answering`);
		expect(connectHelpUrl('sab → 10.0.0.2 → no answer on port 8080. The name works, but nothing answers in time. Check the port, and that no firewall is in the way.')).toBe(`${base}:~:text=Not%20answering`);
	});
	it('gives nothing for other lines', () => {
		expect(connectHelpUrl('Key or password refused. Check them, then test again.')).toBe('');
		expect(connectHelpUrl('')).toBe('');
		expect(connectHelpUrl(undefined)).toBe('');
	});
});
