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
	'google-antigravity': 'google',
	copilot: 'github-copilot',
	grok: 'xai',
	kimi: 'kimi-code-plan-cn',
	'orcarouter-oauth': 'orcarouter',
	'meta-muse': 'meta',
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
