// See https://svelte.dev/docs/kit/types#app.d.ts
declare global {
	namespace App {}

	interface Window {
		// The language the daemon handed the console with the shell. A
		// binary serving the console always sets it; a browser opening the
		// built files directly does not.
		__reloLanguage?: string;
		__reloAppearance?: { theme: string; accent: string };
	}
}

export {};
