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

	it('shows Home and Explore row titles, badges, dates and departments in the picked language', async () => {
		const fr = Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('./messages/fr/*.json', { eager: true })).map((f) => f.default));
		speak(fr);
		const nov11 = new Intl.DateTimeFormat('fr', { month: 'short', day: 'numeric' }).format(new Date(2026, 10, 11));
		for (const [line, want] of [
			['Popular drama', 'Drame populaire'],
			['Popular 1990s comedy', 'Comédie populaire des années 1990'],
			['1980s science fiction', 'Science-fiction des années 1980'],
			['Horror picks for you', 'Sélection Horreur pour vous'],
			['Trending this week everywhere', 'Tendances partout cette semaine'],
			['This week, mostly sci-fi', 'Cette semaine, surtout du science-fiction'],
			['Because you asked for Dune', 'Parce que vous avez demandé Dune'],
			['Added 3d ago', 'Ajouté il y a 3 j'],
			['Season 2', 'Saison 2'],
			['Out Nov 11', `Sortie le ${nov11}`],
			['S2E5 · Nov 11', `S2E5 · ${nov11}`],
			['Directing', 'Réalisation']
		])
			expect(serverWords(line), line).toBe(want);
		// A gap that does not hold what its name says leaves the line as it came.
		for (const line of ['Popular now and close to you', 'Out soon', 'Season two']) expect(serverWords(line)).toBe(line);
	});

	it('leaves English row titles and badges exactly as the server sent them', () => {
		for (const line of ['Popular drama', 'Out Nov 11', 'Added 3d ago', 'Season 2', 'Directing']) expect(serverWords(line)).toBe(line);
	});

	it('shows what a name turned into in the picked language', async () => {
		const de = Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('./messages/de/server-lines.json', { eager: true })).map((f) => f.default));
		speak(de);
		expect(serverWords("qbittorrent → not found. HolyArrs can't find this name. Use the IP instead.")).toBe('qbittorrent → nicht gefunden. HolyArrs findet diesen Namen nicht. Nimm stattdessen die IP.');
		expect(serverWords('qbittorrent → 172.18.0.5 → refused on port 8080. The name works, but nothing answers on that port. Check the port.')).toBe(
			'qbittorrent → 172.18.0.5 → abgelehnt auf Port 8080. Der Name funktioniert, aber auf diesem Port antwortet nichts. Prüfe den Port.'
		);
	});

	it('says why a save on an unknown host name is blocked, in the picked language', async () => {
		const de = Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('./messages/de/server-lines.json', { eager: true })).map((f) => f.default));
		speak(de);
		expect(serverWords("Saving is blocked because HolyArrs doesn't know the name media.example.com. Open HolyArrs at its local address and add this name under Settings > Sign-in and security > Allowed host names, or add it to HOLYARRS_ALLOWED_HOSTS.")).toBe(
			'Speichern ist blockiert, weil HolyArrs den Namen media.example.com nicht kennt. Öffne HolyArrs über seine lokale Adresse und füge den Namen unter Einstellungen > Anmeldung und Sicherheit > Erlaubte Hostnamen hinzu, oder trage ihn in HOLYARRS_ALLOWED_HOSTS ein.'
		);
	});

	it('says the Import by hand folder refusal in the picked language', async () => {
		const de = Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('./messages/de/server-lines.json', { eager: true })).map((f) => f.default));
		speak(de);
		expect(serverWords("Import by hand reads only from the Library folders, the Downloaders' folders and the extra import folders in Settings > Library folders: /data/tv, /downloads")).toBe(
			'Der Import von Hand liest nur aus den Bibliotheksordnern, den Ordnern der Downloader und den zusätzlichen Import-Ordnern unter Einstellungen > Bibliotheksordner: /data/tv, /downloads'
		);
	});

	it('says why a request whose title was deleted from the Library will not arrive, in the picked language', async () => {
		const de = Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('./messages/de/server-lines.json', { eager: true })).map((f) => f.default));
		speak(de);
		expect(serverWords('It was deleted from the Library after it was approved.')).toBe('Es wurde nach der Genehmigung aus der Bibliothek gelöscht.');
	});

	it('says why a file was not placed in the picked language', async () => {
		const de = Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('./messages/de/server-lines.json', { eager: true })).map((f) => f.default));
		speak(de);
		expect(serverWords('a file with the same name that HolyArrs did not place is already in the folder; move it out of the folder, then import again')).toBe(
			'Im Ordner liegt schon eine Datei mit demselben Namen, die HolyArrs nicht abgelegt hat; verschiebe sie aus dem Ordner und importiere erneut'
		);
		expect(serverWords('HolyArrs may not write to the folder this file goes to in the Library; give the user HolyArrs runs as (PUID and PGID) write access to it, then import again')).toMatch(/^HolyArrs darf nicht in den Ordner der Bibliothek schreiben/);
		expect(serverWords("HolyArrs can't find the downloaded file where the Downloader says it is; check the path pair in Settings > Downloaders and that the download folder is mounted, then import again")).toMatch(/^HolyArrs findet die heruntergeladene Datei nicht dort/);
		expect(serverWords('the file could not be placed: hardlink failed (link a b: x), copy failed (open c: y)')).toBe('die Datei konnte nicht abgelegt werden: Hardlink fehlgeschlagen (link a b: x), Kopieren fehlgeschlagen (open c: y)');
		expect(serverWords('hardlink not possible: link a b: x')).toBe('Hardlink nicht möglich: link a b: x');
		expect(serverWords('the file could not be placed: the source is not a plain file (a symlink or something else)')).toBe('die Datei konnte nicht abgelegt werden: die Quelle ist keine einfache Datei (ein Symlink oder etwas anderes)');
		expect(serverWords('the file could not be placed: a file appeared at the target')).toBe('die Datei konnte nicht abgelegt werden: am Ziel ist eine Datei aufgetaucht');
		expect(serverWords('the file for S01E01 could not be placed: the file is not in the download folder any more')).toBe('die Datei für S01E01 konnte nicht abgelegt werden: die Datei ist nicht mehr im Download-Ordner');
		expect(serverWords('the file for S01E01 could not be placed: could not read the file to check it')).toBe('die Datei für S01E01 konnte nicht abgelegt werden: die Datei konnte zum Prüfen nicht gelesen werden');
		for (const [en, de] of [
			['the current file is protected; turn its protection off first', 'die aktuelle Datei ist geschützt; schalte zuerst ihren Schutz aus'],
			['the new path would leave the Library folder', 'der neue Pfad würde den Bibliotheksordner verlassen'],
			['the new path would be too long (300 characters, the limit here is 260)', 'der neue Pfad wäre zu lang (300 Zeichen, die Grenze liegt hier bei 260)'],
			['this is the file it already has', 'das ist die Datei, die schon da ist'],
			['this already has a file; replace it or pick another Version', 'hier gibt es schon eine Datei; ersetze sie oder wähle eine andere Variante'],
			['already a file in the Library', 'schon eine Datei in der Bibliothek'],
			['the current file could not be moved to the holding folder: rename a b: x', 'die aktuelle Datei konnte nicht in den Ordner für Zurückgehaltenes verschoben werden: rename a b: x'],
			['something went wrong; see the log', 'etwas ist schiefgelaufen; siehe Log'],
		])
			expect(serverWords('the file could not be placed: ' + en)).toBe('die Datei konnte nicht abgelegt werden: ' + de);
		expect(serverWords('The file for S01E01 could not be placed: lstat /d/x.mkv: no such file or directory')).toBe('Die Datei für S01E01 konnte nicht abgelegt werden: lstat /d/x.mkv: no such file or directory');
		expect(serverWords('The file could not be placed: x')).toBe('Die Datei konnte nicht abgelegt werden: x');
		expect(serverWords('the file could not be placed: the file is not in the download folder any more')).toBe('die Datei konnte nicht abgelegt werden: die Datei ist nicht mehr im Download-Ordner');
		expect(serverWords('The file could not be placed: could not read the file to check it')).toBe('Die Datei konnte nicht abgelegt werden: die Datei konnte zum Prüfen nicht gelesen werden');
	});

	it('says the in-app update refusals and failures in the picked language', async () => {
		const de = Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('./messages/de/server-lines.json', { eager: true })).map((f) => f.default));
		speak(de);
		expect(serverWords('the download does not match the update list, so it was deleted')).toBe('der Download passt nicht zur Update-Liste und wurde deshalb gelöscht');
		expect(serverWords('still busy with 3 imports; try again when it finishes')).toBe('noch mit 3 Importen beschäftigt; versuche es erneut, wenn sie fertig sind');
		expect(serverWords('not enough free space in the config folder for the 42 MB download')).toBe('nicht genug freier Platz im Config-Ordner für den Download mit 42 MB');
		expect(serverWords('this version changed the database and no copy from before it is kept, so going back is not safe')).toBe('diese Version hat die Datenbank geändert und es gibt keine Kopie von davor, daher ist Zurückgehen nicht sicher');
	});

	it('shows the Because you have row and the short download reasons in the picked language', async () => {
		const de = Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('./messages/de/server-lines.json', { eager: true })).map((f) => f.default));
		speak(de);
		expect(serverWords('Because you have Slow Horses')).toBe('Weil du Slow Horses hast');
		expect(serverWords('Deleted by hand')).toBe('Von Hand gelöscht');
		expect(serverWords('Marked bad')).toBe('Als fehlerhaft markiert');
	});

	it('says a finished download removal, why its files stayed and the Downloader refusal in the picked language', async () => {
		const de = Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('./messages/de/server-lines.json', { eager: true })).map((f) => f.default));
		speak(de);
		expect(serverWords('Download removed; its files were kept: /data/media is a Library folder or is in or around one')).toBe(
			'Download entfernt; seine Dateien wurden behalten: /data/media ist ein Bibliotheksordner oder liegt in oder um einen'
		);
		expect(serverWords('Download removed by luis; its files were kept: another torrent in Q uses /data/x')).toBe(
			'luis hat den Download entfernt; seine Dateien wurden behalten: ein anderer Torrent in Q nutzt /data/x'
		);
		for (const why of ['/downloads is a whole save folder', 'Q did not say where they are', 'HolyArrs could not reach Q', 'HolyArrs could not read its Library folders', 'HolyArrs could not check whether another torrent uses them'])
			expect(serverWords(why)).not.toBe(why);
		expect(serverWords('Could not remove the download from Q: said no; HolyArrs tries again on the next sync')).toBe(
			'Download von Q ließ sich nicht entfernen: said no; HolyArrs versucht es beim nächsten Abgleich noch einmal'
		);
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

	it('says what a Jellyfin or Emby Test answered in the picked language', () => {
		const packs = import.meta.glob<{ default: Record<string, string> }>('./messages/{de,ar,ja}/server-lines.json', { eager: true });
		const lines = [
			'The server did not answer at this address.',
			'Something answered, but it does not look like Emby.',
			'Something answered, but it does not look like Jellyfin.',
			'that address did not answer like Emby; check it and try again',
			'the server did not answer',
			'the server is on the local network, which it may not reach',
			'reading the server answer failed',
			"the server's answer was too big to read",
			'the server turned down the token',
			'the server answered 500',
			'the token could not be read',
			"paste the server's token (Plex) or API key (Jellyfin, Emby)",
			'the address must be an http or https link with a host, like http://192.168.1.10:32400',
			'the emby server did not answer; ask the owner',
			'wrong name or password',
			'too many failed sign-ins; wait a little and try again',
			'this account is turned off on the emby server'
		];
		const said: Record<string, string[]> = {};
		for (const code of ['de', 'ar', 'ja']) {
			speak(packs[`./messages/${code}/server-lines.json`].default);
			said[code] = lines.map((line) => serverWords(line));
			said[code].forEach((got, i) => expect(got, `${code}: ${lines[i]}`).not.toBe(lines[i]));
		}
		expect(said.de.slice(0, 3)).toEqual(['Der Server hat unter dieser Adresse nicht geantwortet.', 'Etwas hat geantwortet, aber es sieht nicht nach Emby aus.', 'Etwas hat geantwortet, aber es sieht nicht nach Jellyfin aus.']);
		expect(said.de[9]).toBe('der Server hat mit 500 geantwortet');
		expect(said.ar[1]).toBe('ورد ردّ، لكنه لا يبدو أنه Emby.');
		expect(said.ar[9]).toContain('500');
		expect(said.ja[0]).toBe('このアドレスではサーバーが応答しませんでした。');
		expect(said.ja[1]).toBe('何かが応答しましたが、Emby ではないようです。');
		expect(said.ja[3]).toContain('Emby');
	});

	it('tells someone their account is turned off on the Jellyfin or Emby server', () => {
		const packs = import.meta.glob<{ default: Record<string, string> }>('./messages/{de,ar,ja}/server-lines.json', { eager: true });
		const line = (word: string) => `Your account is turned off on the ${word} server. Ask the person who runs it to turn it back on.`;
		speak(packs['./messages/de/server-lines.json'].default);
		expect(serverWords(line('Emby'))).toBe('Dein Konto ist auf dem Emby-Server deaktiviert. Bitte die Person, die den Server betreibt, es wieder zu aktivieren.');
		expect(serverWords(line('Jellyfin'))).toContain('Jellyfin-Server');
		speak(packs['./messages/ar/server-lines.json'].default);
		expect(serverWords(line('Emby'))).toBe('حسابك متوقف على خادم Emby. اطلب ممن يدير الخادم أن يعيد تفعيله.');
		speak(packs['./messages/ja/server-lines.json'].default);
		expect(serverWords(line('Jellyfin'))).toBe('あなたのアカウントは、Jellyfin サーバーでオフになっています。サーバーの管理者に、オンに戻すよう依頼してください。');
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
