// A shared reactive open flag so the top bar, the keyboard hook, and the
// palette component agree on one dialog without a store library.

class PaletteState {
	/** The one flag the top bar, the keyboard hook and the palette itself all read and set. */
	value = $state(false);
}

/** The one flag the top bar, the keyboard hook and the palette itself all read and set. */
export const paletteOpen = new PaletteState();
