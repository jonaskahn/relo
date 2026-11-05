export const ISSUE_URL_LIMIT = 7000;
export const GITHUB_ISSUE_BASE = 'https://github.com/jonaskahn/relo/issues/new';

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

/** Opens an issue form containing only redacted, URL-bounded fields. */
export function issueURL(template: string, fields: Record<string, string>): string {
	const safe = Object.fromEntries(
		Object.entries(fields).map(([key, value]) => [key, redactSecrets(value)])
	);
	const address = () => GITHUB_ISSUE_BASE + '?' + new URLSearchParams({ template, ...safe });
	while (address().length > ISSUE_URL_LIMIT) {
		const key = Object.keys(safe).sort((a, b) => safe[b].length - safe[a].length)[0];
		if (!key || !safe[key].length) break;
		safe[key] = safe[key].slice(0, Math.floor(safe[key].length * 0.7));
	}
	return address();
}

/** Fits the bug report's prose without including an attachment in its URL. */
export function buildIssueReport(draft: IssueDraft): { url: string } {
	return {
		url: issueURL('bug_report.yml', {
			summary: draft.summary,
			'what-happened': draft.whatHappened,
			expected: draft.expected ?? '',
			repro: draft.repro ?? '',
			environment: draft.environment,
			diagnostics: draft.diagnostics
		})
	};
}

/** Reserves the GitHub tab during the user's gesture, before any asynchronous reads. */
export function openIssueTab(): Window | null {
	const popup = window.open('', '_blank', 'width=960,height=760');
	if (popup) popup.opener = null;
	return popup;
}

/** Uses the current tab when the browser blocked the reserved popup. */
export function navigateIssue(popup: Window | null, url: string): void {
	if (popup && !popup.closed) {
		popup.location.assign(url);
		popup.focus();
	} else {
		window.location.assign(url);
	}
}
