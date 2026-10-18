import { describe, expect, it } from 'vitest';
import { attention, downloadLine, downloadSubtitle, greetName, healthFix, isActive } from './home.ts';
import type { HealthCheck } from './types.ts';

const check = (key: string, state: HealthCheck['state'], kind: HealthCheck['kind'] = 'source'): HealthCheck => ({ id: 1, key, kind, subject: 'S', state, message: `${key} broke`, since: '', checked_at: '' });

describe('home', () => {
	it('greets people, not ids', () => {
		expect(greetName('Luis')).toBe('Luis');
		expect(greetName('  Ana María ')).toBe('Ana María');
		expect(greetName('preview-e34f0722f7')).toBe('');
		expect(greetName('e2e-owner')).toBe('');
		expect(greetName('1234567')).toBe('');
		expect(greetName('d41d8cd98f00b204')).toBe('');
		expect(greetName(undefined)).toBe('');
	});

	it('writes the download meta line', () => {
		expect(downloadLine({ progress: 0.4, eta: 600, state: 'downloading', label: '' })).toBe('40% · 10m left');
		expect(downloadLine({ progress: 1.4, state: 'downloading', label: '' })).toBe('100%');
		expect(downloadLine({ progress: 0, state: 'queued', label: '' })).toBe('Waiting to start');
		expect(downloadSubtitle({ covers: 'S01E01-E08, 8 episodes', version: 'Main' })).toBe('S01E01-E08, 8 episodes');
		expect(downloadSubtitle({ year: 2023, version: 'Main' })).toBe('2023');
		expect(downloadSubtitle({ year: 2023, version: '4K' })).toBe('2023 · 4K');
		expect(isActive({ active: false, state: 'stalled' })).toBe(true);
		expect(isActive({ active: false, state: 'finished' })).toBe(false);
	});

	it('lists what needs attention, worst first, each with its fix', () => {
		const items = attention({
			failed: [{ id: 3, title: 'Barbie', title_id: 4, text: '' }],
			decisions: [{ slot_id: 7, title: 'Harbour Lights', reason: 'Ran out.', episode: { label: 'S02E01' } }],
			health: [check('w', 'warning', 'disk_space'), check('e', 'error', 'downloader'), check('ok', 'ok')]
		});
		expect(items.map((i) => i.word)).toEqual(['Problem', 'Failed', 'Your call', 'Warning']);
		expect(items[0].href).toBe('/settings/downloaders');
		expect(items[1].text).toBe('Barbie: The download failed.');
		expect(items[2]).toMatchObject({ text: 'Harbour Lights S02E01: Ran out.', href: '/downloads/decide' });
		expect(items[3].href).toBe('/settings/library-folders');
		expect(healthFix('update')).toBe('/settings/this-app');
	});
});
