<script lang="ts">
	import { t } from 'svelte-i18n';

	import Icon from '$lib/components/ui/icon.svelte';
	import type { IntegrationVerifyCheck } from '$lib/types';

	// AgentVerifyChecks reads the wiring checks one verification ran: the key
	// reaching the data plane, the login environment exporting it, the settings
	// files on disk, and the files carrying Relo's block. A check verify
	// repaired itself carries the regenerated mark rather than a failure.
	interface Props {
		checks: IntegrationVerifyCheck[];
	}

	let { checks }: Props = $props();

	const labelKeys: Record<string, string> = {
		key: 'ui.pages.integrationsPage.verifyCheckKey',
		env: 'ui.pages.integrationsPage.verifyCheckEnv',
		settings: 'ui.pages.integrationsPage.verifyCheckSettings',
		config: 'ui.pages.integrationsPage.verifyCheckConfig'
	};
</script>

<ul class="flex flex-col gap-1.5">
	{#each checks as check (check.name)}
		<li class="flex items-start gap-2 rounded-panel px-3 py-2 {check.ok ? '' : 'bg-danger/5'}">
			<Icon
				name={check.ok ? 'circle-check' : 'circle-x'}
				size={14}
				class="mt-0.5 shrink-0 {check.ok ? 'text-ok' : 'text-danger'}"
			/>
			<div class="min-w-0 flex-1">
				<p class="flex flex-wrap items-center gap-2 text-sm font-medium text-ink">
					{labelKeys[check.name] ? $t(labelKeys[check.name]) : check.name}
					{#if check.ok && check.fixed}
						<span
							class="inline-flex items-center gap-1 rounded-full bg-accent-soft px-2 py-0.5 text-[0.7rem] font-medium text-accent-ink"
						>
							<Icon name="refresh" size={11} />
							{$t('ui.pages.integrationsPage.verifyRegenerated')}
						</span>
					{/if}
				</p>
				{#if check.detail}
					<p class="mono-data mt-0.5 truncate text-xs text-muted-foreground" title={check.detail}>
						{check.detail}
					</p>
				{/if}
			</div>
		</li>
	{/each}
</ul>
