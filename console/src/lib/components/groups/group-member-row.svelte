<script lang="ts">
	import { t } from 'svelte-i18n';
	import { memberKind } from '$lib/route-stepper';
	import type { GroupMember } from '$lib/types';
	import Icon from '$lib/components/ui/icon.svelte';
	import ProviderLogo from '$lib/components/ui/provider-logo.svelte';

	// GroupMemberRow is one member of a group, as the card and the detail
	// modal both read it: the order a request would read, the model over its
	// identifier, the rate the catalog has for it, and the state that says
	// whether it can serve a request at all. What a surface leaves out — the
	// weight, the account coverage — is passed empty rather than guessed here.
	interface Props {
		member: GroupMember;
		position: number;
		title?: string;
		label: string;
		price?: string;
		weight?: number | null;
		coverage?: string;
	}

	let {
		member,
		position,
		title = '',
		label,
		price = '',
		weight = null,
		coverage = ''
	}: Props = $props();

	// A member naming a bare identifier follows every connection that serves
	// it, so it has no single logo: the Auto badge says so and the identifier
	// reads alone. One stored before a member could be bare carries no kind
	// and is read the same way.
	const bare = $derived(memberKind(member) === 'auto' || member.provider_id === '');
</script>

<li class="flex items-center gap-2 px-2 py-1.5">
	<span class="mono-data w-4 shrink-0 text-muted-foreground">{position}</span>
	{#if bare}
		<span
			class="badge shrink-0 border border-accent/40 text-accent-ink"
			title={$t('ui.pages.groupsPage.autoHint')}
		>
			{$t('ui.pages.groupsPage.autoBadge')}
		</span>
	{:else}
		<ProviderLogo id={member.provider_id} {label} size="xs" />
	{/if}
	<div class="min-w-0 flex-1">
		<span class="block truncate text-sm">{title || member.model_id}</span>
		<span class="mono-data block truncate text-muted-foreground">
			{bare ? member.model_id : member.provider_id + '/' + member.model_id}
		</span>
	</div>
	{#if !member.enabled}
		<span class="badge shrink-0 border border-border text-muted-foreground">
			{$t('ui.pages.groupsPage.chipDisabled')}
		</span>
	{:else if !member.eligible && member.reason}
		<span class="badge shrink-0 border border-warn/40 bg-warn/10 text-warn">
			<Icon name="alert-triangle" size={11} />
			{member.reason}
		</span>
	{/if}
	{#if coverage}
		<span class="mono-data shrink-0 text-muted-foreground">{coverage}</span>
	{/if}
	{#if weight !== null}
		<span class="mono-data shrink-0 text-muted-foreground" title={$t('ui.pages.groupsPage.weight')}>
			×{weight}
		</span>
	{/if}
	{#if price}
		<span class="mono-data shrink-0 text-muted-foreground">{price}</span>
	{/if}
</li>
