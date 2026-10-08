<script lang="ts">
	import { t } from 'svelte-i18n';

	import Button from '$lib/components/ui/button.svelte';
	import CenteredModal from '$lib/components/ui/centered-modal.svelte';
	import ConfirmDialog from '$lib/components/ui/confirm-dialog.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import Segmented from '$lib/components/ui/segmented.svelte';
	import { CLIENT_OPTIONS, setupSnippet, type DataPlaneURLs } from '$lib/client-setup';
	import type { IssuedAccessKey } from '$lib/types';

	// KeyRevealModal shows a key once, the moment it exists: after a create,
	// or after a rotation minted a new one. Closing it drops the token from
	// the page's memory, and the Done control asks before it lets a key leave
	// the screen uncopied.

	interface Props {
		open?: boolean;
		issued: IssuedAccessKey | null;
		issuedFor: string;
		urls: DataPlaneURLs;
		onclose: () => void;
	}

	let { open = $bindable(false), issued, issuedFor, urls, onclose }: Props = $props();

	let copied = $state(false);
	let doneOpen = $state(false);
	// The snippet tab starts on the client the key was minted for, and only
	// moves when an operator picks the other one.
	let chosenTab = $state<'codex' | 'env' | null>(null);
	const setupTab = $derived(chosenTab ?? (issuedFor === 'codex' ? 'codex' : 'env'));

	const codexLabel = CLIENT_OPTIONS.find((option) => option.id === 'codex')?.label ?? 'Codex';
	const snippet = $derived(
		issued ? setupSnippet(setupTab === 'codex' ? 'codex' : issuedFor, urls, issued.token) : null
	);

	async function copyToken() {
		if (!issued) return;
		try {
			await navigator.clipboard.writeText(issued.token);
			copied = true;
		} catch {
			copied = false;
		}
	}

	function close() {
		open = false;
		doneOpen = false;
	}

	// Closing drops the token from the page's memory, so the next reveal
	// starts with nothing on screen and nothing in the copy state.
	function release() {
		copied = false;
		chosenTab = null;
		onclose();
	}
</script>

<CenteredModal
	bind:open
	onOpenChange={(value) => {
		if (!value) release();
	}}
	size="standard"
	title={$t('ui.pages.keysPage.revealTitle')}
	description={$t('ui.pages.keysPage.revealDescription')}
	dismissible={false}
>
	{#snippet footer()}
		<div class="modal-footer">
			<Button variant="outline" onclick={copyToken}>
				<Icon name={copied ? 'check' : 'copy'} size={14} />
				{copied ? $t('ui.common.copied') : $t('ui.common.copy')}
			</Button>
			<Button onclick={() => (copied ? close() : (doneOpen = true))}>
				<Icon name="check" size={14} />
				{$t('ui.common.done')}
			</Button>
		</div>
	{/snippet}
	{#if issued}
		<div class="flex flex-col gap-4">
			<!-- The one-time notice is a banner, not a stripe: a full tinted
			     border, a tinted fill and a leading icon tile. -->
			<div class="flex items-start gap-3 rounded-card border border-warn/40 bg-warn/10 px-4 py-3">
				<span
					class="flex size-[30px] shrink-0 items-center justify-center rounded-panel bg-warn/15 text-warn"
				>
					<Icon name="alert-triangle" size={16} />
				</span>
				<p class="text-sm text-warn">{$t('ui.pages.keysPage.revealOnce')}</p>
			</div>
			<div class="rounded-panel border border-line bg-sunken px-3 py-2.5">
				<code class="mono-data block break-all text-sm">{issued.token}</code>
			</div>
			<div class="flex flex-col gap-2">
				<Segmented
					value={setupTab}
					onchange={(next) => (chosenTab = next as 'codex' | 'env')}
					options={[
						{ value: 'codex', label: codexLabel },
						{ value: 'env', label: $t('ui.pages.keysPage.setupEnv') }
					]}
					ariaLabel={$t('ui.pages.keysPage.setupTitle')}
				/>
				{#if snippet}
					<pre
						class="mono-data overflow-x-auto rounded-panel border border-line bg-sunken p-3 text-xs">{snippet.code}</pre>
					<div class="grid gap-2 sm:grid-cols-2">
						<div
							class="flex items-center justify-between gap-2 rounded-panel border border-line px-3 py-2"
						>
							<span class="text-xs text-muted-foreground">{$t('ui.pages.keysPage.baseUrl')}</span>
							<span class="mono-data truncate"
								>{snippet.code.includes('ANTHROPIC_BASE_URL')
									? urls.anthropic
									: urls.openai + '/v1'}</span
							>
						</div>
						<div
							class="flex items-center justify-between gap-2 rounded-panel border border-line px-3 py-2"
						>
							<span class="text-xs text-muted-foreground">{$t('ui.pages.keysPage.apiKey')}</span>
							<span class="mono-data truncate">{issued.token.slice(0, 8)}…</span>
						</div>
					</div>
				{/if}
			</div>
		</div>
	{/if}
</CenteredModal>

<ConfirmDialog
	bind:open={doneOpen}
	title={$t('ui.pages.keysPage.doneTitle')}
	body={$t('ui.pages.keysPage.doneBody')}
	confirmLabel={$t('ui.common.done')}
	tone="destructive"
	icon="alert-triangle"
	onconfirm={close}
/>
