import { defineConfig } from 'vitest/config';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { fileURLToPath } from 'node:url';

export default defineConfig({
	plugins: [svelte()],
	resolve: {
		// SvelteKit owns this alias in the app, so a component test that
		// renders one has to resolve it the same way.
		alias: {
			$lib: fileURLToPath(new URL('./src/lib', import.meta.url))
		}
	},
	test: {
		include: ['src/**/*.test.ts'],
		coverage: {
			provider: 'v8',
			reporter: ['text', 'json-summary'],
			// The gate is a floor on what the console's own logic is measured
			// at. Svelte components are exercised through the pages that render
			// them rather than mounted on their own, so they are reported but
			// not counted against the threshold.
			include: ['src/lib/**/*.ts', 'src/routes/**/+page.ts'],
			exclude: ['src/**/*.test.ts', 'src/lib/i18n/locales/**'],
			thresholds: { lines: 85, functions: 85, statements: 85, branches: 85 }
		}
	}
});
