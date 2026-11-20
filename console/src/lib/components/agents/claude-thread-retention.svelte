<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from 'svelte-i18n';

	import { api } from '$lib/api';
	import Icon from '$lib/components/ui/icon.svelte';
	import Select from '$lib/components/ui/select.svelte';
	import SettingsSave from '$lib/components/settings/settings-save.svelte';

	// How long Relo rebuilds a Claude thread from the server side. Claude
	// Desktop sends only the new turn once a thread starts, so a stored reply
	// has to outlive the request that produced it; this window is how long an
	// operator may still continue one.
	interface Props {
		days: number;
		onsaved: (days: number) => void;
	}

	let { days, onsaved }: Props = $props();

	const windows = $derived(
		[1, 3, 7, 14, 30, 60, 90, 365].map((value) => ({
			value: String(value),
			label: $t('ui.pages.integrationsPage.claudeThreadsDays', { values: { count: value } })
		}))
	);

	let choice = $state(untrack(() => String(days)));
	let saving = $state(false);
	let error = $state('');
	const changed = $derived(choice !== String(days));

	async function save() {
		if (saving || !changed) return;
		saving = true;
		error = '';
		try {
			const next = await api<{ days: number }>('/settings/threads', {
				method: 'PUT',
				body: JSON.stringify({ days: Number(choice) })
			});
			onsaved(next.days);
		} catch (failure) {
			error = failure instanceof Error ? failure.message : String(failure);
		} finally {
			saving = false;
		}
	}
</script>

<section class="space-y-3 border-t border-border pt-4">
	<div class="min-w-0">
		<h3 class="text-sm font-semibold">{$t('ui.pages.integrationsPage.claudeThreadsTitle')}</h3>
		<p class="text-xs text-muted-foreground">
			{$t('ui.pages.integrationsPage.claudeThreadsHint')}
		</p>
	</div>

	<div class="max-w-xs">
		<Select
			id="claude-thread-retention"
			ariaLabel={$t('ui.pages.integrationsPage.claudeThreadsTitle')}
			options={windows}
			bind:value={choice}
			disabled={saving}
		/>
	</div>
	<SettingsSave dirty={changed} {saving} {error} onsave={() => void save()} />

	{#if days === 0}
		<p class="flex items-start gap-2 text-xs text-muted-foreground">
			<Icon name="info-circle" size={14} />
			<span>{$t('ui.pages.integrationsPage.claudeThreadsUnlimited')}</span>
		</p>
	{/if}
</section>
