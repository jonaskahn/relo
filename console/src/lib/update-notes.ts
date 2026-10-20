/** A release's changelog as text the dialog shows without rendering HTML. */
export function notesText(markdown: string): string {
	return markdown
		.replace(/\r\n/g, '\n')
		.replace(/\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)/g, '$1 ($2)')
		.replace(/\*\*(.+?)\*\*/g, '$1')
		.replace(/`([^`]+)`/g, '$1')
		.replace(/^#{1,6}\s+/gm, '')
		.replace(/^\s*[*+-]\s+/gm, '• ')
		.replace(/\n{3,}/g, '\n\n')
		.trim();
}

/** Formats a byte count the way a download list reads it. */
export function fileSize(bytes: number | undefined): string {
	if (!bytes || bytes <= 0) return '';
	const units = ['B', 'KB', 'MB', 'GB'];
	let value = bytes;
	let unit = 0;
	while (value >= 1024 && unit < units.length - 1) {
		value /= 1024;
		unit += 1;
	}
	const shown = value < 10 && unit > 0 ? value.toFixed(1) : String(Math.round(value));
	return `${shown} ${units[unit]}`;
}
