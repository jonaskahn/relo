import { describe, expect, it } from 'vitest';

import { fileSize, notesText } from './update-notes';

describe('notesText', () => {
	it('turns a heading and its bullets into readable lines', () => {
		expect(notesText("## What's Changed\n* One change\n* Another change\n")).toBe(
			"What's Changed\n• One change\n• Another change"
		);
	});

	it('unwraps links and emphasis', () => {
		expect(notesText('**Full Changelog**: [compare](https://example.com/a?b=1)')).toBe(
			'Full Changelog: compare (https://example.com/a?b=1)'
		);
	});

	it('keeps markup as text', () => {
		expect(notesText('<script>alert(1)</script>')).toBe('<script>alert(1)</script>');
	});

	it('leaves one blank line between paragraphs', () => {
		expect(notesText('one\n\n\n\ntwo')).toBe('one\n\ntwo');
	});

	it('is empty for empty notes', () => {
		expect(notesText('')).toBe('');
	});
});

describe('fileSize', () => {
	it('reads a megabyte count', () => {
		expect(fileSize(18_964_455)).toBe('18 MB');
	});

	it('keeps one decimal below ten', () => {
		expect(fileSize(4_400_000)).toBe('4.2 MB');
	});

	it('reads a small file in bytes', () => {
		expect(fileSize(582)).toBe('582 B');
	});

	it('is empty without a size', () => {
		expect(fileSize(undefined)).toBe('');
		expect(fileSize(0)).toBe('');
	});
});
