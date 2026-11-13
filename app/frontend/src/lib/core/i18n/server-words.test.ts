import { afterEach, describe, expect, it } from 'vitest';
import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import en from './messages/en/server-lines.json';
import { lang } from './i18n.svelte.ts';
import { serverWords } from './server-words.ts';

const start = { value: lang.value, words: lang.words };
afterEach(() => {
	lang.value = start.value;
	lang.words = start.words;
});

function speak(words: Record<string, string>) {
	lang.value = 'fr';
	lang.words = { ...(en as Record<string, string>), ...words };
}

describe('serverWords', () => {
	it('knows every card line and badge the server sends', async () => {
		const it_ = Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('./messages/it/*.json', { eager: true })).map((f) => f.default));
		speak(it_);
		for (const [line, want] of [['New movie', 'Nuovo film'], ['New show', 'Nuova serie'], ['New episode Mon', 'Nuovo episodio · lun'], ['Coming Friday', 'Arriva venerdì'], ['New season', 'Nuova stagione']]) expect(serverWords(line)).toBe(want);
	});

	it('says the Import by hand folder refusal in the picked language', async () => {
		const de = Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('./messages/de/server-lines.json', { eager: true })).map((f) => f.default));
		speak(de);
		expect(serverWords("Import by hand reads only from the Library folders, the Downloaders' folders and the extra import folders in Settings > Library folders: /data/tv, /downloads")).toBe(
			'Der Import von Hand liest nur aus den Bibliotheksordnern, den Ordnern der Downloader und den zusätzlichen Import-Ordnern unter Einstellungen > Bibliotheksordner: /data/tv, /downloads'
		);
	});

	it('shows the Because you have row and the short download reasons in the picked language', async () => {
		const de = Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('./messages/de/server-lines.json', { eager: true })).map((f) => f.default));
		speak(de);
		expect(serverWords('Because you have Slow Horses')).toBe('Weil du Slow Horses hast');
		expect(serverWords('Deleted by hand')).toBe('Von Hand gelöscht');
		expect(serverWords('Marked bad')).toBe('Als fehlerhaft markiert');
	});

	it('shows a Release reason in the picked language', async () => {
		const de = Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('./messages/de/server-lines.json', { eager: true })).map((f) => f.default));
		speak(de);
		expect(serverWords('holds S01E01, not what was searched for')).toBe('enthält S01E01, nicht das Gesuchte');
		expect(serverWords('no Title in the library is called "Dune"')).toBe('kein Titel in der Bibliothek heißt "Dune"');
	});

	it('names the file a folder path change stopped at', async () => {
		const de = Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('./messages/de/server-lines.json', { eager: true })).map((f) => f.default));
		speak(de);
		expect(serverWords('Dune (2021)/Dune.mkv is not at the new path')).toBe('Dune (2021)/Dune.mkv liegt nicht am neuen Pfad');
		expect(serverWords('a.mkv is 10 bytes at the new path, but 11 bytes in the Library')).toBe('a.mkv hat am neuen Pfad 10 Bytes, in der Bibliothek aber 11 Bytes');
		// The new path is checked like a new folder: these three come back most often.
		for (const line of ['the folder does not exist', 'the path must be an absolute folder path, not a whole drive and not inside or around the config folder', 'that folder, or one inside or around it, is already a Library folder']) expect(serverWords(line)).not.toBe(line);
	});

	it('shows a known line in the picked language', () => {
		speak({ 'server.requests.waiting-for-approval': 'En attente de validation' });
		expect(serverWords('Waiting for approval')).toBe('En attente de validation');
	});

	it('fills a line with slots back in', () => {
		speak({ 'server.discover.s-new-episodes': 'S{season} · {fresh} nouveaux épisodes' });
		expect(serverWords('S5 · 3 new episodes')).toBe('S5 · 3 nouveaux épisodes');
	});

	it('passes slot values that are known lines through too', () => {
		speak({ 'server.requests.problem-fixed-by': '{label} : problème {kindlabel} réglé par {name}', 'server.requests.sound': 'Son' });
		expect(serverWords('Dune: Sound problem fixed by Ana')).toBe('Dune : problème Son réglé par Ana');
	});

	it('shows the newer server lines in the picked language', () => {
		const fr = Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('./messages/fr/*.json', { eager: true })).map((f) => f.default));
		speak(fr);
		expect(serverWords('TMDB could not be read: timeout')).toBe("TMDB n'a pas pu être lu : timeout");
		expect(serverWords('Ana unlinked the trusted proxy sign-in of Ben')).toBe('Ana a délié la connexion trusted proxy de Ben');
		expect(serverWords('Download 1a2b3c4d: completed')).toBe('Téléchargement 1a2b3c4d : completed');
		expect(serverWords('uses {series_title}, which has no HolyArrs word')).toBe("utilise {series_title}, qui n'a pas de mot dans HolyArrs");
		// The stored line starts with the person (a control's Event), and a removal names who removed it.
		expect(serverWords('Ana paused the stage downloads')).toBe('Ana a mis en pause la phase downloads');
		expect(serverWords('Download removed by Ana')).toBe('Téléchargement retiré par Ana');
		expect(serverWords('Download removed')).toBe('Téléchargement retiré');
	});

	it('shows a pick warning, a refused send and a system line in German', () => {
		const de = Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('./messages/de/server-lines.json', { eager: true })).map((f) => f.default));
		speak(de);
		expect(serverWords('sent although it failed: it was turned down for this episode: no Release passed')).toContain('trotzdem gesendet, obwohl es durchgefallen ist: es wurde für diese Folge abgelehnt: ');
		expect(serverWords('this Version already has a good enough copy; the new one will replace it')).toBe('Diese Variante hat schon eine gut genügende Kopie; die neue ersetzt sie');
		expect(serverWords('the Source has no such Release file')).toBe('die Quelle hat diese Release-Datei nicht');
		// A History row capitalises the stored line first.
		expect(serverWords('The Source has no such Release file')).toBe('Die Quelle hat diese Release-Datei nicht');
		expect(serverWords('The Downloader already has this torrent outside HolyArrs, and Show only HolyArrs items is on: move it to HolyArrs\'s category there, or turn that setting off')).toMatch(/^Der Downloader hat diesen Torrent schon außerhalb von HolyArrs/);
		expect(serverWords('Luis signed in')).toBe('Luis hat sich angemeldet');
	});

	it('has every line the server sends (scripts/server-words.mjs --check)', () => {
		const web = fileURLToPath(new URL('../../../../', import.meta.url));
		const file = fileURLToPath(new URL('./messages/en/server-lines.json', import.meta.url));
		const before = readFileSync(file, 'utf8');
		let out = '';
		try {
			out = execFileSync(process.execPath, ['scripts/server-words.mjs', '--check'], { cwd: web, encoding: 'utf8' });
		} catch (e) {
			out = String((e as { stdout?: string }).stdout ?? e);
		}
		expect(readFileSync(file, 'utf8')).toBe(before);
		expect(out).toContain('server words: every line is in the file');
	}, 60_000);

	it('leaves unknown text and English alone', () => {
		speak({});
		expect(serverWords('Something only this server says 42')).toBe('Something only this server says 42');
		expect(serverWords('')).toBe('');
		lang.value = 'en';
		expect(serverWords('Waiting for approval')).toBe('Waiting for approval');
	});
});
