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
		expect(report.mode).toBe('inline');
		expect(report.url).toContain('template=bug_report.yml');
		expect(report.url).toContain('summary=Request+failed');
		expect(report.url).toContain('diagnostics=status%3D429+model%3Dgpt-5');
		expect(report.url.length).toBeLessThanOrEqual(ISSUE_URL_LIMIT);
	});

	it('turns an oversized report into a ZIP attachment note', () => {
		const report = buildIssueReport({
			summary: 'Request failed',
			whatHappened: 'A long daemon log did not fit.',
			environment: environmentLine,
			diagnostics: 'x'.repeat(12000)
		});
		expect(report.mode).toBe('zip');
		expect(report.diagnostics).toContain('relo-daemon-log ZIP');
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
			diagnostics: issueURL({
				summary: raw,
				whatHappened: raw,
				environment: environmentLine,
				diagnostics: raw
			})
		});
		expect(report.url).not.toContain('sk-abcdefghijklmnop');
	});
});
