import { describe, expect, it } from 'vitest';

import {
	buildIssueReport,
	environmentText,
	ISSUE_URL_LIMIT,
	issueURL,
	redactSecrets
} from './report-issue';

const environment = {
	version: '0.2.3',
	platform: 'macOS',
	userAgent: 'Test Browser',
	pageUrl: 'https://relo.test/logs',
	locale: 'en'
};
const environmentLine = environmentText(environment);

describe('GitHub issue reports', () => {
	it('fills the bug form fields and stays under the address cap', () => {
		const report = buildIssueReport({
			summary: 'Request failed',
			whatHappened: 'The upstream refused the request.',
			environment: environmentLine,
			diagnostics: 'status=429 model=gpt-5'
		});

		expect(report.url).toContain('template=bug_report.yml');
		expect(report.url).toContain('summary=Request+failed');
		expect(report.url).toContain('diagnostics=status%3D429+model%3Dgpt-5');
		expect(report.url.length).toBeLessThanOrEqual(ISSUE_URL_LIMIT);
	});

	it('shrinks an oversized report without an attachment', () => {
		const report = buildIssueReport({
			summary: 'Request failed',
			whatHappened: 'A long daemon log did not fit.',
			environment: environmentLine,
			diagnostics: 'x'.repeat(12000)
		});

		expect(report.url).not.toContain('ZIP');
		expect(report.url.length).toBeLessThanOrEqual(ISSUE_URL_LIMIT);
	});

	it('redacts every supported secret shape before a URL is built', () => {
		const raw = [
			'sk-abcdefghijklmnop',
			'Bearer abc.def.ghi',
			'relo_abcdefghijklmnopqrstu',
			'ghp_12345678901234567890',
			'github_pat_12345678901234567890',
			'AKIA1234567890123456',
			'AIza' + 'A'.repeat(35),
			'xoxb-1234567890-abcdef',
			'eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.signature'
		].join(' ');
		const clean = redactSecrets(raw);
		expect(clean).not.toMatch(/sk-|Bearer|relo_|ghp_|github_pat_|AKIA|AIza|xoxb-|eyJ/);
		expect(clean.match(/\[REDACTED\]/g)?.length).toBe(9);
		const report = buildIssueReport({
			summary: raw,
			whatHappened: raw,
			environment: environmentLine,
			diagnostics: issueURL('bug_report.yml', {
				summary: raw,
				whatHappened: raw,
				environment: environmentLine,
				diagnostics: raw
			})
		});
		expect(report.url).not.toContain('sk-abcdefghijklmnop');
	});
});

describe('support forms', () => {
	it.each(['agent_harness_request.yml', 'provider_request.yml'])(
		'prefills only environment in %s',
		(template) => {
			const url = new URL(issueURL(template, { environment: environmentLine }));
			expect(url.searchParams.get('template')).toBe(template);
			expect(url.searchParams.get('environment')).toBe(environmentLine);
			expect(url.searchParams.has('summary')).toBe(false);
		}
	);
	it('fits all fields even when every value is oversized Unicode', () => {
		const url = issueURL('bug_report.yml', {
			summary: '界'.repeat(10000),
			expected: '界'.repeat(10000),
			repro: '界'.repeat(10000)
		});
		expect(url.length).toBeLessThanOrEqual(ISSUE_URL_LIMIT);
	});
});
