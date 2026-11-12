import { describe, expect, it } from 'vitest';
import { languageGroups, matchLanguages } from './language-list.ts';

describe('language list', () => {
	it('puts the ten most used first and the rest, sorted, under More', () => {
		const g = languageGroups();
		expect(g.top).toHaveLength(10);
		expect(g.top[0].code).toBe('en');
		expect(g.top.length + g.more.length).toBe(44);
		const names = g.more.map((l) => l.name);
		expect([...names].sort((a, b) => a.localeCompare(b))).toEqual(names);
	});
	it('finds a language by its own name, accents ignored', () => {
		expect(matchLanguages('por', 'en').map((l) => l.code)).toEqual(['pt-br', 'pt-pt']);
		expect(matchLanguages('espanol', 'en').map((l) => l.code)).toEqual(['es']);
	});
	it('finds a language by its English name or its name in the page language', () => {
		expect(matchLanguages('german', 'en').map((l) => l.code)).toContain('de');
		expect(matchLanguages('deutsch', 'en').map((l) => l.code)).toContain('de');
		expect(matchLanguages('allemand', 'fr').map((l) => l.code)).toContain('de');
	});
	it('finds by code and returns everything for an empty search', () => {
		expect(matchLanguages('zh-tw', 'en').map((l) => l.code)).toEqual(['zh-tw']);
		expect(matchLanguages('  ', 'en')).toHaveLength(44);
		expect(matchLanguages('qqqq', 'en')).toHaveLength(0);
	});
});
