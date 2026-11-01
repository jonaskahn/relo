// GitHub issue report construction: every text is redacted before it reaches a URL, and a
// report too large for the browser's safe limit becomes an attachment note instead.

export const ISSUE_URL_LIMIT = 7000;
export const GITHUB_ISSUE_BASE =
	'https://github.com/jonaskahn/relo/issues/new?template=bug_report.yml';

const ATTACHMENT_NOTE =
	'The daemon log is too large to paste into this form. The relo-daemon-log ZIP download started beside this tab; attach it to this issue.';

const SECRET_PATTERNS: readonly RegExp[] = [
	/sk-[A-Za-z0-9_-]{16,}/g,
	/Bearer\s+[A-Za-z0-9._~+\-=]*/gi,
	/relo_[A-Za-z0-9_-]{16,}/g,
	/gh[pousr]_[A-Za-z0-9]{20,}/g,
	/github_pat_[A-Za-z0-9_]{20,}/g,
	/AKIA[0-9A-Z]{16}/g,
	/AIza[0-9A-Za-z_-]{35}/g,
	/xox[baprs]-[A-Za-z0-9-]{10,}/g,
	/eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/g
];

/** What one browser needs to describe the install the report came from. */
export interface IssueEnvironment {
	version: string;
	platform: string;
	userAgent: string;
	pageUrl: string;
	locale: string;
}

/** The issue-form fields one report fills. */
export interface IssueDraft {
	summary: string;
	whatHappened: string;
	expected?: string;
	repro?: string;
	environment: string;
	diagnostics: string;
}

/** A report after redaction, with the GitHub address a browser opens and whether it needs the
 *  daemon-log ZIP beside it. */
export interface IssueReport {
	mode: 'inline' | 'zip';
	url: string;
	summary: string;
	whatHappened: string;
	environment: string;
	diagnostics: string;
}

/** Replaces the credential shapes a log or error may carry before any report text is built. */
export function redactSecrets(text: string): string {
	let clean = text;
	for (const pattern of SECRET_PATTERNS) clean = clean.replace(pattern, '[REDACTED]');
	return clean;
}

/** Renders the install facts the GitHub form's environment field carries. */
export function environmentText(environment: IssueEnvironment): string {
	return [
		`Relo version: ${environment.version || 'unknown'}`,
		`OS/platform: ${environment.platform || 'unknown'}`,
		`Browser: ${environment.userAgent || 'unknown'}`,
		`Console page: ${environment.pageUrl || 'unknown'}`,
		`Locale: ${environment.locale || 'en'}`
	].join('\n');
}

/** Joins the context a caller knows with the daemon log, keeping each value on its own line. */
export function diagnosticsText(context: string, daemonLog: string): string {
	return [redactSecrets(context.trim()), redactSecrets(daemonLog.trim())]
		.filter((part) => part !== '')
		.join('\n\n');
}

/** Builds the GitHub bug-form address for one redacted report. */
export function issueURL(draft: IssueDraft): string {
	const params = new URLSearchParams({
		template: 'bug_report.yml',
		summary: draft.summary,
		'what-happened': draft.whatHappened,
		expected: draft.expected ?? '',
		repro: draft.repro ?? '',
		environment: draft.environment,
		diagnostics: draft.diagnostics
	});
	return GITHUB_ISSUE_BASE + '&' + params.toString();
}

/** Builds the final report: inline when the complete log fits GitHub's safe address length,
 *  otherwise with a short excerpt and the ZIP attachment instruction. */
export function buildIssueReport(input: IssueDraft): IssueReport {
	const safe = sanitizeDraft(input);
	const inlineURL = issueURL(safe);
	if (inlineURL.length <= ISSUE_URL_LIMIT) {
		return reportFrom(safe, inlineURL, 'inline');
	}
	const excerptLength = 1200;
	const excerpt =
		safe.diagnostics.length <= excerptLength
			? safe.diagnostics
			: '…' + safe.diagnostics.slice(-excerptLength);
	const zip = fitIssueDraft({ ...safe, diagnostics: excerpt + '\n\n' + ATTACHMENT_NOTE });
	return reportFrom(zip, issueURL(zip), 'zip');
}

function sanitizeDraft(input: IssueDraft): IssueDraft {
	return {
		summary: redactSecrets(input.summary).trim(),
		whatHappened: redactSecrets(input.whatHappened).trim(),
		expected: input.expected ? redactSecrets(input.expected).trim() : undefined,
		repro: input.repro ? redactSecrets(input.repro).trim() : undefined,
		environment: redactSecrets(input.environment),
		diagnostics: redactSecrets(input.diagnostics).trim()
	};
}

function reportFrom(draft: IssueDraft, url: string, mode: IssueReport['mode']): IssueReport {
	return {
		mode,
		url,
		summary: draft.summary,
		whatHappened: draft.whatHappened,
		environment: draft.environment,
		diagnostics: draft.diagnostics
	};
}

// The encoded address is the only exact length GitHub sees. Shrink the diagnostics first and
// then the prose, so a pathological install description still leaves a usable issue form.
function fitIssueDraft(draft: IssueDraft): IssueDraft {
	let next = draft;
	while (issueURL(next).length > ISSUE_URL_LIMIT && next.diagnostics.length > 200) {
		next = { ...next, diagnostics: shrink(next.diagnostics) };
	}
	while (issueURL(next).length > ISSUE_URL_LIMIT && next.whatHappened.length > 120) {
		next = { ...next, whatHappened: shrink(next.whatHappened) };
	}
	while (issueURL(next).length > ISSUE_URL_LIMIT && next.environment.length > 200) {
		next = { ...next, environment: shrink(next.environment) };
	}
	return next;
}

function shrink(text: string): string {
	const kept = Math.max(0, Math.floor(text.length * 0.7));
	return '…' + text.slice(-kept);
}
