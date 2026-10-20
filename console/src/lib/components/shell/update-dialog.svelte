<script lang="ts">
	import { t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';

	import { installGuides, type InstallGuide, type InstallMethod } from '$lib/update-install';
	import { fileSize, notesText } from '$lib/update-notes';
	import type { UpdateInstaller, UpdateNotice } from '$lib/updates';
	import Button from '$lib/components/ui/button.svelte';
	import CenteredModal from '$lib/components/ui/centered-modal.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import IconAction from '$lib/components/ui/icon-action.svelte';

	interface Props {
		open?: boolean;
		notice: UpdateNotice | null;
	}

	let { open = $bindable(false), notice }: Props = $props();

	const methodKeys: Partial<Record<UpdateInstaller, string>> = {
		shell: 'ui.updateDialog.methodShell',
		powershell: 'ui.updateDialog.methodPowerShell',
		npm: 'ui.updateDialog.methodNpm',
		brew: 'ui.updateDialog.methodBrew'
	};

	const guides = $derived(notice ? installGuides(notice.installer, notice.latest) : []);
	const methodKey = $derived(notice?.installer ? (methodKeys[notice.installer] ?? '') : '');
	const notes = $derived(notice?.notes ? notesText(notice.notes) : '');
	const downloads = $derived(notice?.downloads ?? []);
	const releasePage = $derived(notice?.page || notice?.url || '');

	let copied = $state<InstallMethod | null>(null);
	let timer: ReturnType<typeof setTimeout> | undefined;

	$effect(() => () => clearTimeout(timer));

	async function copy(guide: InstallGuide) {
		try {
			await navigator.clipboard.writeText(guide.command);
			copied = guide.method;
			clearTimeout(timer);
			timer = setTimeout(() => (copied = null), 1500);
		} catch {
			toast.error($t('ui.common.error'));
		}
	}
</script>

<CenteredModal
	bind:open
	size="compact"
	title={$t('ui.updateDialog.title')}
	description={$t('ui.updateDialog.body', {
		values: { current: notice?.current ?? '', latest: notice?.latest ?? '' }
	})}
>
	<div class="flex flex-col gap-4">
		{#if notice}
			{#if guides.length === 0}
				<p class="text-sm text-muted-foreground">
					{$t(
						notice.installer === 'sparkle' ? 'ui.updateDialog.sparkle' : 'ui.updateDialog.installer'
					)}
				</p>
			{:else}
				<p class="text-sm text-muted-foreground">
					{methodKey
						? $t('ui.updateDialog.detected', { values: { method: $t(methodKey) } })
						: $t('ui.updateDialog.unknown')}
				</p>
				{#each guides as guide (guide.method)}
					<div class="overflow-hidden rounded-panel bg-sunken">
						<div class="flex items-center gap-2 px-3 pt-1.5">
							<span class="mono-data min-w-0 flex-1 truncate text-faint">
								{$t(guide.labelKey)}
							</span>
							<IconAction
								icon={copied === guide.method ? 'check' : 'copy'}
								label={copied === guide.method ? $t('ui.common.copied') : $t('ui.common.copy')}
								onclick={() => copy(guide)}
							/>
						</div>
						<pre class="overflow-x-auto px-3 pt-1 pb-3 text-[13px] leading-6"><code
								class="font-mono">{guide.command}</code
							></pre>
					</div>
				{/each}
			{/if}
			{#if notes}
				<div class="flex flex-col gap-1.5">
					<p class="text-sm font-medium text-ink">{$t('ui.updateDialog.whatsNew')}</p>
					<p class="whitespace-pre-line text-sm leading-6 text-muted-foreground">{notes}</p>
				</div>
			{/if}
			{#if downloads.length > 0}
				<div class="flex flex-col gap-1.5">
					<p class="text-sm font-medium text-ink">{$t('ui.updateDialog.download')}</p>
					{#each downloads as download (download.url)}
						<a
							href={download.url}
							target="_blank"
							rel="external noreferrer"
							class="flex items-center gap-2 rounded-panel bg-sunken px-3 py-2 text-sm text-ink underline-offset-2 hover:underline"
						>
							<Icon name="cloud-download" size={14} />
							<span class="min-w-0 flex-1 truncate">{download.name}</span>
							{#if fileSize(download.size)}
								<span class="mono-data shrink-0 text-muted-foreground">
									{fileSize(download.size)}
								</span>
							{/if}
						</a>
					{/each}
				</div>
			{/if}
			<a
				href={releasePage}
				target="_blank"
				rel="external noreferrer"
				class="inline-flex items-center gap-1.5 text-sm text-accent-deep underline-offset-2 hover:underline"
			>
				<Icon name="external-link" size={14} />
				{$t('ui.updateDialog.releasePage')}
			</a>
		{/if}
	</div>
	{#snippet footer()}
		<div class="modal-footer">
			<Button variant="outline" onclick={() => (open = false)}>
				<Icon name="x" size={14} />
				{$t('ui.common.close')}
			</Button>
		</div>
	{/snippet}
</CenteredModal>
