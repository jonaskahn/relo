import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

// The dev server proxies the management API to a running daemon, so the
// console is developed against real state.
const backend = process.env.RELO_BACKEND_URL ?? 'http://127.0.0.1:10101';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	build: {
		// Rolldown warns whenever a plugin callback takes significant time,
		// and SvelteKit's own compile phase always does on this console. The
		// report says nothing the build log needs, so the check is off.
		rollupOptions: {
			checks: {
				bundlerTimings: false
			}
		}
	},
	server: {
		proxy: {
			// The daemon refuses a write whose Origin does not name the
			// host it serves, so the proxy must pass Host through instead
			// of rewriting it to the backend address.
			'/api': { target: backend, changeOrigin: false },
			'/healthz': { target: backend, changeOrigin: false }
		}
	}
});
