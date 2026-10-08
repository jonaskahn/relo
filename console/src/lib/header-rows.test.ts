import { describe, expect, it } from 'vitest';

import { headerRecord, headerRows, type HeaderRow } from './header-rows';

// The rows are what an operator types into, so what is under test is that the
// record a request sends follows the rows as they are edited and removed.

function rows(): HeaderRow[] {
	let next = 0;
	return headerRows({ 'x-one': '1', 'x-two': '2', 'x-three': '3' }, () => next++);
}

describe('header rows', () => {
	it('gives every row an id of its own, so a row is the row and not its position', () => {
		expect(rows().map((row) => row.id)).toEqual([0, 1, 2]);
	});

	it('keeps the other rows when the middle one is removed', () => {
		const opening = rows();
		const kept = opening.filter((row) => row.id !== opening[1].id);
		expect(headerRecord(kept)).toEqual({ 'x-one': '1', 'x-three': '3' });
	});

	it('leaves out a row the operator has not named yet', () => {
		const opening = rows();
		const edited = opening.map((row) => (row.id === 1 ? { ...row, name: '   ' } : row));
		expect(headerRecord(edited)).toEqual({ 'x-one': '1', 'x-three': '3' });
	});

	it('sends the name trimmed, because that is what it is stored under', () => {
		const opening = rows();
		const edited = opening.map((row) => (row.id === 0 ? { ...row, name: ' x-one ' } : row));
		expect(headerRecord(edited)).toEqual({ 'x-one': '1', 'x-two': '2', 'x-three': '3' });
	});
});
