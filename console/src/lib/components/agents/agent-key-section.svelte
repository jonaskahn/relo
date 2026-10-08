<script lang="ts">
	import { t } from 'svelte-i18n';
	import { Tooltip } from 'bits-ui';

	import Icon from '$lib/components/ui/icon.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import IconAction from '$lib/components/ui/icon-action.svelte';
	import { keyPathOf } from '$lib/integration-actions';
	import type { IntegrationView } from '$lib/types';

	// The key the agent authenticates with. The secret is read on demand, so the
	// eye control shows what the agent actually runs with rather than only the
	// hint; a refused read reports its reason beside the hint instead of in a
	// toast, because the operator asked to see one value.
	interface Props {
		agent: IntegrationView;
		visible: boolean;
		token: string;
		loading: boolean;
		copied: boolean;
		failure: string;
		busy: boolean;
		ontoggle: () => void;
		oncopy: () => void;
	}

	let { agent, visible, token, loading, copied, failure, busy, ontoggle, oncopy }: Props = $props();

	// envNote names the variable the client's own file reads when that file holds
	// a reference rather than the secret, and the file Relo keeps it in.
	const envNote = $derived.by(() => {
		if (!agent.env_key) return '';
		const path = keyPathOf(agent);
		return path
			? $t('ui.pages.integrationsPage.envKey', { values: { variable: agent.env_key, path } })
			: $t('ui.pages.integrationsPage.envKeyNoPath', { values: { variable: agent.env_key } });
	});
</script>

<div class="space-y-2">
	<h3 class="text-sm font-semibold">{$t('ui.pages.integrationsPage.keyTitle')}</h3>
	{#if !agent.key}
		<p class="text-xs text-muted-foreground">
			{agent.enabled
				? $t('ui.pages.integrationsPage.keyNone')
				: $t('ui.pages.integrationsPage.keyNonePending')}
		</p>
	{:else if agent.key}
		{@const hint = agent.key.token_hint}
		<div class="flex flex-wrap items-center gap-2">
			<Tooltip.Provider delayDuration={250} disableHoverableContent>
				<Tooltip.Root>
					<Tooltip.Trigger>
						{#snippet child({ props: { type: _type, tabindex: _tabindex, ...rest } })}
							<code
								{...rest}
								class="mono-data flex h-control min-w-0 flex-1 items-center truncate rounded-lg border border-border bg-sunken px-2.5 text-xs"
							>
								{visible ? token : hint}
							</code>
						{/snippet}
					</Tooltip.Trigger>
					<Tooltip.Portal>
						<Tooltip.Content class="z-50 surface-pop max-w-sm break-all px-2.5 py-1.5 text-xs">
							{visible ? token : hint}
						</Tooltip.Content>
					</Tooltip.Portal>
				</Tooltip.Root>
			</Tooltip.Provider>
			<IconAction
				icon={loading ? 'loader' : visible ? 'eye-off' : 'eye'}
				variant="outline"
				spin={loading}
				label={$t(visible ? 'ui.common.hide' : 'ui.common.reveal')}
				disabled={busy || loading}
				onclick={ontoggle}
			/>
			{#if visible}
				<Button variant="outline" onclick={oncopy}>
					<Icon name={copied ? 'check' : 'copy'} size={14} />
					{copied ? $t('ui.common.copied') : $t('ui.common.copy')}
				</Button>
			{/if}
		</div>
		{#if failure}
			<p class="flex items-start gap-1.5 text-xs text-danger" role="alert">
				<Icon name="circle-x" size={13} class="mt-px shrink-0" />
				{failure}
			</p>
		{:else}
			<p class="text-xs text-muted-foreground">
				{envNote || $t('ui.pages.integrationsPage.keyManaged')}
			</p>
		{/if}
	{/if}
</div>
