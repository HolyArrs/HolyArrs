import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const read = (p: string) => readFileSync(new URL(p, import.meta.url), 'utf8');
const page = read('../../routes/library/import/+page.svelte');
const byHand = read('../../routes/library/import-files/+page.svelte');
const library = read('../../routes/library/+page.svelte');

describe('Import a library while the parts are being sent', () => {
	it('asks before the tab closes or the person moves to another page, and says to keep it open', () => {
		expect(page).toMatch(/<svelte:window onbeforeunload=\{\(e\) => \{ if \(adopting\) e\.preventDefault\(\); \}\}[^\n]*\/>/);
		expect(page).toMatch(/beforeNavigate\(\(nav\) => \{[\s\S]*?confirm\(t\(importKey\('leaveAsk', flat\)\)\)[\s\S]*?nav\.cancel\(\)/);
		expect(page).toContain("t('library.import.keepOpen')");
	});
	it('notes an import cut short and says where it stopped on the way back', () => {
		expect(page).toContain("noteCut({ lf, total: picks.length, jobs: [], why: 'closed' })");
		expect(page).toContain('t(importKey(cut.key, cutFlat),');
	});
	it('holds the folder list and the Follow box still while adopting', () => {
		expect(page).toMatch(/class="checkbar"[^>]*inert=\{adopting\}/);
		expect(page).toMatch(/bind:checked=\{followAll\} disabled=\{adopting\}/);
	});
	it("moves the number inside a part from the job's own progress", () => {
		expect(page).toContain('throughNow(sent, scanCount(job?.ProgressNote))');
	});
	it('leads back to the folders that did not go through, and asks before ticks are lost', () => {
		expect(page).toContain('scanFromFirstLeft(order, results, scanFrom)');
		expect(page).toContain("t('library.import.scanAgain')");
		expect(page).toMatch(/function scanRest[\s\S]*?confirm\(/);
		// A music scan that stopped early keeps the warning about the ticks.
		expect(page).toMatch(/\{#if plan\.partial\}<p class="small"><Isolated text=\{t\('library\.import\.stopped'[^\n]*\{\/if\}\n\s*\{#if plan\.folders\.some\(canAdopt\)\}<p class="small">\{t\('library\.import\.partNext'\)\}<\/p>\{\/if\}/);
	});
});

describe('Paths inside a sentence keep their own direction', () => {
	it('isolates paths and folder names on both import pages', () => {
		expect(page).toContain('path: ltr(plan.path)');
		expect(page).toContain("next: ltr(plan.next ?? '')");
		expect(page).toContain('folder: ltr(lookFolder.folder)');
		expect(page).toContain('<bdi>{r.folder}</bdi>');
		expect(byHand).toContain('path: ltr(scan.path)');
		expect(byHand).toContain('name: ltr(f.name)');
	});
});

describe('Counts on the Library pages', () => {
	it('writes counts with the separators of the language', () => {
		expect(library).toContain('{ n: shown.length.toLocaleString(appLocale()) }');
		expect(library).toContain('total: kindTotal.toLocaleString(appLocale())');
		expect(byHand).toContain('{ n: files.length.toLocaleString(appLocale()), path:');
	});
});

const layout = read('../../routes/+layout.svelte');
const login = read('../../routes/login/+page.svelte');
const toolbar = read('./ImportToolbar.svelte');

describe('Leaving an import, or not', () => {
	it('clears the page-change veil when a page change is cancelled', () => {
		expect(layout).toMatch(/beforeNavigate\(\(nav\) => \{\s*if \(!nav\.willUnload\) navWatch\(nav\.complete,/);
	});
	it('notes why an import stopped: left the page or signed out', () => {
		expect(page).toContain("noteWhy('left')");
		expect(page).toContain("noteWhy('signedOut')");
		expect(page).toContain('cutFromJobs(');
	});
	it('says on the sign-in page that an import paused when the sign-in ended', () => {
		expect(page).toContain("sessionStorage.setItem('holyarrs-import-paused', '1')");
		expect(login).toContain("t('accounts.login.importPaused')");
	});
	it('says HolyArrs is not answering while a part waits for an answer', () => {
		expect(page).toContain("t('library.import.stillTrying')");
	});
});

describe('A long scan', () => {
	it('is picked up again on the way back and after the long-time line', () => {
		expect(page).toContain('holyarrs.import.scan.');
		expect(page).toMatch(/onMount\([\s\S]*?watchScan\(/);
	});
	it('names folders the metadata server did not answer for, and how many', () => {
		expect(page).toContain("t(fail.all ? 'library.import.lookFailAll' : 'library.import.lookFailSome'");
	});
});

describe('Import files by hand', () => {
	it('shows only the result on a row that was imported', () => {
		expect(byHand).toMatch(/\{#if done\?\.outcome !== 'placed'\}\{#if why\}/);
	});
	it('says where the files go before the import', () => {
		expect(byHand).toContain("tr(mode === 'move' ? 'library.byHand.wherePlacedMove' : 'library.byHand.wherePlaced')");
	});
});

describe('The filter row on a phone', () => {
	it('wraps instead of cutting off a long label', () => {
		expect(toolbar).toMatch(/\.phone \.chips \{[^}]*flex-wrap: wrap/);
	});
});

describe('The note of an import cut short by the sign-in ending', () => {
	it('is kept under the person who ran it, not under no one once signed out', () => {
		expect(page).toMatch(/let markKey = '';/);
		expect(page).toMatch(/markKey = cutKey;\s*const free = holdWhile\(navigator\.locks, markKey\);\s*noteCut\(\{ lf, total: picks\.length/);
		expect(page).toContain('localStorage.setItem(markKey || cutKey, JSON.stringify({ ...v, at:');
	});
});

describe('Paths in right-to-left sentences', () => {
	it('render the isolated path in its own left-to-right element', () => {
		const iso = read('../ui/Isolated.svelte');
		expect(iso).toContain('<bdi dir="ltr">');
		// The marks are written as escapes, never as letters nobody can see.
		expect(iso).toContain('String.fromCharCode(0x2066, 0x2069)');
		expect(iso).not.toMatch(new RegExp('[' + String.fromCharCode(0x2066, 0x2069) + ']'));
		expect(page).toContain("<Isolated text={t(importKey('foldersPart'");
		expect(page).toContain("<Isolated text={t(importKey(plan.folders.length === 1 ? 'foldersInOne'");
		expect(page).toContain("<Isolated text={t('library.import.stopped'");
		expect(page).toContain("<Isolated text={t('library.import.lookUpFor'");
		expect(byHand).toContain("<Isolated text={tr(files.length === 1 ? 'library.byHand.filesInOne'");
		expect(byHand).toContain("<Isolated text={tr('library.byHand.lookFor'");
		expect(read('../ui/Sheet.svelte')).toContain('<Isolated text={title} />');
	});
});

describe('The scan the page remembers', () => {
	it('is forgotten once its plan arrives or it fails, so the next visit opens on step 1, and kept when HolyArrs did not answer', () => {
		expect(page).toMatch(/plan = p;\n\t\t\tnoteScan\(null\);/);
		expect(page).toMatch(/const left = scanEnd\(e, back, t\('core\.api\.notAnswering'\)\);\n\t\t\terror = left\.error;\n\t\t\tif \(!gone && !left\.keep\) noteScan\(null\);/);
		expect(page).not.toContain('e instanceof ApiError && e.status === 404');
	});
});

describe('An import in two tabs, and a sign-in that ends in another tab', () => {
	it('keeps one record per tab and clears only its own', () => {
		expect(page).toContain('const cutKey = $derived(cutKeyOf(cutBase, tab));');
		expect(page).toContain('findCut(store, cutBase, now, held, ids)');
	});
	it('keeps its note fresh while it sends, and marks it stopped when it stops or the tab goes', () => {
		expect(page).toContain('at: live ? Date.now() : 0');
		expect(page).toMatch(/if \(!adopting\) return;\s*\/\/[^\n]*\n\s*const iv = setInterval\(\(\) => track\?\.why === 'closed' && noteCut\(track\), 5000\);/);
		expect(page).toMatch(/if \(track\) noteCut\(\{ \.\.\.track, why \}, false\);/);
		expect(page).toContain('onpagehide={() => { if (adopting && track) noteCut(track, false); }}');
	});
	it('names the ended sign-in as the cause when the page goes away with it', () => {
		expect(page).toMatch(/onDestroy\(\(\) => \{\n\t\tgone = true;\n[^\n]*\n\t\tif \(adopting && session\.s\.state !== 'signed_in'\) signedOut\(\);/);
	});
});

describe('Scan the rest on a plan where no folder could be looked up', () => {
	it('is not offered: scanning more folders would only fail the same way', () => {
		expect(page).toContain('{#if plan.more && (plan.partial || !fail?.every)}');
	});
	it('stays when folders already in the Library were read without a lookup', () => {
		expect(page).not.toContain('!fail?.all');
		const en = JSON.parse(read('../core/i18n/messages/en/library.json'));
		expect(en['library.import.lookFailAll']).toContain('none of the folders that needed a lookup');
	});
});

describe('Select all on a plan', () => {
	it('is shown only when some folder shown can be ticked', () => {
		expect(page).toMatch(/\{#if tickNow\.can > 0\}<Button variant="plain" size="md" onclick=\{tickShown\} data-testid="import-select-all">/);
		expect(page).not.toContain('disabled={!tickNow.can}');
	});
});

describe('A note that already says why the import stopped', () => {
	it('stays stopped when a part is taken after the person left or was signed out', () => {
		expect(page).toContain("if (track) noteCut({ ...track, jobs: [...track.jobs, r.job_id] }, track.why === 'closed');");
	});
});

describe('An import in a tab hidden for a long time', () => {
	it('holds a lock while it sends, and other tabs ask for it before saying it stopped', () => {
		expect(page).toMatch(/const free = holdWhile\(navigator\.locks, markKey\);/);
		expect(page).toMatch(/finally \{\n\t\t\tfree\(\);/);
		expect(page).toContain('findCut(store, cutBase, now, held, ids)');
	});
	it('clears older notes of the same folder once an import of it finished, and drops a note the server cannot answer for', () => {
		expect(page).toMatch(/const \{ store, held \} = await notesThenLocks\(localStorage, cutBase, navigator\.locks\);\n\t\t\t\tfor \(const k of staleCuts\(store, cutBase, lf, Date\.now\(\), held\)\) localStorage\.removeItem\(k\);/);
		expect(page).toContain('cutFromJobs(was, jobs)');
	});
	it('drops a note only when the server says it forgot a part, and keeps it untouched when the server did not answer', () => {
		expect(page).toContain('.catch((e) => (jobForgotten(e) ? null : undefined))');
		expect(page).not.toContain('e.status === 404 ? null : undefined');
		expect(page).toMatch(/if \(note === undefined\) wasKey = '';\n\t\t\telse if \(!note\) noteCut\(null\);/);
		expect(page).not.toContain('.catch(() => null)));');
	});
	it('takes the lock before the note says it is running', () => {
		expect(page).toMatch(/markKey = cutKey;\n\t\tconst free = holdWhile\(navigator\.locks, markKey\);\n\t\tnoteCut\(\{ lf, total: picks\.length, jobs: \[\], why: 'closed' \}\);/);
	});
});

describe('The scan record of each tab', () => {
	it('is kept per tab, so an import in one tab never clears a scan running in another', () => {
		expect(page).toContain('const scanKey = $derived(cutKeyOf(scanBase, tab));');
		expect(page).toMatch(/findKept<[^>]*>\(localStorage, scanBase,/);
	});
	it("is taken as this tab's own only when this tab follows that scan, so a tab showing an older note never clears another tab's scan", () => {
		expect(page).not.toContain('[sc, scanWas] = [w.v, w.key]');
		expect(page).toMatch(/if \(sc && !cut && folders\.some\(\(f\) => f\.id === sc\?\.lf\)\) \{\n\t\t\tscanWas = scKey;/);
	});
});

describe('Folders the metadata server did not answer for, in Russian and Ukrainian', () => {
	it('read as not checked, never as not found', () => {
		for (const [lang, bad] of [['ru', 'найти'], ['uk', 'знайти']] as const) {
			const lib = JSON.parse(read(`../core/i18n/messages/${lang}/library.json`));
			const mf = JSON.parse(read(`../core/i18n/messages/${lang}/mediafiles.json`));
			expect(mf['mediafiles.adopt.not_looked_up']).not.toBe(mf['mediafiles.adopt.unmatched']);
			expect(mf['mediafiles.adopt.not_looked_up'].toLowerCase()).not.toContain(lang === 'ru' ? 'найден' : 'знайден');
			expect(lib['library.import.lookFailAll']).not.toContain(bad);
			expect(lib['library.import.lookFailSome']).not.toContain(bad);
			const srv = JSON.parse(read(`../core/i18n/messages/${lang}/server-lines.json`));
			expect(srv['server.library.the-server-holyarrs-gets-details-from']).not.toContain(bad);
			expect(srv['server.library.the-server-holyarrs-gets-details-from']).toContain(lang === 'ru' ? 'проверить' : 'перевірити');
		}
	});
	it('read as not looked up in the other Slavic languages too, never as not found', () => {
		for (const [lang, found] of [['pl', 'znalez'], ['cs', 'nalez'], ['sk', 'nájd'], ['bg', 'намер'], ['sr', 'пронађ'], ['hr', 'pronađ']] as const) {
			const lib = JSON.parse(read(`../core/i18n/messages/${lang}/library.json`));
			const mf = JSON.parse(read(`../core/i18n/messages/${lang}/mediafiles.json`));
			for (const s of [mf['mediafiles.adopt.not_looked_up'], lib['library.import.lookFailAll'], lib['library.import.lookFailSome']]) expect(s.toLowerCase()).not.toContain(found);
		}
	});
});

describe('A proxy in front of HolyArrs that answers 404 while HolyArrs is down', () => {
	it('never drops a stopped note: the parts are asked about only when the Library folders loaded in the same visit', () => {
		expect(page).toContain('const read = await foldersThenNotes(async () => (await loadFolders(), !foldersFailed), () => localStorage, cutBase, navigator.locks);');
		expect(page).toMatch(/\n\t\tif \(read\) \{\n\t\t\tconst \{ store, held, now \} = read;/);
		expect(page).toMatch(/\n\t\tif \(was && !foldersFailed\) \{\n[^\n]*\n\t\t\tconst jobs = await Promise\.all/);
		expect(page).not.toMatch(/\n\t\tif \(was\) \{/);
	});
});

describe('Scan records once the Library folders loaded', () => {
	it('clears the ones of a folder that is gone, and passes them over when picking one up', () => {
		expect(page).toMatch(/if \(!foldersFailed\) \{\n\t\t\ttry \{\n\t\t\t\tconst ids = folders\.map\(\(f\) => f\.id\);\n[^\n]*\n\t\t\t\tfor \(const k of scansGone\(localStorage, scanBase, ids\)\) localStorage\.removeItem\(k\);/);
		expect(page).toContain("(x) => typeof x.job === 'number' && ids.includes(x.lf)");
	});
});

describe('Stopped notes once the Library folders loaded', () => {
	it("clears the ones of a folder that is gone, and passes them over, so they never hide another folder's note", () => {
		expect(page).toMatch(/const ids = folders\.map\(\(f\) => f\.id\);\n\t\t\ttry \{\n[^\n]*\n\t\t\t\tfor \(const k of cutsGone\(store, cutBase, ids, now, held\)\) localStorage\.removeItem\(k\);\n\t\t\t\tconst v = findCut\(store, cutBase, now, held, ids\);/);
	});
});

describe('Notes and locks of other tabs', () => {
	it('reads the notes before asking for the locks, both after the Library folders loaded, and compares them with the time they were read', () => {
		const i = page.indexOf('const read = await foldersThenNotes(');
		expect(i).toBeGreaterThan(-1);
		expect(i).toBeLessThan(page.indexOf('const v = findCut(store, cutBase, now, held, ids);'));
		expect(page).not.toContain('findCut(store, cutBase, Date.now()');
		expect(page).not.toContain('cutsGone(store, cutBase, ids, Date.now()');
		expect(page).not.toContain('await heldLocks(');
	});
});

describe('The first Scan request through a proxy page while HolyArrs is down', () => {
	it("shows the plain line, never the proxy's text, and never a scan HolyArrs forgot as gone", () => {
		expect(page).toMatch(/await watchScan\(r\.job_id\);\n\t\t\} catch \(e\) \{\n\t\t\terror = scanEnd\(e, false, t\('core\.api\.notAnswering'\)\)\.error;\n\t\t\tscanning = false;/);
	});
});

describe('A scan that is being watched', () => {
	it('asks again through a moment without an answer', () => {
		expect(page).toContain('end = await askThrough(() => follow(id), () => gone);');
	});
});

describe('A part whose Follow step failed after its folders went in', () => {
	it('keeps its results and still shows what went wrong as the page error', () => {
		expect(page).toMatch(/const \{ res, fail \} = partEnd\(end, \(m\) => serverWords\(m\) \|\| t\('library\.import\.adoptUnfinished'\)\);\n\t\t\t\t\tif \(fail && !partFail\) partFail = fail;\n\t\t\t\t\treturn res;/);
		expect(page).not.toContain("if (end.State !== 'done' && !res) throw");
		expect(page).toMatch(/results = \[\.\.\.all\][^\n]*\n\t\t\terror = partFail;/);
	});
});

describe('The Ukrainian import wording', () => {
	it('says тека for folder, as the rest of the app does', () => {
		const uk = JSON.parse(read('../core/i18n/messages/uk/library.json'));
		expect(uk['library.import.foldersPart'].startsWith('Тек у {path}: {total}.')).toBe(true);
		const keys = ['foldersPart', 'leftHint', 'leaveAsk', 'interrupted', 'interruptedLeft', 'interruptedSignedOut', 'askScanRestOne', 'askScanRestMany', 'lookFailSome', 'lookFailAll'].map((k) => `library.import.${k}`);
		for (const k of [...keys, 'library.byHand.wherePlaced', 'library.byHand.wherePlacedMove']) expect(uk[k].toLowerCase(), k).not.toMatch(/папк|папок/);
		expect(uk['library.byHand.wherePlaced']).toContain('у теку його назви в бібліотеці');
	});
	it('says тека in the server lines the import shows too', () => {
		const srv = JSON.parse(read('../core/i18n/messages/uk/server-lines.json'));
		const keys = ['is-already-in-the-library-in', 'the-server-holyarrs-gets-details-from', 'the-server-holyarrs-gets-details-from-2'].map((k) => `server.library.${k}`);
		for (const k of keys) expect(srv[k].toLowerCase(), k).not.toMatch(/папк|папок/);
		expect(srv['server.library.is-already-in-the-library-in']).toContain('в іншій теці');
		expect(srv['server.library.the-server-holyarrs-gets-details-from']).toContain('деякі теки не вдалося перевірити');
		expect(srv['server.library.the-server-holyarrs-gets-details-from-2']).toContain('щоб прийняти цю теку');
	});
});
