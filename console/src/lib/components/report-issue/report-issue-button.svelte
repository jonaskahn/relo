<script lang="ts">
	import { t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';

	import { acceptLanguage, api } from '$lib/api';
	import { buildIssueReport, diagnosticsText, environmentText } from '$lib/report-issue';
	import type { DaemonLine } from '$lib/process-logs';
	import type { StatusResponse } from '$lib/types';
	import Button from '$lib/components/ui/button.svelte';
	import Icon from '$lib/components/ui/icon.svelte';

	interface Props {
		summary: string;
		details: string;
		includeLog?: boolean;
		topbar?: boolean;
	}

	let { summary, details, includeLog = false, topbar = false }: Props = $props();
	let working = $state(false);

	const labelKey = $derived(topbar ? 'ui.topbar.reportIssue' : 'ui.pages.logsPage.reportIssue');
	const hintKey = $derived(
		topbar ? 'ui.topbar.reportIssueHint' : 'ui.pages.logsPage.reportIssueHint'
	);

	async function report() {
		if (working) return;
		working = true;
		toast.info($t('ui.pages.logsPage.reportPreparing'));
		const popup = window.open('', '_blank', 'width=960,height=760');
		if (popup) popup.opener = null;
		try {
			const status = await api<StatusResponse>('/status').catch(() => null);
			const daemonLog = includeLog ? await readDaemonLog() : '';
			const environment = environmentText({
				version: status?.version ?? '',
				platform: navigator.platform,
				userAgent: navigator.userAgent,
				pageUrl: window.location.href,
				locale: acceptLanguage()
			});
			const issue = buildIssueReport({
				summary,
				whatHappened: details,
				environment,
				diagnostics: diagnosticsText(details, daemonLog)
			});
			if (issue.mode === 'zip') {
				downloadDaemonLog();
				toast.success($t('ui.pages.logsPage.reportBundleDownloaded'));
			}
			navigate(popup, issue.url);
		} catch {
			popup?.close();
			toast.error($t('ui.pages.logsPage.reportFailed'));
		} finally {
			working = false;
		}
	}

	async function readDaemonLog(): Promise<string> {
		const page = await api<{ items: DaemonLine[] }>('/logs/daemon?limit=500');
		return (page.items ?? [])
			.map((line) => [line.level, line.message, line.detail].filter(Boolean).join(' '))
			.join('\n');
	}

	function downloadDaemonLog() {
		const anchor = document.createElement('a');
		anchor.href = '/api/v1/logs/daemon/export';
		anchor.style.display = 'none';
		document.body.append(anchor);
		anchor.click();
		anchor.remove();
	}

	function navigate(popup: Window | null, url: string) {
		if (popup && !popup.closed) {
			popup.location.assign(url);
			popup.focus();
			return;
		}
		try {
			window.location.assign(url);
		} catch {
			toast.error($t('ui.pages.logsPage.reportPopupBlocked'));
		}
	}
</script>

<Button
	data-report-issue-button
	variant="outline"
	size={topbar ? 'default' : 'sm'}
	class={topbar ? 'max-sm:size-icon' : ''}
	aria-label={$t(labelKey)}
	title={$t(hintKey)}
	disabled={working}
	onclick={() => void report()}
>
	<Icon name="brand-github" size={topbar ? 17 : 14} />
	<span class={topbar ? 'hidden sm:inline' : ''}>{$t(labelKey)}</span>
</Button>
