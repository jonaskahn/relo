<script lang="ts">
	import { t } from 'svelte-i18n';

	import { api } from '$lib/api';
	import Button from '$lib/components/ui/button.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import { integrationPath } from '$lib/integration-actions';
	import type { IntegrationView } from '$lib/types';

	// How Claude Code is pointed at Relo. Environment writes the client's own
	// settings; proxy leaves the client talking to Anthropic and puts Relo's
	// intercept in the path instead, which keeps a claude.ai login working.
	interface Props {
		agent: IntegrationView;
		onchanged: (agent: IntegrationView) => void;
	}

	let { agent, onchanged }: Props = $props();

	let busy = $state(false);
	let failure = $state('');
	let confirmProxy = $state(false);

	const mode = $derived(agent.mode === 'proxy' ? 'proxy' : 'env');
	const proxyState = $derived(agent.proxy_state ?? '');

	async function choose(next: 'env' | 'proxy') {
		if (next === mode) return;
		if (next === 'proxy' && !confirmProxy) {
			// The warning is the point of this step: an operator turning proxy
			// mode on reads what it does to their account before it is written.
			confirmProxy = true;
			return;
		}
		busy = true;
		failure = '';
		try {
			const answer = await api<IntegrationView>(integrationPath(agent.id, 'mode'), {
				method: 'POST',
				body: JSON.stringify({ mode: next })
			});
			confirmProxy = false;
			onchanged(answer);
		} catch (error) {
			failure = error instanceof Error ? error.message : String(error);
		} finally {
			busy = false;
		}
	}

	function proxyStateKey(state: string): string {
		switch (state) {
			case 'applied':
				return 'ui.pages.integrationsPage.claudeModeStateApplied';
			case 'stale':
				return 'ui.pages.integrationsPage.claudeModeStateStale';
			case 'foreign':
				return 'ui.pages.integrationsPage.claudeModeStateForeign';
			case 'unreadable':
				return 'ui.pages.integrationsPage.claudeModeStateUnreadable';
			default:
				return 'ui.pages.integrationsPage.claudeModeStateAbsent';
		}
	}
</script>

<section class="space-y-3 border-t border-border pt-4">
	<div class="min-w-0">
		<h3 class="text-sm font-semibold">{$t('ui.pages.integrationsPage.claudeModeTitle')}</h3>
		<p class="text-xs text-muted-foreground">{$t('ui.pages.integrationsPage.claudeModeHint')}</p>
	</div>

	<div class="grid gap-3 sm:grid-cols-2">
		<button
			type="button"
			class="rounded-lg border p-3 text-left transition-colors {mode === 'env'
				? 'border-primary bg-primary/5'
				: 'border-border hover:border-muted-foreground/40'}"
			disabled={busy}
			aria-pressed={mode === 'env'}
			onclick={() => void choose('env')}
		>
			<span class="flex items-center gap-2 text-sm font-medium">
				<Icon name={mode === 'env' ? 'circle-check' : 'circle-off'} size={14} />
				{$t('ui.pages.integrationsPage.claudeModeEnv')}
			</span>
			<span class="mt-1 block text-xs text-muted-foreground">
				{$t('ui.pages.integrationsPage.claudeModeEnvHint')}
			</span>
		</button>
		<button
			type="button"
			class="rounded-lg border p-3 text-left transition-colors {mode === 'proxy'
				? 'border-primary bg-primary/5'
				: 'border-border hover:border-muted-foreground/40'}"
			disabled={busy}
			aria-pressed={mode === 'proxy'}
			onclick={() => void choose('proxy')}
		>
			<span class="flex items-center gap-2 text-sm font-medium">
				<Icon name={mode === 'proxy' ? 'circle-check' : 'circle-off'} size={14} />
				{$t('ui.pages.integrationsPage.claudeModeProxy')}
			</span>
			<span class="mt-1 block text-xs text-muted-foreground">
				{$t('ui.pages.integrationsPage.claudeModeProxyHint')}
			</span>
		</button>
	</div>

	{#if mode === 'proxy'}
		<p class="flex items-start gap-2 text-xs text-muted-foreground">
			<Icon name="info-circle" size={14} />
			<span>{$t(proxyStateKey(proxyState))}</span>
		</p>
	{/if}

	{#if confirmProxy}
		<div class="rounded-lg border border-warning/40 bg-warning/5 p-3">
			<p class="text-xs text-warning">{$t('ui.pages.integrationsPage.claudeModeProxyWarning')}</p>
			<div class="mt-2 flex flex-wrap items-center gap-2">
				<Button size="sm" disabled={busy} onclick={() => void choose('proxy')}>
					{$t('ui.pages.integrationsPage.claudeModeProxyConfirm')}
				</Button>
				<Button size="sm" variant="ghost" disabled={busy} onclick={() => (confirmProxy = false)}>
					{$t('ui.common.cancel')}
				</Button>
			</div>
		</div>
	{/if}

	{#if busy}
		<p class="flex items-center gap-2 text-xs text-muted-foreground">
			<Icon name="loader" size={14} spin />
			{$t('ui.common.saving')}
		</p>
	{:else if failure}
		<p class="text-sm text-danger" role="alert">{failure}</p>
	{/if}
</section>
