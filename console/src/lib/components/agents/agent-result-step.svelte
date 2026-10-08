<script lang="ts">
	import { t } from 'svelte-i18n';

	import Icon from '$lib/components/ui/icon.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import { cn } from '$lib/utils';
	import { setupSnippet, type DataPlaneURLs } from '$lib/client-setup';
	import { stepSwap } from '$lib/modal-motion';
	import type { IntegrationView } from '$lib/types';

	// What an action did, or the one-time key it minted. This is a step of the
	// detail modal rather than a modal of its own: the operator never leaves the
	// surface they started the work on, and the key is shown in the step that
	// produced it.
	interface Props {
		agent: IntegrationView;
		state: 'success' | 'failed';
		copy: string;
		token: string;
		urls: DataPlaneURLs;
		copied: boolean;
		oncopy: () => void;
	}

	let { agent, state, copy, token, urls, copied, oncopy }: Props = $props();

	// A failure stays here with its reason and the way out, so the operator reads
	// it before leaving rather than after a toast has gone.
	const snippet = $derived(
		agent.manages_files || !token ? '' : setupSnippet(agent.client, urls, token).code
	);
	const hasToken = $derived(token !== '');
</script>

{#if hasToken}
	<div class="space-y-4" in:stepSwap>
		<p
			class={cn('flex items-start gap-2 text-sm', state === 'failed' ? 'text-danger' : 'text-ink')}
			role={state === 'failed' ? 'alert' : 'status'}
		>
			<Icon
				name={state === 'failed' ? 'circle-x' : 'circle-check'}
				size={16}
				class={cn('mt-0.5 shrink-0', state === 'failed' ? 'text-danger' : 'text-ok')}
			/>
			{copy}
		</p>
		<div class="flex items-center gap-2">
			<code
				class="mono-data min-w-0 flex-1 truncate rounded-lg border border-border bg-sunken px-3 py-2 text-xs"
				>{token}</code
			>
			<Button variant="outline" onclick={oncopy}>
				<Icon name={copied ? 'check' : 'copy'} size={14} />
				{copied ? $t('ui.common.copied') : $t('ui.common.copy')}
			</Button>
		</div>
		{#if snippet}
			<pre
				class="mono-data overflow-x-auto rounded-lg border border-border bg-sunken p-3 text-xs">{snippet}</pre>
		{/if}
		<p class="text-xs text-muted-foreground">{$t('ui.pages.integrationsPage.revealOnce')}</p>
	</div>
{:else}
	<div class="flex flex-col items-center gap-3 py-2 text-center" in:stepSwap>
		<Icon
			name={state === 'failed' ? 'circle-x' : 'circle-check'}
			size={28}
			class={state === 'failed' ? 'text-danger' : 'text-ok'}
		/>
		<p class="text-sm text-muted-foreground">{copy}</p>
	</div>
{/if}
