import logoIds from '$lib/provider-logo-ids.json';
import { providerLogoPaths, type LogoPath } from '$lib/provider-logo-paths';

const available = new Set(logoIds);

// Marks a mask cannot draw. A mask keeps only alpha, so a mark is out when its
// own inks are the drawing, or when it is a raster with no alpha to speak of:
// 302ai is drawn in three greys, and atomic-chat embeds a raster that is opaque
// to its edge. These are drawn as plain images, which costs them the themed
// ink but keeps the mark readable.
const colored = new Set(['302ai', 'atomic-chat']);

const aliases: Record<string, string> = {
	'openai-codex': 'openai',
	claude: 'anthropic',
	copilot: 'github-copilot',
	grok: 'xai',
	kimi: 'kimi-code-plan-cn',
	'orcarouter-oauth': 'orcarouter',
	'meta-muse': 'meta',
	// The keyless lane wears the gateway's mark: it is the same vendor, and the
	// free pool is not a product of its own.
	'kilo-free': 'kilo',
	// Claude Desktop has no mark of its own and wears the vendor it talks to.
	// An agent that does have its own mark needs no entry here.
	'claude-desktop': 'anthropic'
};

/** The mark the console draws for one connection.
 *  The chart and the logo tile share this so an alias cannot drift. */
export function providerLogoSrc(id: string): string | null {
	const logoId = aliases[id] ?? id;
	if (!available.has(logoId)) return null;
	return `/provider-logos/${encodeURIComponent(logoId)}.svg`;
}

/** Reports whether that id's mark has to be drawn as an image rather than a mask, because its
 *  own inks are part of the drawing. */
export function providerLogoIsColored(id: string): boolean {
	return colored.has(aliases[id] ?? id);
}

/** Returns the geometry for one mark, for the surfaces that cannot draw the .svg file itself.
 *  The universe chart is one: a CSS mask does not reach an SVG child and currentColor does not
 *  resolve inside one, so a themed mark has to be a path the chart paints itself.
 *  It is null both for an id with no mark and for a mark that only exists as a file, so a caller
 *  must fall back the same way it does for a missing file. */
export function providerLogoPath(id: string): LogoPath | null {
	const logoId = aliases[id] ?? id;
	return providerLogoPaths[logoId] ?? null;
}

/** The two-letter fallback when that id has no logo file. */
export function providerMonogram(label: string): string {
	const words = label.split(/\s+/).filter(Boolean);
	if (words.length >= 2) return (words[0][0] + words[1][0]).toUpperCase();
	return label.slice(0, 2).toUpperCase();
}

/** The transform that draws one mark centred on a point.
 *  The geometry is drawn in its own viewBox, so it is scaled into the box the caller wants
 *  and shifted by half its own size: a mark that is not square keeps its shape and sits on
 *  the point either way. */
export function logoPathTransform(path: LogoPath, cx: number, cy: number, size: number): string {
	const scale = size / Math.max(path.width, path.height);
	const [vx, vy] = path.viewBox.split(' ').map(Number);
	return `translate(${cx} ${cy}) scale(${scale}) translate(${-vx - path.width / 2} ${
		-vy - path.height / 2
	})`;
}
