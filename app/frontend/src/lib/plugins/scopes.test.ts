import { describe, expect, it } from 'vitest';
import { groupLine, needLabel, reaches, resourcesLine, scopeWarnings } from './view.ts';
import { scopeLines } from './view.ts';
import type { Plugin } from './types.ts';

const base: Plugin = { id: 'poster-maker', version: '1.0.0', installed: false, running: false, enabled: false, access: null };

describe('plugin permissions on the install screen', () => {
	it.each([
		[{ id: 'library', change: false, scopes: ['library.read'] }, 'Can see your movies, shows and music'],
		[{ id: 'library', change: true, scopes: ['library.artwork.set'] }, 'Can see and change your movies, shows and music'],
		[{ id: 'requests', change: true, scopes: ['requests.change'] }, 'Can make requests for people on this server'],
		[{ id: 'network', change: true, scopes: ['network.forward'] }, 'Can add pages and tools, or make HolyArrs reachable from outside your home network'],
	])('says group %o in plain words', (g, want) => {
		expect(groupLine(g)).toBe(want);
	});
	it('lists each red warning once', () => {
		const groups = [
			{ id: 'files', change: true, scopes: ['files.rename.change', 'files.delete'], warnings: ['files', 'delete'] },
			{ id: 'library', change: true, scopes: ['library.delete'], warnings: ['delete'] },
			{ id: 'network', change: true, scopes: ['network.forward'], warnings: ['forward'] },
		];
		expect(scopeWarnings(groups)).toEqual(['Can change your media files', 'Can delete titles, files or downloads', 'Sees everything sent through it, including sign-ins']);
		expect(scopeWarnings(undefined)).toEqual([]);
	});
	it('fills in default caps', () => {
		expect(resourcesLine(undefined)).toBe('');
		expect(resourcesLine({ memory_mib: 2048 })).toBe('Asks for 2048 MiB of memory, 1 CPUs, 256 processes and 1024 open files');
	});
	it('says new permissions and caps that need approval', () => {
		expect(needLabel('scope:library.change', base)).toBe('New permission: library.change');
		expect(needLabel('resources', base)).toBe('More memory, CPUs, processes or open files than usual');
	});
	it('shows groups instead of raw scope names', () => {
		const p = { ...base, access: { hosts: null, folders: null, actions: ['library.read'] }, scope_groups: [{ id: 'library', change: false, scopes: ['library.read'] }] };
		expect(reaches(p)).toEqual(['Can see your movies, shows and music']);
	});
});

describe('long lists', () => {
	it('cuts a long host list to five and a count', () => {
		const hosts = Array.from({ length: 100 }, (_, i) => `h${i}.example.com`);
		expect(reaches({ ...base, access: { hosts, folders: null, actions: null } })[0]).toBe('Connect to h0.example.com, h1.example.com, h2.example.com, h3.example.com, h4.example.com +95');
	});
});

describe('every granted scope has its own line', () => {
	it('lists library.read on its own, with its plain words', () => {
		const lines = scopeLines([{ id: 'library', change: true, scopes: ['library.change', 'library.read'] }]);
		expect(lines.map((l) => l.scope)).toEqual(['library.change', 'library.read']);
		expect(lines[1].line).not.toBe(lines[0].line);
		expect(lines.every((l) => l.line && !l.line.startsWith('plugins.'))).toBe(true);
	});
});
