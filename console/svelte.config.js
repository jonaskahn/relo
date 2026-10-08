import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		// The console is a single-page application: the Go server owns the
		// URLs, serves the shell for all of them, and the browser routes.
		adapter: adapter({ fallback: 'index.html' })
	}
};

export default config;
