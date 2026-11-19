import { readFileSync } from 'node:fs';
import { compile } from 'svelte/compiler';
// @ts-expect-error Svelte's generated client runtime has no public declaration file.
import * as client from 'svelte/internal/client';
import ts from 'typescript';
import { describe, expect, it } from 'vitest';
// Run the actual compiled setup and open() with Svelte's proxy runtime.
// Rendering is omitted: this regression is in the async initialization before
// the form becomes ready, not in the sheet's DOM or layout.
function dialog(kind: 'Movie' | 'Show', load: () => Promise<unknown>) {
	const file = `Add${kind}Dialog.svelte`;
	const code = compile(readFileSync(new URL(file, import.meta.url), 'utf8'), { filename: file, generate: 'client' }).js.code;
	const ast = ts.createSourceFile(file + '.js', code, ts.ScriptTarget.Latest, true, ts.ScriptKind.JS);
	const component = ast.statements.find((s): s is ts.FunctionDeclaration => ts.isFunctionDeclaration(s) && s.name?.text === `Add${kind}Dialog`)!;
	const open = component.body!.statements.find((s): s is ts.FunctionDeclaration => ts.isFunctionDeclaration(s) && s.name?.text === 'open')!;
	const setup = code.slice(component.body!.getStart(ast) + 1, open.end);
	const visible = kind === 'Movie' ? 'addOpen' : 'sheetOpen';
	const make = new Function('$', '$$props', 'savedDefaults', 'startChoices', 'loadChoices', 'gates', 'secondVersion', `${setup}
 $.pop();
 return {open, close:()=>$.set(${visible},false), read:()=>({ready:$.get(ready), folderId:$.get(folderId), name:$.get(picked)?.name})};`);
	return make(client, { folders: [], rules: [], defaults: null }, { load }, (d: {
		folderId: number;
	} | null) => ({ folderId: d?.folderId ?? 1, rulesId: 2, keep: true, searchOnAdd: false }), () => ({}), [], () => ({ label: '4K', pick_rules_id: 2, library_folder_id: 1 })) as {
		open: (title: {
			name: string;
		}) => Promise<void>;
		close: () => void;
		read: () => {
			ready: boolean;
			folderId: number;
			name: string;
		};
	};
}
function pending() {
	let resolve!: (value: {
		folderId: number;
	} | null) => void;
	const promise = new Promise<{
		folderId: number;
	} | null>((r) => { resolve = r; });
	return { promise, resolve };
}
for (const kind of ['Movie', 'Show'] as const)
	describe(`${kind} add dialog initialization`, () => {
		it('becomes ready after defaults load for a plain API response', async () => {
			const d = pending(), sheet = dialog(kind, () => d.promise);
			const opened = sheet.open({ name: 'Example' });
			expect(sheet.read().ready).toBe(false);
			d.resolve({ folderId: 7 });
			await opened;
			expect(sheet.read()).toEqual({ ready: true, folderId: 7, name: 'Example' });
		});
		it('also accepts an already reactive title', async () => {
			const sheet = dialog(kind, async () => ({ folderId: 7 }));
			await sheet.open(client.proxy({ name: 'Example' }));
			expect(sheet.read().ready).toBe(true);
		});
		it('uses fallback choices when the shared loader returns null after an error', async () => {
			const sheet = dialog(kind, async () => null);
			await sheet.open({ name: 'Example' });
			expect(sheet.read()).toEqual({ ready: true, folderId: 1, name: 'Example' });
		});
		for (const same of [false, true])
			it(`ignores an older defaults response (${same ? 'same' : 'different'} title)`, async () => {
				const a = pending(), b = pending();
				let calls = 0;
				const sheet = dialog(kind, () => calls++ === 0 ? a.promise : b.promise);
				const title = client.proxy({ name: 'First' });
				const first = sheet.open(title);
				const second = sheet.open(same ? title : { name: 'Second' });
				b.resolve({ folderId: 9 });
				await second;
				a.resolve({ folderId: 3 });
				await first;
				expect(sheet.read()).toEqual({ ready: true, folderId: 9, name: same ? 'First' : 'Second' });
			});
		it('does not initialize a closed dialog', async () => {
			const d = pending(), sheet = dialog(kind, () => d.promise);
			const opened = sheet.open(client.proxy({ name: 'Example' }));
			sheet.close();
			d.resolve({ folderId: 7 });
			await opened;
			expect(sheet.read().ready).toBe(false);
		});
	});
for (const kind of ['Movie', 'Show'] as const) {
	it(`${kind}: closing and reopening ignores the old response even for the same title`, async () => {
		const old = pending(), current = pending();
		let calls = 0;
		const sheet = dialog(kind, () => calls++ === 0 ? old.promise : current.promise);
		const title = client.proxy({ name: 'Example' });
		const first = sheet.open(title);
		sheet.close();
		const second = sheet.open(title);
		old.resolve({ folderId: 3 });
		await first;
		expect(sheet.read().ready).toBe(false);
		current.resolve({ folderId: 9 });
		await second;
		expect(sheet.read()).toEqual({ ready: true, folderId: 9, name: 'Example' });
	});
	it(`${kind}: concurrent opens can share the same defaults promise`, async () => {
		const d = pending(), sheet = dialog(kind, () => d.promise);
		const first = sheet.open({ name: 'First' }), second = sheet.open({ name: 'Second' });
		d.resolve({ folderId: 7 });
		await Promise.all([first, second]);
		expect(sheet.read()).toEqual({ ready: true, folderId: 7, name: 'Second' });
	});
}
