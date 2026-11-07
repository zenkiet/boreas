import prettier from 'eslint-config-prettier';
import path from 'node:path';
import js from '@eslint/js';
import boundaries from 'eslint-plugin-boundaries';
import svelte from 'eslint-plugin-svelte';
import { defineConfig, includeIgnoreFile } from 'eslint/config';
import globals from 'globals';
import ts from 'typescript-eslint';

const gitignorePath = path.resolve(import.meta.dirname, '.gitignore');

const LAYERS = ['app', 'pages', 'entities', 'shared'];
const SLICED = ['pages', 'entities'];

const downward = LAYERS.flatMap((layer, i) =>
	LAYERS.slice(i + 1).map((target) => ({
		from: { element: { type: layer } },
		allow: {
			to: {
				element: SLICED.includes(target)
					? { type: target, fileInternalPath: 'index.ts' }
					: { type: target }
			}
		}
	}))
);

const sameSlice = SLICED.map((layer) => ({
	from: { element: { type: layer, captured: { slice: '{{slice}}' } } },
	allow: {
		to: { element: { type: layer, captured: { slice: '{{from.captured.slice}}' } } }
	}
}));

export default defineConfig(
	includeIgnoreFile(gitignorePath),
	js.configs.recommended,
	ts.configs.recommended,
	svelte.configs.recommended,
	prettier,
	svelte.configs.prettier,
	{
		languageOptions: { globals: { ...globals.browser, ...globals.node } },
		rules: {
			// typescript-eslint strongly recommend that you do not use the no-undef lint rule on TypeScript projects.
			// see: https://typescript-eslint.io/troubleshooting/faqs/eslint/#i-get-errors-from-the-no-undef-rule-about-global-variables-not-being-defined-even-though-there-are-no-typescript-errors
			'no-undef': 'off'
		}
	},
	{
		files: ['**/*.svelte', '**/*.svelte.ts', '**/*.svelte.js'],
		languageOptions: {
			parserOptions: {
				projectService: true,
				extraFileExtensions: ['.svelte'],
				parser: ts.parser
			}
		}
	},
	{
		files: [
			'src/pages/directory/ui/NoteMarkdown.svelte',
			'src/pages/privacy/ui/PrivacyPage.svelte'
		],
		rules: { 'svelte/no-at-html-tags': 'off' }
	},
	{
		files: ['src/**/*.ts', 'src/**/*.svelte'],
		plugins: { boundaries },
		settings: {
			'import/resolver': { typescript: { project: 'tsconfig.json' } },
			'boundaries/dependency-nodes': ['import', 'dynamic-import'],
			'boundaries/elements': [
				...SLICED.map((type) => ({ type, pattern: `src/${type}/*`, capture: ['slice'] })),
				{ type: 'shared', pattern: 'src/shared/*', capture: ['segment'] },
				{ type: 'app', pattern: 'src/app/**', partialMatch: false },
				{ type: 'app', pattern: 'src/routes/**', partialMatch: false }
			]
		},
		rules: {
			'boundaries/dependencies': [
				'error',
				{
					default: 'disallow',
					policies: [
						...downward,
						...sameSlice,
						{
							from: { element: { type: 'shared' } },
							allow: { to: { element: { type: 'shared' } } }
						},
						{ from: { element: { type: 'app' } }, allow: { to: { element: { type: 'app' } } } }
					]
				}
			]
		}
	}
);
