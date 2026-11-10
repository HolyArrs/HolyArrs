// Builds src/lib/core/i18n/messages/en/server-lines.json: the English lines the
// server sends that a person reads (API errors, labels, health lines, card
// lines, event summaries). serverWords() in lib/core/i18n/server-words.ts maps them
// back to keys so the page can show them in the picked language.
// Run: node scripts/server-words.mjs  (then review the diff by hand).
// Lines already in the file keep their keys (the other languages hold them);
// a run only adds keys for new lines. Reword an old line by editing its value by hand.
import { existsSync, readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs';
import { join, relative, sep } from 'node:path';

const root = new URL('../../backend/internal/', import.meta.url).pathname.replace(/^\/(\w:)/, '$1');
const out = new URL('../src/lib/core/i18n/messages/en/server-lines.json', import.meta.url).pathname.replace(/^\/(\w:)/, '$1');
// Words the app never shows (scripts/check-words.mjs); a line holding one is left out.
const banned = ['Monitored', 'Activity', 'Wanted', 'Cutoff', 'Quality Profile', 'Root Folder', 'Download Client',
	'Interactive Search', 'Manual Import', 'Grabbed', 'Connections', 'Discover', 'Series'];
// Files whose fmt.Sprintf lines are display text (card lines, labels).
const display = /(accounts[\\/]events|discover[\\/](home|explore_np|title_view)|shows[\\/]episode_labels|requests[\\/](episodes|follows\w*|reports)|health[\\/](health|routes_settings_status)|migrate[\\/]plan_\w+|metadata[\\/]moods)\.go$/;

function walk(dir, files = []) {
	for (const name of readdirSync(dir)) {
		const p = join(dir, name);
		if (statSync(p).isDirectory()) walk(p, files);
		else if (p.endsWith('.go') && !p.endsWith('_test.go')) files.push(p);
	}
	return files;
}

// Reads one Go expression from i: up to a top-level , ) } or a line end not after +.
function expr(src, i) {
	let depth = 0, j = i, last = '';
	for (; j < src.length; j++) {
		const c = src[j];
		if (c === '"' || c === '`') {
			j = str(src, j).end - 1;
			last = '"';
			continue;
		}
		if ('([{'.includes(c)) depth++;
		else if (')]}'.includes(c)) {
			if (depth === 0) break;
			depth--;
		} else if (c === ',' && depth === 0) break;
		else if (c === '\n' && depth === 0 && last !== '+') break;
		if (!/\s/.test(c)) last = c;
	}
	return src.slice(i, j).trim();
}

function str(src, i) {
	const q = src[i];
	let j = i + 1;
	while (j < src.length && src[j] !== q) j += src[j] === '\\' && q === '"' ? 2 : 1;
	const raw = src.slice(i + 1, j);
	const text = q === '`' ? raw : raw.replace(/\\(["\\nt])/g, (_, c) => (c === 'n' ? '\n' : c === 't' ? '\t' : c));
	return { text, end: j + 1 };
}

function parts(e) {
	const ps = [];
	let depth = 0, start = 0;
	for (let j = 0; j < e.length; j++) {
		const c = e[j];
		if (c === '"' || c === '`') j = str(e, j).end - 1;
		else if ('([{'.includes(c)) depth++;
		else if (')]}'.includes(c)) depth--;
		else if (c === '+' && depth === 0) {
			ps.push(e.slice(start, j).trim());
			start = j + 1;
		}
	}
	ps.push(e.slice(start).trim());
	return ps;
}

function slotName(e) {
	if (/\b(?:byName|userName)\(/.test(e)) return 'name';
	const inner = e.match(/\(([^()]*)\)\s*$/);
	const ids = (inner && inner[1].trim() ? inner[1] : e).match(/[A-Za-z_]\w*/g) ?? ['x'];
	return ids[ids.length - 1].toLowerCase();
}

function sprintf(e) {
	const m = e.match(/^fmt\.Sprintf\(\s*("(?:[^"\\]|\\.)*"|`[^`]*`)\s*,?([\s\S]*)\)$/);
	if (!m) return null;
	const fmt = str(m[1], 0).text;
	const args = [];
	let rest = m[2].trim();
	while (rest) {
		const a = expr(rest + ')', 0);
		args.push(a);
		rest = rest.slice(a.length).replace(/^\s*,/, '').trim();
		if (!a) break;
	}
	let k = 0;
	return fmt.replace(/%%|%[-+# 0]*\d*(?:\.\d+)?[dsvqfx]/g, (v) => (v === '%%' ? '%' : `{${slotName(args[k++] ?? 'x')}}`));
}

// The English template of an expression, or null when it holds no words.
function template(e) {
	let text = '', words = false;
	for (const p of parts(e)) {
		if (/^("(?:[^"\\]|\\.)*"|`[^`]*`)$/.test(p)) {
			text += str(p, 0).text;
			words = true;
		} else if (p.startsWith('fmt.Sprintf(')) {
			const s = sprintf(p);
			if (s == null) return null;
			text += s;
			words = true;
		} else {
			text += `{${slotName(p)}}`;
		}
	}
	if (!words) return null;
	// Each slot name once, so a line can be filled back by name.
	const seen = new Map();
	return text.trim().replace(/\{(\w+)\}/g, (_, n) => {
		const c = (seen.get(n) ?? 0) + 1;
		seen.set(n, c);
		return `{${c > 1 ? n + c : n}}`;
	});
}

function readable(t, error = false) {
	if (!t || t.includes('\n') || t.length > 300) return false;
	if (migrateExtra.includes(t)) return true;
	if (!(error ? /^[A-Za-z{]/ : /^[A-Z{]/).test(t)) return false;
	if ((t.replace(/\{\w+\}/g, '').match(/[A-Za-z]/g) ?? []).length < 3) return false;
	if (/%[wdsvq]|\{x\}|^\{\w+\}$|\b[a-z]+_[a-z_]+\b|\b(sql|SELECT|json|nil)\b/.test(t)) return false;
	return !banned.some((w) => t.includes(w));
}


// The person-facing lines of internal/migrate: result details, reasons, notes, next steps.
const migrateFile = /[\\/]internal[\\/]migrate[\\/][^\\/]+\.go$/;
// Words kept for slots (a line's gap is filled with these) and a few bare labels.
const migrateExtra = ['(missing)', '(no name)', '(none supported)', 'announced', 'in cinemas', 'released', "this server's default"];

// The top-level arguments of the call or literal whose opening bracket sits just before i.
function args(src, i) {
	const out = [];
	let depth = 0, start = i, j = i;
	for (; j < src.length; j++) {
		const c = src[j];
		if (c === '"' || c === '`') {
			j = str(src, j).end - 1;
			continue;
		}
		if ('([{'.includes(c)) depth++;
		else if (')]}'.includes(c)) {
			if (depth === 0) break;
			depth--;
		} else if (c === ',' && depth === 0) {
			out.push(src.slice(start, j).trim());
			start = j + 1;
		}
	}
	out.push(src.slice(start, j).trim());
	return out.filter(Boolean);
}

function migratePass(file, src, addLine) {
	const at = (re, f) => {
		for (const m of src.matchAll(re)) f(m.index + m[0].length);
	};
	const one = (i) => addLine(template(expr(src, i)));
	// Result words next to a result: return ResX, "..."  result(err, ResX, "...")  r.line(..., ResX, "...", "")
	at(/\bRes[A-Za-z]+,\s*(?=["`]|fmt\.Sprintf|[a-z]\w*\s*\+)/g, one);
	// A not-imported line: what it is, why it is left out.
	at(/\bb\.not\(/g, (i) => {
		const a = args(src, i);
		if (a.length < 2) return;
		addLine(template(a[a.length - 1]));
		const name = template(a[a.length - 2]);
		if (name && (name.replace(/\{\w+\}/g, '').match(/[A-Za-z]/g) ?? []).length >= 8) addLine(name);
	});
	// Fields and locals that hold a sentence.
	at(/\b(?:Detail|Why|Name|Needs)\s*:\s*(?=["`]|fmt\.Sprintf|[a-z]\w*\s*\+)/g, one);
	at(/\b(?:detail|why|reason|check|was|needs|values|d|it\.Needs|ap\.Note)\s*:?=\s*(?=["`]|fmt\.Sprintf)/g, one);
	at(/\bwhat\s*:?=\s*(?=["`])/g, one);
	at(/\b(?:scores|out)\s*=\s*(?=fmt\.Sprintf)/g, one);
	// Next steps and notes: lists and appends.
	at(/\b(?:Next|Notes)\s*(?::|=)\s*\[\]string\{/g, (i) => args(src, i).forEach((a) => addLine(template(a))));
	at(/\bappend\(\s*(?:out|rep\.Next|\*?\w+\.notes|\w+\.Notes|\w+\.NeedsReentry|b\.p\.Notes|s\.Notes|out\.notes|\*t\.notes)\s*,\s*/g, one);
	// Errors a person reads.
	at(/\b(?:Err|err)[A-Z]\w*\s*=\s*errors\.New\(/g, one);
	at(/\bconst\s+\w+Note\s*=\s*/g, one);
	at(/\bkept[A-Z]\w*\s*=\s*/g, one);
	// A music issue: the third part is the sentence.
	at(/\bmusicIssue\{/g, (i) => {
		const a = args(src, i);
		if (a.length === 3) addLine(template(a[2]));
	});
	// return "label", "sentence"   and   return "", "", "sentence"
	at(/\breturn\s+"[^"]*",\s*(?=["`])/g, one);
	at(/\breturn\s+"",\s*"",\s*(?=")/g, one);
	// Small tables: a map literal indexed in place, and the gate words.
	for (const m of src.matchAll(/map\[string\]string\{([^}]*)\}\[/g))
		for (const v of m[1].matchAll(/:\s*("(?:[^"\\]|\\.)*")/g)) addLine(str(v[1], 0).text.trim());
	for (const m of src.matchAll(/var\s+gateWords\s*=\s*map\[[^\]]+\]string\s*\{([^}]*)\}/g))
		for (const v of m[1].matchAll(/:\s*("(?:[^"\\]|\\.)*")/g)) addLine(str(v[1], 0).text);
	// The group names in lower case ("you left library folders out").
	if (/plan_types\.go$/.test(file))
		for (const m of src.matchAll(/Group\w+:\s*("(?:[^"\\]|\\.)*")/g)) {
			const w = str(m[1], 0).text;
			if (w.toLowerCase() !== w) addLine(w.toLowerCase());
		}
	// The address-read failures.
	if (/fetch\.go$/.test(file)) at(/\breturn\s+(?=")/g, one);
}

// The lenient test for migrate lines: lower case starts are fine, SQL and code are not.
function migrateKeep(t) {
	if (!t || t.includes('\n') || /\b(SELECT|INSERT|UPDATE|DELETE|WHERE|Job|job queue)\b|^[?, ]+$/.test(t)) return false;
	return migrateExtra.includes(t) || /[a-z]{3}/.test(t.replace(/\{\w+\}/g, ''));
}

const found = new Map(); // text -> module
function add(file, t, error = false) {
	const mod = relative(root, file).split(sep)[0];
	// arrapi answers Sonarr/Radarr-style apps, not people.
	if (mod === 'arrapi' || !readable(t, error) || found.has(t)) return;
	found.set(t, mod);
}

for (const file of walk(root)) {
	const src = readFileSync(file, 'utf8');
	const at = (re, f) => {
		for (const m of src.matchAll(re)) f(m.index + m[0].length);
	};
	// API errors.
	at(/\b(?:fail|webserver\.Fail\w*)\(\s*w\s*,\s*[^,]+,\s*/g, (i) => add(file, template(expr(src, i)), true));
	// Event summaries (the bell and history).
	at(/\bSummary:\s*/g, (i) => add(file, template(expr(src, i))));
	at(/\bsummary\s*:?=\s*/g, (i) => add(file, template(expr(src, i))));
	// The line of an Event written through a helper: a.event(r, by, kind, line), accountEvent(...), h.Event(...), m.event(...).
	at(/\b(?:a\.event|accountEvent|h\.Event|m\.event)\(/g, (i) => {
		const a = args(src, i);
		if (a.length >= 4) add(file, template(a[3]));
	});
	// Labels.
	at(/\b\w*Label:\s*(?=")/g, (i) => add(file, template(expr(src, i))));
	for (const m of src.matchAll(/var\s+\w*(?:Labels?|Words|Names)\s*=\s*map\[[^\]]+\]string\s*\{([\s\S]*?)\n\}/g))
		for (const v of m[1].matchAll(/:\s*("(?:[^"\\]|\\.)*")/g)) add(file, str(v[1], 0).text);
	// Why a synced trait was left out, and the test kinds named with it.
	at(/\bReason:\s*(?=")/g, (i) => add(file, template(expr(src, i)), true));
	for (const m of src.matchAll(/var\s+\w*Kinds\s*=\s*map\[[^\]]+\]string\s*\{([\s\S]*?)\n\}/g))
		for (const v of m[1].matchAll(/:\s*("(?:[^"\\]|\\.)*")/g)) add(file, str(v[1], 0).text, true);
	for (const v of src.matchAll(/\bbad\["([a-z][a-z ]+)"\]\s*=\s*true/g)) add(file, v[1], true);
	// Health lines.
	at(/\bStatusItem\{\s*[^,]+,\s*[^,]+,\s*/g, (i) => add(file, template(expr(src, i))));
	if (migrateFile.test(file)) migratePass(file, src, (t) => t && migrateKeep(t) && add(file, t, true));
	if (display.test(file)) {
		at(/(?=fmt\.Sprintf\()/g, (i) => add(file, template(expr(src, i))));
		at(/\breturn\s+(?=")/g, (i) => add(file, template(expr(src, i))));
		if (/moods\.go$/.test(file)) for (const v of src.matchAll(/"([A-Z][A-Za-z &'-]+)"/g)) add(file, v[1]);
	}
}

// Keep every line already in the file under its key; give new lines new keys.
const words = existsSync(out) ? JSON.parse(readFileSync(out, 'utf8')) : {};
const keys = new Set(Object.keys(words));
const known = new Set(Object.values(words));
let added = 0;
for (const [text, mod] of [...found].sort((a, b) => a[1].localeCompare(b[1]) || a[0].localeCompare(b[0]))) {
	if (known.has(text)) continue;
	let slug = text.replace(/\{\w+\}/g, ' ').toLowerCase().replace(/[^a-z0-9]+/g, ' ').trim().split(' ').slice(0, 6).join('-') || 'line';
	// A key ending in a secret word looks like a stored secret to the commit scan.
	if (/(password|passwd|token|secret|key)$/.test(slug)) slug += '-line';
	let key = `server.${mod}.${slug}`;
	for (let n = 2; keys.has(key); n++) key = `server.${mod}.${slug}-${n}`;
	keys.add(key);
	words[key] = text;
	added++;
}
writeFileSync(out, JSON.stringify(words, null, '\t') + '\n');
console.log(`server words: ${keys.size} lines (${added} new) -> ${relative(process.cwd(), out)}`);
for (const t of known) if (!found.has(t)) console.log(`not found in the Go text any more: ${t}`);
