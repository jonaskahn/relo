// Captions on usage surfaces: the account or group name, then the provider
// in parentheses. The id stays off the label so two rows of one connection
// read as "work (ChatGPT)" rather than a hex prefix.

/** "name (provider)" when both are known. A missing name falls back to the stored id, which is
 *  what a deleted row still has. An empty key is a dash. */
export function namedCaption(id: string, name?: string, provider?: string): string {
	if (id === '') return '—';
	const label = name?.trim() ?? '';
	if (label === '') return id;
	const host = provider?.trim() ?? '';
	return host === '' ? label : label + ' (' + host + ')';
}

/** Names one account the same way a group is named. */
export function accountCaption(id: string, label?: string, provider?: string): string {
	return namedCaption(id, label, provider);
}
