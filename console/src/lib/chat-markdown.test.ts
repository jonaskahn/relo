import { describe, expect, it } from 'vitest';

import { parseInline, parseMarkdown, safeHref, type Block, type Inline } from './chat-markdown';

// Flattens a run of inline nodes back to the words a reader sees, so an
// assertion about the text reads as what a reader would read.
function textOf(nodes: Inline[]): string {
	return nodes
		.map((node) => {
			switch (node.kind) {
				case 'text':
					return node.text;
				case 'break':
					return '\n';
				case 'code':
					return node.text;
				default:
					return textOf(node.children);
			}
		})
		.join('');
}

function first(blocks: Block[]): Block {
	expect(blocks.length).toBeGreaterThan(0);
	return blocks[0];
}

describe('the blocks a reply is read as', () => {
	it('keeps paragraphs, and reads a line break inside one as a break', () => {
		const blocks = parseMarkdown('one\ntwo\n\nthree');
		expect(blocks).toHaveLength(2);
		expect(first(blocks)).toMatchObject({ kind: 'paragraph' });
		expect(textOf((first(blocks) as { children: Inline[] }).children)).toBe('one\ntwo');
		expect(textOf((blocks[1] as { children: Inline[] }).children)).toBe('three');
	});

	it('reads the level of a heading', () => {
		const heading = first(parseMarkdown('## Overview'));
		expect(heading).toMatchObject({ kind: 'heading', level: 2 });
		expect(textOf((heading as { children: Inline[] }).children)).toBe('Overview');
	});

	it('keeps what is inside a fence as written', () => {
		const block = first(parseMarkdown('```ts\nconst a = *b*;\n```'));
		expect(block).toEqual({ kind: 'code', language: 'ts', text: 'const a = *b*;' });
	});

	it('runs a fence that never closes to the end of the reply', () => {
		const block = first(parseMarkdown('```\nlet x = 1;\nlet y = 2;'));
		expect(block).toEqual({ kind: 'code', language: '', text: 'let x = 1;\nlet y = 2;' });
	});

	it('nests a list inside the item that carries it', () => {
		const list = first(parseMarkdown('- one\n  - nested\n- two'));
		expect(list.kind).toBe('list');
		if (list.kind !== 'list') return;
		expect(list.ordered).toBe(false);
		expect(list.start).toBe(1);
		expect(list.items).toHaveLength(2);
		expect(list.items[0].map((block) => block.kind)).toEqual(['paragraph', 'list']);
		const nested = list.items[0][1];
		if (nested.kind !== 'list') return;
		expect(textOf((nested.items[0][0] as { children: Inline[] }).children)).toBe('nested');
		expect(textOf((list.items[1][0] as { children: Inline[] }).children)).toBe('two');
	});

	it('keeps the number a numbered list starts at', () => {
		const list = first(parseMarkdown('3. three\n4. four'));
		expect(list).toMatchObject({ kind: 'list', ordered: true, start: 3 });
	});

	it('reads a table and the alignment of its columns', () => {
		const table = first(parseMarkdown('| a | b |\n| :-- | --: |\n| 1 | 2 |'));
		expect(table).toMatchObject({ kind: 'table', align: ['left', 'right'] });
		if (table.kind !== 'table') return;
		expect(table.header.map(textOf)).toEqual(['a', 'b']);
		expect(table.rows).toHaveLength(1);
		expect(table.rows[0].map(textOf)).toEqual(['1', '2']);
	});

	it('reads a quote and a divider', () => {
		const blocks = parseMarkdown('> quoted\n\n---');
		expect(blocks).toHaveLength(2);
		expect(first(blocks)).toMatchObject({ kind: 'quote' });
		expect(blocks[1]).toEqual({ kind: 'divider' });
	});

	it('leaves a task list and an indented line as plain text', () => {
		const list = first(parseMarkdown('- [ ] todo\n\n    indented'));
		expect(list.kind).toBe('list');
		if (list.kind !== 'list') return;
		expect(list.items[0].map((block) => block.kind)).toEqual(['paragraph', 'paragraph']);
		expect(textOf((list.items[0][0] as { children: Inline[] }).children)).toBe('[ ] todo');
		expect(textOf((list.items[0][1] as { children: Inline[] }).children)).toBe('indented');
	});
});

describe('the marks inside a line', () => {
	it('reads bold, and italics inside it', () => {
		const nodes = parseInline('**bold _both_**');
		expect(nodes).toHaveLength(1);
		expect(nodes[0]).toMatchObject({ kind: 'strong' });
		if (nodes[0].kind !== 'strong') return;
		expect(textOf(nodes[0].children)).toBe('bold both');
		expect(nodes[0].children[1]).toMatchObject({ kind: 'em' });
	});

	it('keeps a code span as it was written', () => {
		const nodes = parseInline('`a *b*`');
		expect(nodes).toEqual([{ kind: 'code', text: 'a *b*' }]);
	});

	it('keeps an underscore inside a word a name', () => {
		const nodes = parseInline('file_name and _em_');
		expect(textOf(nodes)).toBe('file_name and em');
		expect(nodes.some((node) => node.kind === 'em')).toBe(true);
	});

	it('leaves arithmetic alone', () => {
		expect(parseInline('2 * 3 * 4')).toEqual([{ kind: 'text', text: '2 * 3 * 4' }]);
	});

	it('reads strikethrough', () => {
		expect(parseInline('~~gone~~')).toEqual([
			{ kind: 'del', children: [{ kind: 'text', text: 'gone' }] }
		]);
	});

	it('turns a safe address into a link', () => {
		const nodes = parseInline('[docs](https://example.com/guide)');
		expect(nodes).toEqual([
			{
				kind: 'link',
				href: 'https://example.com/guide',
				children: [{ kind: 'text', text: 'docs' }]
			}
		]);
	});

	it('leaves an address that is not one of the three schemes as text', () => {
		const nodes = parseInline('[x](javascript:alert(1))');
		expect(nodes.every((node) => node.kind === 'text')).toBe(true);
		expect(textOf(nodes)).toBe('[x](javascript:alert(1))');
	});

	it('reads a bare address without the full stop a sentence adds', () => {
		const nodes = parseInline('see https://example.com.');
		expect(nodes[1]).toMatchObject({ kind: 'link', href: 'https://example.com' });
		expect(textOf(nodes)).toBe('see https://example.com.');
	});

	it('shows markup a model wrote as the text it is', () => {
		const nodes = parseInline('<img src=x onerror=alert(1)>');
		expect(nodes).toEqual([{ kind: 'text', text: '<img src=x onerror=alert(1)>' }]);
	});

	it('keeps the closing bracket that belongs to the address', () => {
		// The bracket count is what tells a sentence's full stop from the end
		// of a path, so a link wrapped in them keeps both.
		const nodes = parseInline('see (https://example.com/a_(b))');
		expect(nodes.some((node) => node.kind === 'link')).toBe(true);
	});

	it('drops the brackets a sentence added around an address', () => {
		const nodes = parseInline('see https://example.com/a_(b).');
		const link = nodes.find((node) => node.kind === 'link');
		expect(link).toMatchObject({ href: 'https://example.com/a_(b)' });
	});
});

describe('safeHref', () => {
	it('accepts the three schemes a reply may link to', () => {
		expect(safeHref('https://example.com')).toBe('https://example.com');
		expect(safeHref('http://example.com')).toBe('http://example.com');
		expect(safeHref('mailto:someone@example.com')).toBe('mailto:someone@example.com');
		expect(safeHref('  https://example.com  ')).toBe('https://example.com');
	});

	it('refuses every other scheme, so a reply cannot run script', () => {
		for (const href of [
			'javascript:alert(1)',
			'data:text/html,<script>alert(1)</script>',
			'vbscript:msgbox(1)',
			'file:///etc/passwd'
		]) {
			expect(safeHref(href), href).toBeNull();
		}
	});

	it('refuses a target that names no scheme', () => {
		expect(safeHref('/relative/path')).toBeNull();
		expect(safeHref('example.com')).toBeNull();
		expect(safeHref('')).toBeNull();
		expect(safeHref('   ')).toBeNull();
	});

	it('reads the scheme whatever case it was written in', () => {
		expect(safeHref('HTTPS://example.com')).toBe('HTTPS://example.com');
	});
});
