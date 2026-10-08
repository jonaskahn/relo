import js from '@eslint/js';
import { defineConfig, globalIgnores } from 'eslint/config';
import eslintConfigPrettier from 'eslint-config-prettier';
import svelte from 'eslint-plugin-svelte';
import globals from 'globals';
import ts from 'typescript-eslint';

import svelteConfig from './svelte.config.js';

export default defineConfig(
	// Generated output and local caches carry no source rules.
	globalIgnores(['.svelte-kit/', 'build/', 'coverage/', '.vitest/']),
	js.configs.recommended,
	...ts.configs.recommended,
	...svelte.configs.recommended,
	// Prettier owns formatting, so its config comes last and turns off the
	// stylistic rules the two would otherwise disagree about.
	eslintConfigPrettier,
	...svelte.configs.prettier,
	{
		languageOptions: {
			globals: { ...globals.browser, ...globals.node }
		}
	},
	{
		files: ['**/*.ts', '**/*.svelte', '**/*.svelte.ts'],
		rules: {
			// A type that says nothing is worse than the compiler demanding one:
			// unknown forces the narrowing that any would skip.
			'@typescript-eslint/no-explicit-any': 'error',
			'@typescript-eslint/consistent-type-imports': 'error',
			// A leading underscore is how this codebase marks a value it
			// deliberately takes and drops, as in a rest destructure.
			'@typescript-eslint/no-unused-vars': [
				'error',
				{
					argsIgnorePattern: '^_',
					varsIgnorePattern: '^_',
					caughtErrorsIgnorePattern: '^_'
				}
			]
		}
	},
	{
		// Three rules this project deliberately does not enforce, each because
		// the rule and the way this console is built disagree, not because the
		// code was left sloppy. Every other recommended rule is on.
		files: ['**/*.svelte', '**/*.svelte.ts', '**/*.ts'],
		rules: {
			// resolve() is typed against SvelteKit's generated route literals,
			// so a route that carries a query string cannot be expressed with
			// it (resolve('/keys') + '?create=1' still trips the rule). The
			// console navigates with query state on every major page, and the
			// app is served from the root by the Go daemon, so there is no
			// base path for resolve() to protect.
			'svelte/no-navigation-without-resolve': 'off',
			// SvelteMap and friends are for collections that are read in a
			// reactive context. This codebase's Map and Set use is mostly
			// local grouping inside pure view logic, where the built-ins are
			// the right tool and a reactive collection would be a lie.
			'svelte/prefer-svelte-reactivity': 'off',
			// A bare `{' '}` is how a space survives the compiler's whitespace
			// handling at a block edge, and this rule's autofix rewrites it to a
			// space the compiler then eats. The three sites in
			// provider-pane.svelte and routes/+page.svelte are that case.
			'svelte/no-useless-mustaches': 'off'
		}
	},
	{
		files: ['**/*.svelte', '**/*.svelte.ts', '**/*.svelte.js'],
		languageOptions: {
			parserOptions: {
				projectService: true,
				extraFileExtensions: ['.svelte'],
				parser: ts.parser,
				svelteConfig
			}
		}
	}
);
