// Rendering helpers for the data the cockpit surfaces: durations, byte
// counts, token counts, micro-dollar costs, and millisecond timestamps.

import { get } from 'svelte/store';
import { t } from 'svelte-i18n';

/** Reads a millisecond duration the way a log row shows it: seconds under a minute, minutes
 *  under an hour, hours beyond. */
export function formatDuration(ms: number): string {
	if (ms < 1000) return ms + ' ms';
	const seconds = ms / 1000;
	if (seconds < 60) return seconds.toFixed(seconds < 10 ? 1 : 0) + ' s';
	const minutes = Math.floor(seconds / 60);
	const rest = Math.round(seconds % 60);
	if (minutes < 60) return minutes + 'm ' + rest + 's';
	const hours = Math.floor(minutes / 60);
	return (hours + (minutes % 60) / 60).toFixed(1) + ' h';
}

/** Reads a daemon uptime in seconds as the status bar shows it. */
export function formatUptime(seconds: number): string {
	const days = Math.floor(seconds / 86400);
	const hours = Math.floor((seconds % 86400) / 3600);
	const minutes = Math.floor((seconds % 3600) / 60);
	if (days > 0) return days + 'd ' + hours + 'h';
	if (hours > 0) return hours + 'h ' + minutes + 'm';
	if (minutes > 0) return minutes + 'm';
	return Math.floor(seconds) + 's';
}

/** Reads a byte count in the largest unit that keeps it readable. */
export function formatBytes(bytes: number): string {
	if (bytes < 1024) return bytes + ' B';
	const units = ['KiB', 'MiB', 'GiB', 'TiB'];
	let value = bytes;
	let unit = -1;
	do {
		value /= 1024;
		unit++;
	} while (value >= 1024 && unit < units.length - 1);
	return value.toFixed(value < 10 ? 1 : 0) + ' ' + units[unit];
}

/** Abbreviates a token count with the ladder k, M, B, T, Qa, Qi.
 *  Past the largest unit the figure keeps growing rather than inventing a unit nothing knows. */
export function formatTokens(count: number): string {
	if (count < 1000) return String(count);
	if (count < 1_000_000) return (count / 1000).toFixed(count < 10_000 ? 1 : 0) + 'k';
	if (count < 1_000_000_000) return (count / 1_000_000).toFixed(count < 10_000_000 ? 2 : 1) + 'M';
	if (count < 1e12) return (count / 1e9).toFixed(1) + 'B';
	if (count < 1e15) return (count / 1e12).toFixed(1) + 'T';
	if (count < 1e18) return (count / 1e15).toFixed(1) + 'Qa';
	return (count / 1e18).toFixed(1) + 'Qi';
}

/** Renders the whole token count with its unit, which is what a tooltip reads behind an
 *  abbreviated figure. */
export function formatTokensExact(count: number): string {
	return count.toLocaleString() + ' ' + get(t)('ui.pages.usagePage.unitTokens');
}

/** Renders integer micro-dollars with enough decimals that no non-zero cost reads as free: a
 *  sub-cent spend keeps six decimals, a sub-dollar spend four, and anything above two. */
export function formatCost(micros: number | null | undefined): string {
	if (micros == null) return '—';
	if (micros === 0) return '$0.00';
	const dollars = micros / 1_000_000;
	if (Math.abs(dollars) < 0.01) return '$' + dollars.toFixed(6);
	if (Math.abs(dollars) < 1) return '$' + dollars.toFixed(4);
	return '$' + (micros / 1_000_000).toFixed(2);
}

/** Reads a per-million-token price stored in micros. */
export function formatMicroPrice(micros: number): string {
	return formatCost(micros);
}

/** Renders a rate in integer USD micros per million tokens, keeping enough digits that a cheap
 *  model is not shown as free. */
export function formatUnitPrice(micros: number | null | undefined): string {
	if (micros == null) return '—';
	const dollars = micros / 1_000_000;
	if (dollars === 0) return '$0';
	if (dollars < 0.01) return '$' + dollars.toFixed(4);
	if (dollars < 1) return '$' + dollars.toFixed(3);
	return '$' + dollars.toFixed(2);
}

/** Renders a rate in integer USD micros as the plain dollars-per-million figure an operator
 *  types back into a price field, empty when the rate is unknown. */
export function formatDollars(micros: number | null | undefined): string {
	if (micros === null || micros === undefined) return '';
	return String(micros / 1_000_000);
}

/** Reads a dollars-per-million figure into integer USD micros.
 *  An empty or unusable entry states no rate, which stays distinct from zero. */
export function parseDollars(value: string): number | null {
	const trimmed = value.trim();
	if (trimmed === '') return null;
	const parsed = Number(trimmed);
	if (!Number.isFinite(parsed) || parsed < 0) return null;
	return Math.round(parsed * 1_000_000);
}

/** Renders how long ago a fetch happened, which is what a page shows beside a catalog it did not
 *  just download. */
export function ageOf(ms: number | undefined | null): string {
	if (!ms) return get(t)('ui.pages.providersPage.header.agoJustNow');
	const diff = Math.max(0, Date.now() - ms);
	if (diff < 60_000) return get(t)('ui.pages.providersPage.header.agoJustNow');
	if (diff < 3_600_000) {
		return get(t)('ui.pages.providersPage.header.agoMinutes', {
			values: { count: Math.floor(diff / 60_000) }
		});
	}
	if (diff < 86_400_000) {
		return get(t)('ui.pages.providersPage.header.agoHours', {
			values: { count: Math.floor(diff / 3_600_000) }
		});
	}
	return get(t)('ui.pages.providersPage.header.agoDays', {
		values: { count: Math.floor(diff / 86_400_000) }
	});
}

/** Reads the short clock time a log row leads with. */
export function formatTimestamp(ms: number): string {
	return new Date(ms).toLocaleTimeString(undefined, {
		hour: '2-digit',
		minute: '2-digit',
		second: '2-digit'
	});
}

/** Reads the full date and time a detail view shows. A missing stamp is a dash. */
export function formatDateTime(ms: number | undefined | null): string {
	if (!ms) return '—';
	return new Date(ms).toLocaleString(undefined, {
		year: 'numeric',
		month: 'short',
		day: 'numeric',
		hour: '2-digit',
		minute: '2-digit'
	});
}

/** Reads the day a key or a model record was created. */
export function formatDate(ms: number | undefined | null): string {
	if (!ms) return '—';
	return new Date(ms).toLocaleDateString(undefined, {
		year: 'numeric',
		month: 'short',
		day: 'numeric'
	});
}

/** Reads a stamp as the time since it, for a column that must stay short. */
export function relativeTime(ms: number | undefined | null): string {
	if (!ms) return '—';
	const diff = Date.now() - ms;
	if (diff < 60_000) return '<1m';
	if (diff < 3_600_000) return timeAgo(Math.floor(diff / 60_000) + 'm');
	if (diff < 86_400_000) return timeAgo(Math.floor(diff / 3_600_000) + 'h');
	return timeAgo(Math.floor(diff / 86_400_000) + 'd');
}

function timeAgo(value: string): string {
	return get(t)('ui.common.timeAgo', { values: { value } });
}
