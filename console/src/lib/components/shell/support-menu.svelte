<script lang="ts">
	import { t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';
	import { DropdownMenu } from 'bits-ui';
	import { acceptLanguage, api } from '$lib/api';
	import { environmentText, issueURL, openIssueTab, navigateIssue } from '$lib/report-issue';
	import type { StatusResponse } from '$lib/types';
	import Button from '$lib/components/ui/button.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import type { IconName } from '$lib/components/ui/icon.svelte';
	import MenuPanel from '$lib/components/ui/menu-panel.svelte';

	const items: { key: string; template: string; icon: IconName }[] = [
		{ key: 'supportReportIssue', template: 'bug_report.yml', icon: 'brand-github' },
		{ key: 'supportRequestHarness', template: 'agent_harness_request.yml', icon: 'robot' },
		{ key: 'supportRequestConnection', template: 'provider_request.yml', icon: 'plug-connected' }
	];

	async function open(template: string) {
		const popup = openIssueTab();
		try {
			const status = await api<StatusResponse>('/status').catch(() => null);
			const fields: Record<string, string> = {
				environment: environmentText({
					version: status?.version ?? '',
					platform: navigator.platform,
					userAgent: navigator.userAgent,
					pageUrl: window.location.href,
					locale: acceptLanguage()
				})
			};
			if (template === 'bug_report.yml') {
				fields.summary = 'Relo console issue';
				fields.diagnostics = '';
			}
			navigateIssue(popup, issueURL(template, fields));
		} catch {
			popup?.close();
			toast.error($t('ui.pages.logsPage.reportPopupBlocked'));
		}
	}
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger>
		{#snippet child({ props })}
			<Button
				variant="outline"
				size="icon"
				{...props}
				aria-label={$t('ui.topbar.support')}
				title={$t('ui.topbar.support')}
			>
				<Icon name="lifebuoy" size={18} />
			</Button>
		{/snippet}
	</DropdownMenu.Trigger>
	<DropdownMenu.Portal>
		<MenuPanel align="end" class="surface-pop z-50 min-w-44 p-1">
			{#each items as item (item.key)}
				<DropdownMenu.Item
					class="flex min-h-control cursor-pointer items-center gap-2 rounded-control px-2.5 py-1.5 text-sm outline-none"
					onSelect={() => void open(item.template)}
				>
					<Icon name={item.icon} size={15} />
					{$t('ui.topbar.' + item.key)}
				</DropdownMenu.Item>
			{/each}
		</MenuPanel>
	</DropdownMenu.Portal>
</DropdownMenu.Root>
