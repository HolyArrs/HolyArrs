import { describe, expect, it } from 'vitest';
import { readFile, readdir } from 'node:fs/promises';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

// The 1.1.0 wording pass changed these English lines; every other language has to say
// the new thing, not the old one. The checks below hold in any language: the copy and
// link notes name the Library, the friends' count uses the Requests word, the Tailscale
// note drops the word "tailnet", and no line is left in English.
const messages = join(fileURLToPath(new URL('.', import.meta.url)), 'messages');
type Words = Record<string, string>;
async function readWords(dir: string): Promise<Words> {
	const names = (await readdir(dir)).filter((f) => f.endsWith('.json'));
	return Object.assign({}, ...(await Promise.all(names.map(async (f) => JSON.parse(await readFile(join(dir, f), 'utf8')) as Words))));
}
const langs = new Map(await Promise.all((await readdir(messages)).map(async (l) => [l, await readWords(join(messages, l))] as const)));
const en = langs.get('en') ?? {};
const others = [...langs].filter(([l]) => l !== 'en');

const reworded = [
	'discover.hidden.pickPerson',
	'downloads.import.copy',
	'health.check.clearWaiting',
	'health.kind.clearWaiting',
	'mediafiles.byHand.copied',
	'mediafiles.byHand.linked',
	'settings.downloaders.spacing',
	'settings.downloaders.seedRemoveHint',
	'settings.downloaders.seedRemoveHintGoal',
	'settings.downloaders.seedRatioHint',
	'settings.downloaders.seedHours',
	'settings.downloaders.deadMinutes',
	'settings.requests.plexFriendsHint',
	'settings.requests.plexFriendsRead',
	'settings.requests.plexFriendsNotYet',
	'settings.requests.plexFriendsAsked',
	'settings.signIn.whoHint',
	'settings.signIn.fixBridge',
	'settings.signIn.cgnat',
	'settings.signIn.cgnatHint',
	'settings.tmdb.ownNote',
	'library.stats.noMatch'
];

// What de, fr, es and ar said before the English was reworded.
const before: Record<string, Words> = {
	de: {
		'settings.downloaders.spacing': 'Sekunden zwischen Sendungen an einen Downloader',
		'settings.downloaders.seedRemoveHint': 'Nur Downloads, die HolyArrs geschickt und schon importiert hat. Fertig, wenn {app} das Seeden stoppt.',
		'settings.downloaders.seedRatioHint': 'Leer: auf {app} warten. Eine unbekannte Ratio zählt nie.',
		'settings.downloaders.deadMinutes': 'Hängt bei der Dateisuche seit (Minuten)',
		'settings.requests.plexFriendsRead': 'Gelesen {when}',
		'settings.signIn.whoHint': 'Wähle das Kind-Profil, wenn Kinder dieses Netz nutzen. Wird die Person entfernt, melden sich wieder alle an.',
		'settings.tmdb.ownNote': 'Mit eigenen Schlüsseln fragst du jede Quelle selbst. Musik-Charts, neue Musik-Reihen, Streaming-Top-10, Wikipedia-Fakten und Anime-Zuordnungen kommen nur vom HolyArrs-Server und sind daher aus.',
		'library.stats.noMatch': 'Zuletzt gelesen {ago}. Nichts vom Gesehenen passt bisher zu deiner Bibliothek.'
	},
	fr: {
		'health.check.clearWaiting': 'Téléchargements en attente de nettoyage',
		'settings.downloaders.seedRemoveHint': 'Seulement les téléchargements envoyés par HolyArrs et déjà importés. Terminé quand {app} arrête de partager.',
		'settings.requests.plexFriendsRead': 'Lu {when}',
		'settings.signIn.cgnat': 'Aussi ma plage Tailscale'
	},
	es: {
		'health.check.clearWaiting': 'Descargas esperando la limpieza',
		'settings.downloaders.seedRatioHint': 'Vacío: esperar a {app}. Un ratio desconocido nunca cuenta.',
		'settings.signIn.fixBridge': 'El bridge de Docker. Entraría cualquiera'
	},
	ar: {
		'health.check.clearWaiting': 'تنزيلات بانتظار التنظيف',
		'settings.downloaders.seedRemoveHint': 'فقط التنزيلات التي أرسلها HolyArrs واستوردها. ينتهي عندما يوقف {app} المشاركة.',
		'settings.requests.plexFriendsRead': 'قُرئت {when}',
		'settings.signIn.cgnat': 'ونطاق Tailscale الخاص بي'
	}
};

// A short stem copes with case endings: Biblioteka / Biblioteką, Médiatár / Médiatárba.
const stem = (w: string) => [...w.toLocaleLowerCase()].slice(0, 4).join('');

describe('lines reworded in English are reworded in every language', () => {
	it('every reworded line is still an English key', () => {
		for (const k of reworded) expect(en[k], k).toBeTypeOf('string');
	});

	it('no language still says what it said before', () => {
		const stale: string[] = [];
		for (const [l, old] of Object.entries(before)) for (const [k, line] of Object.entries(old)) if (langs.get(l)?.[k] === line) stale.push(`${l}: ${k}`);
		expect(stale).toEqual([]);
	});

	it('no reworded line is left in English', () => {
		const english: string[] = [];
		for (const [l, words] of others) for (const k of reworded) if (words[k] === en[k]) english.push(`${l}: ${k}`);
		expect(english).toEqual([]);
	});

	it('the copy and link notes name the Library', () => {
		const missing: string[] = [];
		for (const [l, words] of others) {
			const library = stem(words['downloads.bar.library']);
			for (const k of ['downloads.import.copy', 'mediafiles.byHand.copied', 'mediafiles.byHand.linked']) if (!words[k].toLocaleLowerCase().includes(library)) missing.push(`${l}: ${k}`);
		}
		expect(missing).toEqual([]);
	});

	it("the friends' count uses the Requests word", () => {
		const wrong: string[] = [];
		for (const [l, words] of others) if (!words['settings.requests.plexFriendsAsked'].toLocaleLowerCase().includes(stem(words['frame.nav.requests']))) wrong.push(l);
		expect(wrong).toEqual([]);
	});

	it('the Tailscale note says Tailscale network, not tailnet', () => {
		const wrong: string[] = [];
		for (const [l, words] of others) {
			const line = words['settings.signIn.cgnatHint'];
			if (/tailnet/i.test(line) || !line.includes('Tailscale')) wrong.push(l);
		}
		expect(wrong).toEqual([]);
	});
});
