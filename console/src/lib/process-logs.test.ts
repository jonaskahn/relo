import { describe, expect, test } from 'vitest';

import {
	buildDaemonQuery,
	buildRequestQuery,
	computeRequestKpis,
	countRequestFilters,
	daemonLevelTone,
	formatHourGroup,
	groupDaemonLines,
	groupRequestsByHour,
	isDaemonLevel,
	isRequestOrigin,
	isRequestStatus,
	mergeDaemonLines,
	parseLogSection,
	startupOutcomeTone,
	type DaemonLine,
	type RequestFilters
} from './process-logs';
import type { UsageLogRow } from './types';

function line(offset: number) {
	return { offset, timestamp_ms: offset * 1000, level: 'info', message: 'relo serving' };
}

describe('parseLogSection', () => {
	test('keeps the request log for anything it does not name', () => {
		expect(parseLogSection(null)).toBe('agent');
		expect(parseLogSection('agent')).toBe('agent');
		expect(parseLogSection('overview')).toBe('agent');
	});

	test('names the daemon and startup sections', () => {
		expect(parseLogSection('daemon')).toBe('daemon');
		expect(parseLogSection('startup')).toBe('startup');
	});
});

describe('buildDaemonQuery', () => {
	test('encodes a first read with no cursors', () => {
		expect(buildDaemonQuery({ limit: 200, level: 'all', hideRequests: true })).toBe(
			'limit=200&hide_requests=1'
		);
	});

	test('carries the paging and live cursors with a level floor', () => {
		expect(
			buildDaemonQuery({ limit: 50, before: '412', level: 'warning', hideRequests: false })
		).toBe('limit=50&before=412&level=warning');
		expect(buildDaemonQuery({ limit: 200, after: '671', level: 'error', hideRequests: true })).toBe(
			'limit=200&after=671&level=error&hide_requests=1'
		);
	});
});

const openLog: RequestFilters = {
	provider: '',
	status: 'any',
	origin: 'any',
	key: '',
	client: ''
};

describe('buildRequestQuery', () => {
	test('reads a first page with no cursor and no narrowing', () => {
		expect(buildRequestQuery(openLog)).toBe('limit=50');
		expect(buildRequestQuery(openLog, '12')).toBe('limit=50&cursor=12');
	});

	test('names a status class rather than one code', () => {
		expect(buildRequestQuery({ ...openLog, status: 'ok' })).toBe('limit=50&status=2xx');
		expect(buildRequestQuery({ ...openLog, status: 'error' })).toBe('limit=50&status=4xx%2C5xx');
	});

	test('carries the origin, the access key and the coding client it was issued for', () => {
		expect(buildRequestQuery({ ...openLog, origin: 'internal' })).toBe('limit=50&origin=internal');
		expect(buildRequestQuery({ ...openLog, origin: 'external', provider: 'openai' })).toBe(
			'limit=50&provider=openai&origin=external'
		);
		expect(buildRequestQuery({ ...openLog, key: 'k1', client: 'codex' })).toBe(
			'limit=50&client=k1&client_app=codex'
		);
	});
});

describe('countRequestFilters', () => {
	test('counts only the filters that narrow the log', () => {
		expect(countRequestFilters(openLog)).toBe(0);
		expect(countRequestFilters({ ...openLog, status: 'ok' })).toBe(1);
		expect(
			countRequestFilters({
				provider: 'openai',
				status: 'error',
				origin: 'external',
				key: 'k1',
				client: 'codex'
			})
		).toBe(5);
	});
});

describe('mergeDaemonLines', () => {
	test('prepends a live tail without repeating a line', () => {
		const merged = mergeDaemonLines([line(30), line(20)], [line(40), line(30)]);
		expect(merged.map((entry) => entry.offset)).toEqual([40, 30, 20]);
	});

	test('holds the newest lines when a tab stays live', () => {
		const current = Array.from({ length: 5 }, (_, index) => line(index));
		const merged = mergeDaemonLines(current, [line(9)], 6);
		expect(merged.map((entry) => entry.offset)).toEqual([9, 4, 3, 2, 1, 0]);
	});
});

describe('tones', () => {
	test('a log severity reads as a word, never color alone', () => {
		expect(daemonLevelTone('info')).toBe('muted');
		expect(daemonLevelTone('warning')).toBe('warn');
		expect(daemonLevelTone('warn')).toBe('warn');
		expect(daemonLevelTone('ERROR')).toBe('fail');
		expect(daemonLevelTone('debug')).toBe('muted');
	});

	test('a failed start fails the pill, a serving one settles it', () => {
		expect(startupOutcomeTone('started')).toBe('pass');
		expect(startupOutcomeTone('failed')).toBe('fail');
	});
});

describe('the guards over a log filter choice', () => {
	test('accepts only the levels the daemon log is read at', () => {
		expect(isDaemonLevel('all')).toBe(true);
		expect(isDaemonLevel('warning')).toBe(true);
		expect(isDaemonLevel('error')).toBe(true);
		expect(isDaemonLevel('debug')).toBe(false);
	});

	test('accepts only the statuses and origins the log separates', () => {
		expect(isRequestStatus('any')).toBe(true);
		expect(isRequestStatus('ok')).toBe(true);
		expect(isRequestStatus('Any')).toBe(false);
		expect(isRequestOrigin('any')).toBe(true);
		expect(isRequestOrigin('internal')).toBe(true);
		expect(isRequestOrigin('external')).toBe(true);
		expect(isRequestOrigin('loopback')).toBe(false);
	});
});

function request(patch: Partial<UsageLogRow> = {}): UsageLogRow {
	return {
		ID: 1,
		RequestID: 'r',
		TimestampMs: 0,
		Provider: 'p',
		Model: 'm',
		RequestedModel: 'm',
		GroupID: null,
		CredentialLabel: '',
		Surface: '',
		Status: 200,
		DurationMs: 0,
		InputTokens: 0,
		OutputTokens: 0,
		CacheReadTokens: 0,
		CacheWriteTokens: 0,
		EstimatedCostMicros: null,
		RouteProvider: '',
		RouteReason: '',
		ClientKeyID: '',
		ClientKeyName: '',
		ClientApp: '',
		Origin: '',
		...patch
	};
}

describe('computeRequestKpis', () => {
	test('an empty list reports a full success rate and nothing else', () => {
		expect(computeRequestKpis([])).toEqual({
			total: 0,
			successCount: 0,
			successRate: 100,
			totalTokens: 0,
			avgDurationMs: 0,
			slowestDurationMs: 0,
			maxTokens: 0
		});
	});

	test('counts answers, tokens, and the slowest request', () => {
		const kpis = computeRequestKpis([
			request({ Status: 204, InputTokens: 10, OutputTokens: 5, DurationMs: 40 }),
			request({ Status: 500, DurationMs: 100 }),
			request({ Status: 299, InputTokens: 1, DurationMs: 10 }),
			request({ Status: 199, InputTokens: 1, DurationMs: 10 })
		]);
		expect(kpis).toEqual({
			total: 4,
			successCount: 2,
			successRate: 50,
			totalTokens: 17,
			avgDurationMs: 40,
			slowestDurationMs: 100,
			maxTokens: 15
		});
	});
});

describe('groupRequestsByHour', () => {
	test('keeps consecutive rows in one hour and starts a group when the hour changes', () => {
		const hour = Date.parse('2026-01-01T13:10:00Z');
		const later = Date.parse('2026-01-01T14:05:00Z');
		const groups = groupRequestsByHour([
			request({ TimestampMs: hour, InputTokens: 2, OutputTokens: 3 }),
			request({ TimestampMs: hour + 1000, OutputTokens: 1 }),
			request({ TimestampMs: later, InputTokens: 4 })
		]);
		expect(groups).toHaveLength(2);
		expect(groups[0].requests).toHaveLength(2);
		expect(groups[0].totalTokens).toBe(6);
		expect(groups[0].hourLabel).toBe(formatHourGroup(hour));
		expect(groups[1].requests).toHaveLength(1);
		expect(groups[1].totalTokens).toBe(4);
	});

	test('a missing timestamp has no hour label', () => {
		expect(formatHourGroup(0)).toBe('');
	});
});

describe('groupDaemonLines', () => {
	test('collapses a run of the same message and splits when the text changes', () => {
		const first: DaemonLine = { offset: 1, timestamp_ms: 30, level: 'info', message: 'up' };
		const middle: DaemonLine = { offset: 2, timestamp_ms: 10, level: 'info', message: 'up' };
		const other: DaemonLine = { offset: 3, timestamp_ms: 40, level: 'error', message: 'down' };
		const groups = groupDaemonLines([first, middle, other]);
		expect(groups).toHaveLength(2);
		expect(groups[0]).toMatchObject({ count: 2, firstMs: 10, latestMs: 30, message: 'up' });
		expect(groups[0].lines).toEqual([first, middle]);
		expect(groups[1]).toMatchObject({ count: 1, firstMs: 40, latestMs: 40, message: 'down' });
	});
});
