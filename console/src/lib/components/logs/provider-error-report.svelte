<script lang="ts">
	import { t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';
	import { acceptLanguage, api } from '$lib/api';
	import {
		buildIssueReport,
		environmentText,
		openIssueTab,
		navigateIssue,
		redactSecrets
	} from '$lib/report-issue';
	import type { ProviderReport } from '$lib/provider-report';
	import type { UsageLogRow } from '$lib/types';
	import Button from '$lib/components/ui/button.svelte';
	import Icon from '$lib/components/ui/icon.svelte';

	let { row }: { row: UsageLogRow } = $props();
	let working = $state(false);

	async function report() {
		if (working) return;
		working = true;
		const popup = openIssueTab();
		let resolveText!: (text: string) => void;
		let rejectText!: (error: unknown) => void;
		const text = new Promise<string>((resolve, reject) => {
			resolveText = resolve;
			rejectText = reject;
		});
		// Safari requires the clipboard write itself to start inside the click gesture.
		let copied: Promise<boolean>;
		try {
			copied = navigator.clipboard
				.write([
					new ClipboardItem({
						'text/plain': text.then((value) => new Blob([value], { type: 'text/plain' }))
					})
				])
				.then(
					() => true,
					() => false
				);
		} catch {
			copied = Promise.resolve(false);
		}
		void text.catch(() => {});
		try {
			const bundle = await api<ProviderReport>('/activity/requests/' + row.ID + '/provider-report');
			const clean = redactSecrets(JSON.stringify(bundle, null, 2));
			resolveText(clean);
			download(clean, bundle.request_id);
			const first = bundle.attempts[0];
			const issue = buildIssueReport({
				summary:
					'Provider error ' + first.status + ' from ' + first.provider + ' (' + first.model + ')',
				whatHappened: [
					'Request ID: ' + bundle.request_id,
					'Surface: ' + row.Surface,
					'Route: ' + (row.RouteProvider || row.Provider),
					'Attempts: ' + row.Attempts,
					'Duration: ' + row.DurationMs + ' ms'
				].join('\n'),
				environment: environmentText({
					version: bundle.relo_version,
					platform: navigator.platform,
					userAgent: navigator.userAgent,
					pageUrl: window.location.href,
					locale: acceptLanguage()
				}),
				diagnostics:
					'The masked provider exchange was downloaded. Paste the copied exchange into "Provider exchange" or attach relo-request-' +
					bundle.request_id +
					'.json.'
			});
			navigateIssue(popup, issue.url);
			toast.success(
				$t(
					'ui.pages.logsPage.' +
						((await copied) ? 'reportCopiedDownloaded' : 'reportDownloadedOnly')
				)
			);
		} catch (error) {
			rejectText(error);
			popup?.close();
			toast.error($t('ui.pages.logsPage.reportFailed'));
		} finally {
			working = false;
		}
	}

	function download(text: string, requestId: string) {
		const url = URL.createObjectURL(new Blob([text], { type: 'application/json' }));
		const anchor = document.createElement('a');
		anchor.href = url;
		anchor.download = 'relo-request-' + requestId.replace(/[^a-zA-Z0-9_-]/g, '_') + '.json';
		document.body.append(anchor);
		anchor.click();
		anchor.remove();
		setTimeout(() => URL.revokeObjectURL(url), 1000);
	}
</script>

<Button
	data-report-issue-button
	variant="outline"
	size="sm"
	disabled={working}
	title={$t('ui.pages.logsPage.reportProviderErrorHint')}
	onclick={() => void report()}
>
	<Icon name="brand-github" size={14} />
	{$t('ui.pages.logsPage.reportProviderError')}
</Button>
