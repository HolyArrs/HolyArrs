// Import zones for src/ (see docs/dev/modules.md): a module reaches another
// module only through its index.ts, lib/core imports no module, lib/ui imports
// only lib/core, and pages reach modules only through index.ts.
// The layer order is checked by src/lib/core/structure.test.ts.
import { readdirSync, statSync } from 'node:fs';
import importX, { createNodeResolver } from 'eslint-plugin-import-x';
import svelteParser from 'svelte-eslint-parser';
import ts from 'typescript-eslint';

const lib = './src/lib';
const modules = readdirSync(lib).filter((d) => d !== 'core' && d !== 'ui' && statSync(`${lib}/${d}`).isDirectory());
const indexes = modules.map((m) => `./${m}/index.ts`);

const zones = [
	{ target: `${lib}/core`, from: lib, except: ['./core', './ui/Icon.svelte'], message: 'lib/core imports no module.' },
	{ target: `${lib}/ui`, from: lib, except: ['./core', './ui'], message: 'lib/ui imports only lib/core.' },
	...modules.map((m) => ({
		target: `${lib}/${m}`,
		from: lib,
		except: ['./core', './ui', `./${m}`, ...indexes.filter((i) => i !== `./${m}/index.ts`)],
		message: 'Import another module only through its index.ts.'
	})),
	{ target: './src/routes', from: lib, except: ['./core', './ui', ...indexes], message: 'Pages reach a module only through its index.ts.' }
];

export default ts.config(
	{ ignores: ['dist/**', 'node_modules/**', '.svelte-kit/**', 'build/**'] },
	{ linterOptions: { reportUnusedDisableDirectives: 'off' } },
	{
		files: ['src/**/*.ts', 'src/**/*.svelte'],
		ignores: ['**/*.test.ts'],
		languageOptions: { parser: ts.parser },
		plugins: { 'import-x': importX },
		settings: { 'import-x/resolver-next': [createNodeResolver({ extensions: ['.ts', '.js', '.svelte'] })] },
		rules: { 'import-x/no-restricted-paths': ['error', { zones }] }
	},
	{
		files: ['src/**/*.svelte'],
		languageOptions: { parser: svelteParser, parserOptions: { parser: ts.parser, extraFileExtensions: ['.svelte'] } }
	}
);
