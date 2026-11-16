<script lang="ts">
	import { t } from 'svelte-i18n';

	import Icon from '$lib/components/ui/icon.svelte';
	import type { IntegrationView } from '$lib/types';

	// How Claude Code, or Claude Desktop, is pointed at Relo. Environment writes the
	// client's own settings, or Desktop's gateway profile; proxy leaves the client
	// talking to Anthropic and puts Relo's intercept in the path instead, which
	// keeps a claude.ai login working.
	//
	// Choosing writes nothing. The pick is held by the page, whose primary button
	// becomes Save while it differs from what is stored, so the operator confirms
	// the change before the wiring moves.
	interface Props {
		agent: IntegrationView;
		selected: 'env' | 'proxy';
		onselect: (mode: 'env' | 'proxy') => void;
		busy?: boolean;
		failure?: string;
	}

	let { agent, selected, onselect, busy = false, failure = '' }: Props = $props();

	const mode = $derived(selected);
	const desktop = $derived(agent.id === 'claude-desktop');
	const stored = $derived(agent.enabled && agent.mode === 'proxy' ? 'proxy' : 'env');
	const proxyState = $derived(agent.proxy_state ?? '');
	// The warning reads while proxy is chosen but not yet the stored mode.
	const warn = $derived(mode === 'proxy' && stored !== 'proxy');
	const dirty = $derived(agent.enabled && mode !== stored);

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
		<p class="text-xs text-muted-foreground">
			{$t(
				desktop
					? 'ui.pages.integrationsPage.claudeModeDesktopHint'
					: 'ui.pages.integrationsPage.claudeModeHint'
			)}
		</p>
	</div>

	<div class="grid gap-3 sm:grid-cols-2">
		<button
			type="button"
			class="rounded-lg border p-3 text-left transition-colors {mode === 'env'
				? 'border-primary bg-primary/5'
				: 'border-border hover:border-muted-foreground/40'}"
			disabled={busy}
			aria-pressed={mode === 'env'}
			onclick={() => onselect('env')}
		>
			<span class="flex items-center gap-2 text-sm font-medium">
				<Icon name={mode === 'env' ? 'circle-check' : 'circle-off'} size={14} />
				{$t('ui.pages.integrationsPage.claudeModeEnv')}
			</span>
			<span class="mt-1 block text-xs text-muted-foreground">
				{$t(
					desktop
						? 'ui.pages.integrationsPage.claudeModeDesktopEnvHint'
						: 'ui.pages.integrationsPage.claudeModeEnvHint'
				)}
			</span>
		</button>
		<button
			type="button"
			class="rounded-lg border p-3 text-left transition-colors {mode === 'proxy'
				? 'border-primary bg-primary/5'
				: 'border-border hover:border-muted-foreground/40'}"
			disabled={busy}
			aria-pressed={mode === 'proxy'}
			onclick={() => onselect('proxy')}
		>
			<span class="flex items-center gap-2 text-sm font-medium">
				<Icon name={mode === 'proxy' ? 'circle-check' : 'circle-off'} size={14} />
				{$t('ui.pages.integrationsPage.claudeModeProxy')}
			</span>
			<span class="mt-1 block text-xs text-muted-foreground">
				{$t(
					desktop
						? 'ui.pages.integrationsPage.claudeModeDesktopProxyHint'
						: 'ui.pages.integrationsPage.claudeModeProxyHint'
				)}
			</span>
		</button>
	</div>

	{#if warn}
		<p class="rounded-lg border border-warning/40 bg-warning/5 p-3 text-xs text-warning">
			{$t('ui.pages.integrationsPage.claudeModeProxyWarning')}
		</p>
	{:else if mode === 'proxy' && agent.enabled}
		<p class="flex items-start gap-2 text-xs text-muted-foreground">
			<Icon name="info-circle" size={14} />
			<span>{$t(proxyStateKey(proxyState))}</span>
		</p>
	{/if}

	{#if dirty}
		<p class="text-xs text-muted-foreground" role="status">
			{$t('ui.pages.integrationsPage.claudeModeUnsaved')}
		</p>
	{/if}

	{#if failure}
		<p class="text-sm text-danger" role="alert">{failure}</p>
	{/if}
</section>
