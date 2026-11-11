import { describe, expect, it } from 'vitest';
import { testCount, testLabel, testVersion } from './test-tag.ts';

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
		expect(testLabel('Prowlarr 1.30.2.4939 answered.', 'Prowlarr', '3 indexers')).toBe('Prowlarr v1.30.2 · 3 indexers');
		expect(testLabel('The Source answered.')).toBe('Connected');
		expect(testLabel('The feed answered with 2 Releases.', '', testCount('The feed answered with 2 Releases.'))).toBe('Connected · 2 Releases');
	});
});
