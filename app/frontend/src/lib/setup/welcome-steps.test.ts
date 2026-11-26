import { describe, expect, it } from 'vitest';
import { PHASES, STEPS, firstOpen, phasesFor, nextStep, phaseOf, plexAddress, plexChoiceLabel, plexFields, plexFormUrl, plexMayFallBack, plexShown, plexTries, defaultPlexPick, prevStep, resumeStep, serverKindOf, stepFromName, stepsFor } from './welcome-steps.ts';

const fresh = { withServer: true, serverDone: false, foldersDone: false };

describe('setup step bar', () => {
	it('has the agreed stops in a fixed order', () => {
		expect(PHASES).toEqual(['language', 'welcome', 'signin', 'server', 'parts', 'folders', 'sources', 'downloads', 'subtitles', 'access', 'done']);
		expect(STEPS.map(phaseOf)).toEqual(PHASES.slice(3));
	});
	it('shows only the steps this person gets', () => {
		expect(serverKindOf('plex')).toBe('plex');
		expect(serverKindOf('jellyfin')).toBe('jellyfin');
		expect(serverKindOf('local_setup')).toBeNull();
		expect(stepsFor({ withServer: false })).toEqual(['parts', 'folders', 'sources', 'downloads', 'subtitles', 'access', 'done']);
		expect(stepsFor({ withServer: true, subtitles: false })).toEqual(['server', 'parts', 'folders', 'sources', 'downloads', 'access', 'done']);
		expect(stepsFor({ withServer: false, subtitles: true, handsOff: true })).not.toContain('subtitles');
		expect(phasesFor({ withServer: false, subtitles: false })).toEqual(['language', 'welcome', 'signin', 'parts', 'folders', 'sources', 'downloads', 'access', 'done']);
	});
	it('maps older addresses onto the steps of today', () => {
		for (const s of ['start', 'import', 'apps']) expect(stepFromName(s)).toBe('parts');
		expect(stepFromName('connect')).toBe('server');
		expect(stepFromName('media')).toBe('folders');
		expect(stepFromName('downloader')).toBe('downloads');
		expect(stepFromName('source')).toBe('sources');
		expect(stepFromName('nope')).toBeNull();
		expect(stepFromName(null)).toBeNull();
	});
});

describe('setup gating', () => {
	it('opens on Link your media servers, never past it while a server is required', () => {
		expect(resumeStep(null, fresh)).toBe('server');
		expect(resumeStep('server', fresh)).toBe('server');
		expect(resumeStep('folders', fresh)).toBe('server');
		expect(resumeStep('done', fresh)).toBe('server');
	});
	it('then never past Folders until folders are saved or Hands-off is on', () => {
		const p = { ...fresh, serverDone: true };
		expect(resumeStep('folders', p)).toBe('folders');
		expect(resumeStep('done', p)).toBe('folders');
		expect(firstOpen(p)).toBe('folders');
		expect(resumeStep('done', { ...p, foldersDone: true })).toBe('done');
		expect(resumeStep('access', { ...p, handsOff: true })).toBe('access');
	});
	it('skips Media server for a password owner', () => {
		const p = { withServer: false, serverDone: false, foldersDone: false };
		expect(resumeStep(null, p)).toBe('parts');
		expect(resumeStep('server', p)).toBe('parts');
		expect(resumeStep('done', p)).toBe('folders');
		expect(nextStep('parts', p)).toBe('folders');
		expect(prevStep('parts', p)).toBeUndefined();
		expect(prevStep('parts', { withServer: true })).toBe('server');
		expect(nextStep('server', { withServer: true })).toBe('parts');
		expect(nextStep('downloads', { withServer: false, subtitles: false })).toBe('access');
		expect(nextStep('done', { withServer: true })).toBe('done');
	});
	it('turns a picked Plex address into a full one', () => {
		expect(plexAddress({ address: '10.0.0.5', port: 32400, secure: false })).toBe('http://10.0.0.5:32400');
		expect(plexAddress({ address: 'a.plex.direct', port: 32400, secure: true })).toBe('https://a.plex.direct:32400');
	});
});

describe('Plex server list', () => {
	const words = { local: 'local', remote: 'remote' };
	const base = { id: 'm', name: 'Docker 5.0', port: 32400 };
	const remote = { ...base, address: '72.80.212.121', local: false, url: 'http://72.80.212.121:32400', secure_url: 'https://72-80-212-121.abc.plex.direct:32400' };
	const local = { ...base, address: '10.10.25.50', local: true, url: 'http://10.10.25.50:32400', secure_url: 'https://10-10-25-50.abc.plex.direct:32400' };
	it('shows one row per address, never the plex.direct name', () => {
		const list = [local, remote].map((c) => plexChoiceLabel(c, words));
		expect(list).toEqual(['Docker 5.0 · 10.10.25.50 · local', 'Docker 5.0 · 72.80.212.121 · remote']);
		expect(list.join()).not.toContain('plex.direct');
	});
	it('picks the local one by default', () => {
		expect(defaultPlexPick([remote, local])).toBe('1');
		expect(defaultPlexPick([remote])).toBe('0');
		expect(defaultPlexPick([])).toBe('manual');
	});
	it('tries the plain local IP first, then the secure address', () => {
		expect(plexTries(local)).toEqual([local.url, local.secure_url]);
		expect(plexFields(plexTries(local)[0])).toEqual({ host: '10.10.25.50', port: '32400', ssl: false });
		expect(plexFields(local.secure_url!)?.ssl).toBe(true);
		expect(plexTries({ ...local, secure_url: undefined })).toEqual([local.url]);
	});
	it('never offers plain http for a remote Plex server', () => {
		const remote = { ...local, local: false, url: 'https://1-2-3-4.abc.plex.direct:32400', secure_url: 'https://1-2-3-4.abc.plex.direct:32400' };
		expect(plexTries(remote)).toEqual([remote.secure_url]);
		expect(plexTries({ ...remote, url: 'http://1.2.3.4:32400' })).toEqual([remote.secure_url]);
	});
	it('falls back to http only when the server is unreachable, never on a certificate error', () => {
		expect(plexMayFallBack('Nothing answers on that port. Check the port, and that the app is running.')).toBe(true);
		expect(plexMayFallBack('No answer in time. Check the address, and that no firewall is in the way.')).toBe(true);
		expect(plexMayFallBack('Certificate not trusted. Use the name on its certificate, or http:// if it has none.')).toBe(false);
		expect(plexMayFallBack('tls: handshake failure')).toBe(false);
		expect(plexMayFallBack('Key or password refused.')).toBe(false);
	});
	it('sets the SSL box from the address in use', () => {
		expect(plexFields(local.secure_url)).toEqual({ host: '10-10-25-50.abc.plex.direct', port: '32400', ssl: true });
		expect(plexFields(local.url)).toEqual({ host: '10.10.25.50', port: '32400', ssl: false });
		expect(plexFields('not an address')).toBeNull();
	});
	it('a remote row shows its IP but saves the secure plex.direct address', () => {
		const used = plexTries(remote)[0];
		const shown = plexShown(remote, used);
		expect(shown).toEqual({ host: '72.80.212.121', port: '32400', ssl: true });
		expect(plexFormUrl({ pick: '1', listed: used, ...shown })).toBe(remote.secure_url);
	});
	it('a local row shows its IP and saves plain http to it', () => {
		const used = plexTries(local)[0];
		const shown = plexShown(local, used);
		expect(shown).toEqual({ host: '10.10.25.50', port: '32400', ssl: false });
		expect(plexFormUrl({ pick: '0', listed: used, ...shown })).toBe('http://10.10.25.50:32400');
	});
	it('editing the host switches to manual and uses exactly what is typed', () => {
		expect(plexFormUrl({ pick: 'manual', listed: '', host: '72.80.212.121', port: '32400', ssl: false })).toBe('http://72.80.212.121:32400');
		expect(plexFormUrl({ pick: 'manual', listed: '', host: 'https://plex.example.com', port: '443', ssl: true })).toBe('https://plex.example.com:443');
	});
});
