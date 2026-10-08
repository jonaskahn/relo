// A reply arrives as text, and the console reads the part of Markdown a model
// actually writes: headings, lists, tables, quotes, fenced code, and the
// inline marks that travel inside them. Parsing happens here, apart from the
// component, so the rules a reply is read by can be checked directly.
//
// The parser builds a tree and the component renders that tree with snippets,
// so nothing a model wrote is ever handed to {@html}: an <img> tag in a reply
// stays the text it is.

/** One inline mark of a reply, split by kind so the component renders each with its own snippet
 *  and never hands a model's text to {@html}. */
export type Inline =
	| { kind: 'text'; text: string }
	| { kind: 'break' }
	| { kind: 'code'; text: string }
	| { kind: 'strong'; children: Inline[] }
	| { kind: 'em'; children: Inline[] }
	| { kind: 'del'; children: Inline[] }
	| { kind: 'link'; href: string; children: Inline[] };

/** How one markdown table column is justified. */
export type TableAlign = 'left' | 'center' | 'right' | null;

/** One parsed markdown block, discriminated by kind. */
export type Block =
	| { kind: 'paragraph'; children: Inline[] }
	| { kind: 'heading'; level: number; children: Inline[] }
	| { kind: 'code'; language: string; text: string }
	| { kind: 'list'; ordered: boolean; start: number; items: Block[][] }
	| { kind: 'quote'; children: Block[] }
	| { kind: 'divider' }
	| { kind: 'table'; align: TableAlign[]; header: Inline[][]; rows: Inline[][][] };

const FENCE = /^\s{0,3}(`{3,}|~{3,})(.*)$/;
const HEADING = /^\s{0,3}(#{1,6})\s+(.*)$/;
const QUOTE = /^\s{0,3}>\s?(.*)$/;
const DIVIDER = /^\s{0,3}(?:-{3,}|\*{3,}|_{3,})\s*$/;
const BULLET = /^(\s*)[-*+]\s+(.*)$/;
const NUMBERED = /^(\s*)(\d{1,9})[.)]\s+(.*)$/;
// The row under a table header is what makes it a table: dashes with optional
// colons, which is also where the alignment of each column is read.
const TABLE_DELIMITER = /^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$/;

/** Reads a whole reply into blocks. */
export function parseMarkdown(source: string): Block[] {
	const lines = source.replace(/\r\n?/g, '\n').split('\n');
	const blocks: Block[] = [];
	let index = 0;

	while (index < lines.length) {
		const line = lines[index];
		if (line.trim() === '') {
			index += 1;
			continue;
		}

		const fence = FENCE.exec(line);
		if (fence !== null) {
			const [block, next] = readFence(lines, index, fence);
			blocks.push(block);
			index = next;
			continue;
		}

		if (DIVIDER.test(line)) {
			blocks.push({ kind: 'divider' });
			index += 1;
			continue;
		}

		const heading = HEADING.exec(line);
		if (heading !== null) {
			blocks.push({ kind: 'heading', level: heading[1].length, children: parseInline(heading[2]) });
			index += 1;
			continue;
		}

		if (QUOTE.test(line)) {
			const [block, next] = readQuote(lines, index);
			blocks.push(block);
			index = next;
			continue;
		}

		if (BULLET.test(line) || NUMBERED.test(line)) {
			const [block, next] = readList(lines, index);
			blocks.push(block);
			index = next;
			continue;
		}

		if (isTableStart(lines, index)) {
			const [block, next] = readTable(lines, index);
			blocks.push(block);
			index = next;
			continue;
		}

		const [block, next] = readParagraph(lines, index);
		blocks.push(block);
		index = next;
	}

	return blocks;
}

function readFence(lines: string[], start: number, open: RegExpExecArray): [Block, number] {
	const marker = open[1][0];
	const length = open[1].length;
	const language = open[2].trim().split(/\s+/)[0] ?? '';
	const close = new RegExp('^\\s{0,3}' + (marker === '`' ? '`' : '~') + '{' + length + ',}\\s*$');
	const body: string[] = [];
	let index = start + 1;

	while (index < lines.length) {
		if (close.test(lines[index])) {
			return [{ kind: 'code', language, text: body.join('\n') }, index + 1];
		}
		body.push(lines[index]);
		index += 1;
	}

	return [{ kind: 'code', language, text: body.join('\n') }, index];
}

function readQuote(lines: string[], start: number): [Block, number] {
	const body: string[] = [];
	let index = start;
	while (index < lines.length) {
		const match = QUOTE.exec(lines[index]);
		if (match === null) break;
		body.push(match[1]);
		index += 1;
	}
	return [{ kind: 'quote', children: parseMarkdown(body.join('\n')) }, index];
}

function readParagraph(lines: string[], start: number): [Block, number] {
	const body: string[] = [];
	let index = start;
	while (index < lines.length) {
		if (lines[index].trim() === '') break;
		if (index > start && isBlockStart(lines, index)) break;
		body.push(lines[index].trim());
		index += 1;
	}
	return [{ kind: 'paragraph', children: parseInline(body.join('\n')) }, index];
}

function isBlockStart(lines: string[], index: number): boolean {
	const line = lines[index];
	return (
		FENCE.test(line) ||
		HEADING.test(line) ||
		QUOTE.test(line) ||
		DIVIDER.test(line) ||
		BULLET.test(line) ||
		NUMBERED.test(line) ||
		isTableStart(lines, index)
	);
}

function isTableStart(lines: string[], index: number): boolean {
	if (!lines[index].includes('|')) return false;
	return index + 1 < lines.length && TABLE_DELIMITER.test(lines[index + 1]);
}

function readList(lines: string[], start: number): [Block, number] {
	const base = indentOf(lines[start]);
	const numbered = NUMBERED.exec(lines[start]);
	const ordered = numbered !== null;
	const first = numbered === null ? 1 : Number(numbered[2]);
	const body: string[] = [];
	let index = start;

	while (index < lines.length) {
		const line = lines[index];
		if (line.trim() === '') {
			// A blank line belongs to the list only when indented content follows
			// it, which is how a list ends without swallowing the next paragraph.
			const next = lines[index + 1];
			if (next === undefined || next.trim() === '' || indentOf(next) <= base) break;
			body.push('');
			index += 1;
			continue;
		}
		const indent = indentOf(line);
		if (indent < base) break;
		if (indent === base && !isMarker(line)) break;
		body.push(line);
		index += 1;
	}

	const items: Block[][] = [];
	let current: string[] | null = null;
	for (const line of body) {
		const marker = line.trim() === '' ? null : (BULLET.exec(line) ?? NUMBERED.exec(line));
		if (marker !== null && indentOf(line) === base) {
			// A bullet match carries two groups and a numbered one carries three,
			// so the item's own text is the last of them.
			const text = marker[marker.length - 1];
			current = [text];
			items.push(parseMarkdown(text));
			continue;
		}
		if (current === null) continue;
		const stripped = stripIndent(line, base);
		current.push(stripped);
		// The item is parsed again as it grows, so a nested list under it lands
		// inside the same item rather than beside it.
		items[items.length - 1] = parseMarkdown(current.join('\n'));
	}

	return [{ kind: 'list', ordered, start: first, items }, index];
}

function readTable(lines: string[], start: number): [Block, number] {
	const header = splitRow(lines[start]).map((cell) => parseInline(cell));
	const align = splitRow(lines[start + 1]).map(cellAlign);
	const rows: Inline[][][] = [];
	let index = start + 2;

	while (index < lines.length && lines[index].trim() !== '' && lines[index].includes('|')) {
		const cells = splitRow(lines[index]).map((cell) => parseInline(cell));
		while (cells.length < header.length) cells.push([]);
		rows.push(cells.slice(0, header.length));
		index += 1;
	}

	return [{ kind: 'table', align, header, rows }, index];
}

function splitRow(line: string): string[] {
	const text = line
		.trim()
		.replace(/^\|/, '')
		.replace(/\|\s*$/, '');
	const cells: string[] = [];
	let current = '';
	for (let index = 0; index < text.length; index += 1) {
		const char = text[index];
		if (char === '\\' && text[index + 1] === '|') {
			current += '|';
			index += 1;
			continue;
		}
		if (char === '|') {
			cells.push(current.trim());
			current = '';
			continue;
		}
		current += char;
	}
	cells.push(current.trim());
	return cells;
}

function cellAlign(cell: string): TableAlign {
	const trimmed = cell.trim();
	if (!/^:?-+:?$/.test(trimmed)) return null;
	const left = trimmed.startsWith(':');
	const right = trimmed.endsWith(':');
	if (left && right) return 'center';
	if (right) return 'right';
	if (left) return 'left';
	return null;
}

/** Reads the marks that travel inside a line: code spans, bold, italics, strikethrough and
 *  links. */
export function parseInline(source: string): Inline[] {
	const nodes: Inline[] = [];
	let buffer = '';
	let index = 0;

	const flush = () => {
		if (buffer === '') return;
		pushText(nodes, buffer);
		buffer = '';
	};

	while (index < source.length) {
		const char = source[index];

		if (
			char === '\\' &&
			index + 1 < source.length &&
			/[\\`*_{}[\]()#+\-.!>~|]/.test(source[index + 1])
		) {
			buffer += source[index + 1];
			index += 2;
			continue;
		}

		if (char === '\n') {
			flush();
			nodes.push({ kind: 'break' });
			index += 1;
			continue;
		}

		if (char === '`') {
			const run = runLength(source, index, '`');
			const marker = '`'.repeat(run);
			const close = source.indexOf(marker, index + run);
			if (close !== -1) {
				const raw = source.slice(index + run, close);
				flush();
				nodes.push({ kind: 'code', text: raw.replace(/^ (.*) $/s, '$1') });
				index = close + run;
				continue;
			}
		}

		if (char === '*' || char === '_') {
			const run = runLength(source, index, char);
			const double = run >= 2;
			const marker = char.repeat(double ? 2 : 1);
			const after = source[index + marker.length];
			const opens =
				after !== undefined && !/\s/.test(after) && (double || isWordEdge(source, index));
			if (opens) {
				const close = findClose(source, marker, index + marker.length);
				if (close !== -1) {
					flush();
					nodes.push({
						kind: double ? 'strong' : 'em',
						children: parseInline(source.slice(index + marker.length, close))
					});
					index = close + marker.length;
					continue;
				}
			}
		}

		if (char === '~' && source.startsWith('~~', index)) {
			const close = source.indexOf('~~', index + 2);
			if (close > index + 2) {
				flush();
				nodes.push({ kind: 'del', children: parseInline(source.slice(index + 2, close)) });
				index = close + 2;
				continue;
			}
		}

		if (char === '[' || (char === '!' && source[index + 1] === '[')) {
			const image = char === '!';
			const open = image ? index + 1 : index;
			const label = matchBracket(source, open);
			if (label !== -1 && source[label + 1] === '(') {
				const end = matchParen(source, label + 1);
				if (end !== -1) {
					const href = safeHref(destination(source.slice(label + 2, end)));
					if (href !== null) {
						const text = source.slice(open + 1, label);
						flush();
						nodes.push({
							kind: 'link',
							href,
							// An image has only its description to read, so the link
							// it becomes is labelled with that instead of showing one.
							children: image ? [{ kind: 'text', text: plainText(text) }] : parseInline(text)
						});
						index = end + 1;
						continue;
					}
				}
			}
		}

		if (
			char === 'h' &&
			(source.startsWith('http://', index) || source.startsWith('https://', index))
		) {
			const url = bareUrl(source.slice(index));
			if (url !== '') {
				flush();
				nodes.push({ kind: 'link', href: url, children: [{ kind: 'text', text: url }] });
				index += url.length;
				continue;
			}
		}

		buffer += char;
		index += 1;
	}

	flush();
	return nodes;
}

function pushText(nodes: Inline[], text: string): void {
	const last = nodes[nodes.length - 1];
	if (last !== undefined && last.kind === 'text') {
		last.text += text;
		return;
	}
	nodes.push({ kind: 'text', text });
}

function runLength(text: string, index: number, char: string): number {
	let run = 0;
	while (text[index + run] === char) run += 1;
	return run;
}

function findClose(source: string, marker: string, from: number): number {
	let index = from;
	while (index < source.length) {
		const found = source.indexOf(marker, index);
		if (found === -1) return -1;
		const before = source[found - 1];
		if (found === from || /\s/.test(before)) {
			index = found + marker.length;
			continue;
		}
		return found;
	}
	return -1;
}

function isWordEdge(source: string, index: number): boolean {
	if (source[index] !== '_') return true;
	const before = source[index - 1];
	const after = source[index + 1];
	if (after === undefined || /\s/.test(after)) return false;
	return before === undefined || !/[\w]/.test(before);
}

function matchBracket(source: string, open: number): number {
	let depth = 0;
	for (let index = open; index < source.length; index += 1) {
		const char = source[index];
		if (char === '\\') {
			index += 1;
			continue;
		}
		if (char === '[') depth += 1;
		else if (char === ']') {
			depth -= 1;
			if (depth === 0) return index;
		}
	}
	return -1;
}

function matchParen(source: string, open: number): number {
	let depth = 0;
	for (let index = open; index < source.length; index += 1) {
		const char = source[index];
		if (char === '\\') {
			index += 1;
			continue;
		}
		if (char === '(') depth += 1;
		else if (char === ')') {
			depth -= 1;
			if (depth === 0) return index;
		}
	}
	return -1;
}

function destination(inner: string): string {
	const text = inner.trim();
	if (text.startsWith('<') && text.includes('>')) return text.slice(1, text.indexOf('>'));
	const space = text.search(/\s/);
	return space === -1 ? text : text.slice(0, space);
}

/** Answers the addresses a reply may be turned into a link for.
 *  A scheme outside the three is left as the text it was written as. */
export function safeHref(href: string): string | null {
	const trimmed = href.trim();
	if (trimmed === '') return null;
	const scheme = /^([a-zA-Z][a-zA-Z0-9+.-]*):/.exec(trimmed);
	if (scheme === null) return null;
	const name = scheme[1].toLowerCase();
	if (name !== 'http' && name !== 'https' && name !== 'mailto') return null;
	return trimmed;
}

function bareUrl(text: string): string {
	let end = 0;
	while (end < text.length && !/[\s<>"'`]/.test(text[end])) end += 1;
	let url = text.slice(0, end).replace(/[.,;:!?]+$/, '');
	while (url.endsWith(')') && countOf(url, ')') > countOf(url, '(')) url = url.slice(0, -1);
	return url;
}

function countOf(text: string, char: string): number {
	let count = 0;
	for (const candidate of text) if (candidate === char) count += 1;
	return count;
}

function plainText(source: string): string {
	return source.replace(/\\([\\`*_{}[\]()#+\-.!>~|])/g, '$1');
}

function indentOf(line: string): number {
	const match = /^\s*/.exec(line);
	return match === null ? 0 : match[0].length;
}

function stripIndent(line: string, indent: number): string {
	return line.slice(Math.min(indent, indentOf(line)));
}

function isMarker(line: string): boolean {
	return BULLET.test(line) || NUMBERED.test(line);
}
