// The colour maths behind the radar and the accent it writes: coordinates for
// picking, a hex for saving, and the ink that stays legible on the result.

/** A colour the radar writes, as the daemon stores it: #rrggbb. */
export type HexColour = `#${string}`;

/** Where the radar's cursor sits: hue in degrees, saturation and value as
 *  fractions of the full range. */
export interface Hsv {
	hue: number;
	saturation: number;
	value: number;
}

/** Reports whether a value is a colour in the shape both ends store. */
export function isHexColour(value: string): value is HexColour {
	return /^#[0-9a-fA-F]{6}$/.test(value);
}

/** Reads what an operator typed, with or without the leading #, as a colour
 *  the rest of the console can apply. An unreadable value reports nothing. */
export function parseHex(value: string): HexColour | undefined {
	const bare = value.trim().replace(/^#/, '');
	if (!/^[0-9a-fA-F]{6}$/.test(bare)) return undefined;
	return `#${bare.toLowerCase()}` as HexColour;
}

/** Reads a colour as the radar's coordinates. */
export function hexToHsv(hex: string): Hsv {
	const value = hex.replace(/^#/, '');
	const red = parseInt(value.slice(0, 2), 16) / 255;
	const green = parseInt(value.slice(2, 4), 16) / 255;
	const blue = parseInt(value.slice(4, 6), 16) / 255;
	const high = Math.max(red, green, blue);
	const low = Math.min(red, green, blue);
	const span = high - low;
	let hue = 0;
	if (span > 0) {
		if (high === red) hue = 60 * (((green - blue) / span) % 6);
		else if (high === green) hue = 60 * ((blue - red) / span + 2);
		else hue = 60 * ((red - green) / span + 4);
	}
	return {
		hue: (hue + 360) % 360,
		saturation: high === 0 ? 0 : span / high,
		value: high
	};
}

/** Writes the radar's coordinates back as a colour. */
export function hsvToHex(hsv: Hsv): HexColour {
	const hue = ((hsv.hue % 360) + 360) % 360;
	const saturation = Math.min(1, Math.max(0, hsv.saturation));
	const value = Math.min(1, Math.max(0, hsv.value));
	const chroma = value * saturation;
	const segment = hue / 60;
	const second = chroma * (1 - Math.abs((segment % 2) - 1));
	const match = value - chroma;
	const [red, green, blue] =
		segment < 1
			? [chroma, second, 0]
			: segment < 2
				? [second, chroma, 0]
				: segment < 3
					? [0, chroma, second]
					: segment < 4
						? [0, second, chroma]
						: segment < 5
							? [second, 0, chroma]
							: [chroma, 0, second];
	const channel = (part: number) =>
		Math.round((part + match) * 255)
			.toString(16)
			.padStart(2, '0');
	return `#${channel(red)}${channel(green)}${channel(blue)}`;
}

/** The pure hue the radar's rail and cursor show, as a stylesheet value. The
 *  components cannot name colours themselves, so this string is the only place
 *  a hue is written. */
export function hueColour(hue: number): string {
	return `hsl(${Math.round(((hue % 360) + 360) % 360)} 100% 50%)`;
}

/** The ink that reads best on an accent base, measured the way WCAG measures
 *  contrast, so a colour no preset shipped with still carries 4.5:1 text. */
export function onAccent(base: string): string {
	return contrast(base, '#ffffff') >= contrast(base, '#17171c') ? '#ffffff' : '#17171c';
}

/** The WCAG contrast ratio of two #rrggbb colours. A value that is not one
 *  reports no contrast, which leaves the caller with the other ink. */
function contrast(a: string, b: string): number {
	const first = luminance(a);
	const second = luminance(b);
	if (first === null || second === null) return 0;
	const high = Math.max(first, second);
	const low = Math.min(first, second);
	return (high + 0.05) / (low + 0.05);
}

/** The WCAG relative luminance of a #rrggbb colour. */
function luminance(hex: string): number | null {
	if (!isHexColour(hex)) return null;
	const channel = (part: number) => {
		const value = part / 255;
		return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
	};
	const value = parseInt(hex.slice(1), 16);
	return (
		0.2126 * channel((value >> 16) & 0xff) +
		0.7152 * channel((value >> 8) & 0xff) +
		0.0722 * channel(value & 0xff)
	);
}
