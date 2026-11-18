import { describe, expect, it } from 'vitest';
import { copyActions, copyBadges, editionDevice, editionProblem, editionRefused, saveThenReload, showSaved } from './copy-menu.ts';

describe('movie copy menu', () => {
	it('offers nothing for a Version that has no file and cannot be removed', () => {
		expect(copyActions({ main: true }, undefined)).toEqual([]);
	});

	it('offers edition, protection and keep-and-add for a copy that exists', () => {
		expect(copyActions({ main: true }, { Protected: false })).toEqual(['edition', 'protect', 'keepAdd']);
	});

	it('offers to remove protection once a copy is protected', () => {
		expect(copyActions({ main: true }, { Protected: true })).toEqual(['edition', 'unprotect', 'keepAdd']);
	});

	it('keeps removing a second Version, with or without a file', () => {
		expect(copyActions({ main: false }, { Protected: false })).toEqual(['edition', 'protect', 'keepAdd', 'remove']);
		expect(copyActions({ main: false }, undefined)).toEqual(['remove']);
	});
});

describe('copy badges', () => {
	it('lists quality, edition and protection once each', () => {
		expect(copyBadges(['4K', 'CC'], 'Directors Cut', 'Protected')).toEqual(['4K', 'CC', 'Directors Cut', 'Protected']);
	});

	it('drops an edition that repeats a quality badge or the protected word', () => {
		expect(copyBadges(['4K', 'CC'], 'CC', '')).toEqual(['4K', 'CC']);
		expect(copyBadges(['HD'], '4K', 'Protected')).toEqual(['HD', '4K', 'Protected']);
		expect(copyBadges([], 'Protected', 'Protected')).toEqual(['Protected']);
	});
});

describe('edition check', () => {
	it('accepts plain labels in any language', () => {
		expect(editionProblem('')).toBe(false);
		expect(editionProblem('Directors Cut')).toBe(false);
		expect(editionProblem('Расширенная версия')).toBe(false);
	});

	it('counts bytes like the server, not characters', () => {
		expect(editionProblem('Расширенная режиссёрская версия')).toBe(true);
		expect(editionProblem('a'.repeat(40))).toBe(false);
		expect(editionProblem('a'.repeat(41))).toBe(true);
	});

	it('refuses characters the server refuses', () => {
		for (const bad of ['{bad}', 'a/b', 'a\\b', 'Vol.', 'a[1]', 'x‎y']) expect(editionProblem(bad)).toBe(true);
	});
});

describe('edition refusal', () => {
	it('counts a long non-Latin name that is under 40 characters but over 40 bytes as a problem', () => {
		expect(editionProblem('Расширенная режиссёрская версия')).toBe(true);
	});

	it('blames the edition only when the server names it', () => {
		expect(editionRefused(400, 'invalid copy choice: edition needs a plain label of at most 40 characters')).toBe(true);
		expect(editionRefused(400, 'invalid copy choice: choose a current main copy')).toBe(false);
		expect(editionRefused(500, 'edition')).toBe(false);
	});
});

describe('device names', () => {
	it('spots the names Windows keeps for devices, in any case and with an extension', () => {
		for (const bad of ['CON', 'con', ' Nul ', 'PRN', 'aux', 'COM1', 'lpt9', 'com0', 'CON.txt', 'nul.final.cut']) expect(editionDevice(bad)).not.toBe('');
	});

	it('names only the part Windows keeps, not the extension', () => {
		expect(editionDevice('com1.txt')).toBe('com1');
		expect(editionDevice(' Nul ')).toBe('Nul');
		expect(editionDevice('CON')).toBe('CON');
	});

	it('leaves ordinary editions alone', () => {
		for (const ok of ['', 'Final Cut', 'Concert', 'COM10', 'COMA', 'LPTX', 'Console', 'Extended CON']) expect(editionDevice(ok)).toBe('');
	});
});

describe('byte order mark at the ends', () => {
	it('is trimmed away before the check and before sending, so the server never sees it', () => {
		expect('﻿Final Cut﻿ '.trim()).toBe('Final Cut');
		expect(editionProblem('﻿Final Cut﻿')).toBe(false);
		expect(editionProblem('Final﻿Cut')).toBe(true);
	});
});

describe('show saved', () => {
	it('puts the saved edition and protection on the right card without a reload', () => {
		const slots = [{ file: { ID: 4, Size: 1, Protected: false, MediaInfo: { resolution: '1080p' } as Record<string, unknown> | null } }, { file: { ID: 9, Size: 2, Protected: false, MediaInfo: null as Record<string, unknown> | null } }];
		showSaved(slots, { ID: 9, Size: 0, Protected: true, MediaInfo: { edition: 'Final Cut' } });
		expect(slots[1].file).toEqual({ ID: 9, Size: 2, Protected: true, MediaInfo: { edition: 'Final Cut' } });
		expect(slots[0].file).toEqual({ ID: 4, Size: 1, Protected: false, MediaInfo: { resolution: '1080p' } });
	});

	it('changes nothing when the answer names no copy on the page', () => {
		const slots = [{ file: { ID: 4, Protected: false, MediaInfo: null } }];
		showSaved(slots, { ID: 5, Protected: true, MediaInfo: null });
		showSaved(slots, null);
		expect(slots[0].file).toEqual({ ID: 4, Protected: false, MediaInfo: null });
	});
});

describe('save then reload', () => {
	it('does not report a saved change as failed when only the reload fails', async () => {
		let saved = false;
		await expect(saveThenReload(async () => { saved = true; }, async () => { throw new Error('network'); })).resolves.toBeUndefined();
		expect(saved).toBe(true);
	});

	it('still reports a failed save, and does not reload after it', async () => {
		let reloaded = false;
		await expect(saveThenReload(async () => { throw new Error('refused'); }, async () => { reloaded = true; })).rejects.toThrow('refused');
		expect(reloaded).toBe(false);
	});
});
