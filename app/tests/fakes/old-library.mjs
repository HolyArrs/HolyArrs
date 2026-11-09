import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';

/**
 * makeOldLibrary writes a small movie library as someone might already
 * have it, for the import tests: a folder named "Name (Year)" (matched), one
 * with a tmdb tag (matched), one with half a name (unsure), home videos
 * (unmatched) and a folder with no video. The videos are tiny stand-ins.
 * @param {string} dir
 */
export function makeOldLibrary(dir) {
	/** @type {[string, string][]} */
	const files = [
		['Marble Comet (2014)', 'Marble.Comet.2014.1080p.WEB-DL.H264-HOLYARRS.mkv'],
		['Willow Signal {tmdb-900013}', 'willow signal.mkv'],
		['Cinder (2012)', 'Cinder.2012.720p.BluRay.x264-OLD.mkv'],
		['Home Videos', 'beach day.mp4'],
		['Notes', 'readme.txt']
	];
	for (const [folder, name] of files) {
		mkdirSync(join(dir, folder), { recursive: true });
		writeFileSync(join(dir, folder, name), 'stand-in video '.repeat(64));
	}
}
