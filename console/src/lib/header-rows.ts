// The headers editor holds one editable pair per row. The id names the row rather than the
// header: an operator can retype a row while it sits in the list, and a removal has to take the
// row they pointed at rather than whatever moved into its place.

/** One editable header pair as the editor holds it. */
export interface HeaderRow {
	id: number;
	name: string;
	value: string;
}

/** Opens one row per header the provider already sends, numbering each from `takeId`. */
export function headerRows(headers: Record<string, string>, takeId: () => number): HeaderRow[] {
	return Object.entries(headers).map(([name, value]) => ({ id: takeId(), name, value }));
}

/** Writes the rows back out as the headers a request sends. A row with no name is an edit in
 *  progress rather than a header, and the name is taken trimmed because that is what the daemon
 *  stores it under. */
export function headerRecord(rows: readonly HeaderRow[]): Record<string, string> {
	const out: Record<string, string> = {};
	for (const row of rows) {
		const name = row.name.trim();
		if (name === '') continue;
		out[name] = row.value;
	}
	return out;
}
