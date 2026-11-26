// The lines a folder kept to tagged Sources puts on a Slot and a search are shown in the picked language.
import { lang } from './i18n.svelte.ts';
import { serverWords } from './server-words.ts';
import de from './messages/de/server-lines.json';
import en from './messages/en/server-lines.json';
import ja from './messages/ja/server-lines.json';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

// English words a German line must not keep (Source names and tags aside).
const english = /\b(only|has|have|feed|search|turned|off|paused|its|tags|are|not|on|this|title|in|the|Pick rules|takes|no Source|nothing|asked|can|be|right now)\b/;

describe('a folder kept to tagged Sources, in German', () => {
	const was = { value: lang.value, words: lang.words };
	beforeEach(() => {
		lang.value = 'de';
		lang.words = { ...(en as Record<string, string>), ...(de as Record<string, string>) };
	});
	afterEach(() => {
		lang.value = was.value;
		lang.words = was.words;
	});
	it('says why nobody was asked, each Source with its own cause', () => {
		const line = serverWords('This folder only takes Sources tagged anime, subs. None can be searched right now (Nyaa only has a feed; Tosho is paused; Subs has search turned off), so nothing was asked.');
		for (const kept of ['anime, subs', 'Nyaa', 'Tosho', 'Subs', 'Ordner']) expect(line).toContain(kept);
		expect(line.replace(/Nyaa|Tosho|Subs|anime, subs/g, '')).not.toMatch(english);
	});
	it('says a Source a person paused in the words of its card, apart from one that paused by itself', () => {
		const line = serverWords('This folder only takes Sources tagged anime. None can be searched right now (Nyaa is paused by you), so nothing was asked.');
		expect(line).toContain('Nyaa wurde von dir pausiert');
		expect(serverWords('This folder only takes Sources tagged anime. None can be searched right now (Nyaa is paused), so nothing was asked.')).toContain('(Nyaa ist pausiert)');
		expect(line.replace(/Nyaa|anime/g, ''), line).not.toMatch(english);
	});
	it('says a held download was let go', () => {
		expect(serverWords("not sent: its Source is not one this folder's tags allow")).not.toMatch(english);
	});
	it('says no Source takes the tags, every part of it', () => {
		const line = serverWords("no Source takes this title's tags: Nyaa: its tags (kids) are not on this title; Tosho: its tags (kids, subs) are not in the Pick rules' tags (hd)");
		for (const kept of ['Nyaa', 'Tosho', '(kids)', '(kids, subs)', '(hd)']) expect(line).toContain(kept);
		expect(line.replace(/Nyaa|Tosho|kids|subs|hd/g, '')).not.toMatch(english);
	});
	it('keeps a stored line with "; " in its own words whole when it fills a gap', () => {
		expect(serverWords('Download failed: the metadata server did not answer; try again shortly')).toContain('versuche es gleich noch einmal');
		expect(serverWords('Download failed: the import does not bring tags over; check it before you resume it')).toContain('bevor du es fortsetzt');
		expect(serverWords('Download failed: you left them out; their asks come over as yours')).toContain('du hast sie ausgelassen; ihre Anfragen kommen als deine her');
	});
	it('says every Downloader that cannot take a download', () => {
		const line = serverWords('Download failed: no torrent Downloader can take it now: qBit is paused; Deluge is paused');
		for (const kept of ['qBit', 'Deluge', 'torrent']) expect(line).toContain(kept);
		expect(line.replace(/qBit|Deluge|torrent|Downloader|Download/g, ''), line).not.toMatch(english);
	});
	it('says an untagged Source is left out by Pick rules with tags', () => {
		const line = serverWords("no Source takes this title's tags: General: it has no tags, and the Pick rules only take Sources tagged hd");
		expect(line).toContain('General');
		expect(line.replace(/General|hd/g, ''), line).not.toMatch(english);
	});
	it('says a Downloader is resting', () => {
		const line = serverWords('Download failed: no torrent Downloader can take it now: qBit is resting until 00:33 UTC after failing; Deluge is paused');
		for (const kept of ['qBit', 'Deluge', '00:33 UTC']) expect(line).toContain(kept);
		expect(line.replace(/qBit|Deluge|torrent|Downloader|Download|UTC/g, ''), line).not.toMatch(/\b(resting|until|after|failing|is|paused)\b/);
	});
	it('keeps a Source named with a colon and still says its cause', () => {
		const line = serverWords("no Source takes this title's tags: Nyaa: Anime: its tags (kids) are not on this title");
		expect(line).toContain('Nyaa: Anime');
		expect(line.replace(/Nyaa: Anime|kids/g, ''), line).not.toMatch(english);
	});
	it('names several tags in the plural', () => {
		expect(serverWords('This folder only takes Sources tagged anime, subs. None can be searched right now (Nyaa is paused), so nothing was asked.')).toContain('mit den Tags anime, subs');
		expect(serverWords("no Source takes this title's tags: General: it has no tags, and the Pick rules only take Sources tagged hd, uhd")).toContain('mit den Tags hd, uhd');
	});
});

describe('names a person gave, in Japanese', () => {
	const was = { value: lang.value, words: lang.words };
	beforeEach(() => {
		lang.value = 'ja';
		lang.words = { ...(en as Record<string, string>), ...(ja as Record<string, string>) };
	});
	afterEach(() => {
		lang.value = was.value;
		lang.words = was.words;
	});
	it('stay as they were named, also when the name is a word the app knows', () => {
		const line = serverWords('no torrent Downloader can take it now: Movies is paused; Music is resting until 00:33 UTC after failing');
		for (const kept of ['Movies', 'Music', '00:33 UTC']) expect(line, line).toContain(kept);
		expect(line, line).not.toMatch(/is paused|resting|can take it/);
	});
});
