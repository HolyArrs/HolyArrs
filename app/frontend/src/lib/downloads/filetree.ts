// Files tab tree: folder rows and file rows built from a torrent's flat file list (paths use forward slashes). Pure, so it can be tested.

/** A file as the torrent client reports it; done is bytes finished. */
export interface FlatFile { name: string; size: number; done: number }

export interface TreeRow { key: string; name: string; depth: number; dir: boolean; size: number; done: number; i: number }

/** Builds the folder and file rows, hiding what sits inside a closed folder. */
export function fileTree(files: FlatFile[], closed: (key: string) => boolean): TreeRow[] {
	const dirs = new Map<string, { size: number; done: number; order: number }>();
	files.forEach((f) => {
		const parts = f.name.split('/');
		for (let d = 1; d < parts.length; d++) {
			const key = parts.slice(0, d).join('/');
			const e = dirs.get(key) ?? { size: 0, done: 0, order: dirs.size };
			e.size += f.size;
			e.done += f.done;
			dirs.set(key, e);
		}
	});
	const out: TreeRow[] = [];
	const seen = new Set<string>();
	files.forEach((f, i) => {
		const parts = f.name.split('/');
		for (let d = 1; d < parts.length; d++) {
			const key = parts.slice(0, d).join('/');
			if (seen.has(key)) continue;
			seen.add(key);
			if (parts.slice(0, d - 1).some((_, k) => closed(parts.slice(0, k + 1).join('/')))) continue;
			const e = dirs.get(key)!;
			out.push({ key, name: parts[d - 1], depth: d - 1, dir: true, size: e.size, done: e.done, i: -1 });
		}
		for (let k = 1; k < parts.length; k++) if (closed(parts.slice(0, k).join('/'))) return;
		out.push({ key: f.name, name: parts[parts.length - 1], depth: parts.length - 1, dir: false, size: f.size, done: f.done, i });
	});
	return out;
}
