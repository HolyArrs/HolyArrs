import { readFileSync } from 'node:fs';
import { compile } from 'svelte/compiler';
import { describe, expect, it } from 'vitest';

// + Add from a hover card stayed on "Loading the defaults". The dialogs checked
// `picked !== title` after awaiting the saved defaults, but a $state variable holds a proxy
// of a plain object, so the check was always true and open() quit before `ready`.
describe('Add dialogs open from any title object (#525)', () => {
	for (const file of ['AddShowDialog.svelte', 'AddMovieDialog.svelte']) {
		const src = readFileSync(new URL(file, import.meta.url), 'utf8');
		const open = src.slice(src.indexOf('export async function open('), src.indexOf('\n\t}\n', src.indexOf('export async function open(')));

		it(`${file}: the browser build stores a proxy, so identity can never match`, () => {
			const js = compile(src, { filename: file, generate: 'client' }).js.code;
			expect(js).toMatch(/\$\.set\(picked, [sm], true\)/); // true = store a proxy
		});

		it(`${file}: open() guards a late answer with an open counter, not object identity`, () => {
			expect(open).toMatch(/const mine = \+\+opens;/);
			expect(open).toMatch(/if \(mine !== opens \|\| !(sheetOpen|addOpen)\) return;/);
			expect(open).not.toMatch(/picked !==/);
			expect(open.indexOf('++opens')).toBeLessThan(open.indexOf('await savedDefaults.load()'));
			expect(open.indexOf('ready = true')).toBeGreaterThan(open.indexOf('mine !== opens'));
		});
	}
});
