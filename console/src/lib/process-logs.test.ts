import { describe, expect, test } from 'vitest';

import {
	buildDaemonQuery,
	buildRequestQuery,
	countRequestFilters,
	daemonLevelTone,
	isDaemonLevel,
	isRequestOrigin,
	isRequestStatus,
	mergeDaemonLines,
	parseLogSection,
	startupOutcomeTone,
	type RequestFilters
} from './process-logs';

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
		expect(daemonLevelTone('error')).toBe('fail');
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
