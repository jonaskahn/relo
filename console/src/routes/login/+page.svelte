<script lang="ts">
	import { t } from 'svelte-i18n';

	import { page } from '$app/state';

	import { ApiError, api } from '$lib/api';
	import Icon from '$lib/components/ui/icon.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import Card from '$lib/components/ui/card.svelte';
	import CardContent from '$lib/components/ui/card-content.svelte';
	import CardDescription from '$lib/components/ui/card-description.svelte';
	import CardHeader from '$lib/components/ui/card-header.svelte';
	import CardTitle from '$lib/components/ui/card-title.svelte';
	import AppearancePopover from '$lib/components/shell/appearance-popover.svelte';
	import LogoMark from '$lib/components/ui/logo-mark.svelte';
	import SecretInput from '$lib/components/ui/secret-input.svelte';
	import Field from '$lib/components/ui/field.svelte';

	let token = $state('');
	let failure = $state('');
	let submitting = $state(false);

	async function signIn(event: SubmitEvent) {
		event.preventDefault();
		failure = '';
		submitting = true;
		try {
			await api('/auth/login', { method: 'POST', body: JSON.stringify({ admin_token: token }) });
			const returnTo = page.url.searchParams.get('return_to');
			// The session is a cookie the root layout reads once, on mount. A
			// client-side navigation would leave that layout holding its old
			// answer, so the console is loaded afresh instead.
			window.location.href = returnTo && returnTo.startsWith('/') ? returnTo : '/';
		} catch (error) {
			failure = error instanceof ApiError ? error.message : $t('ui.login.failure');
			submitting = false;
		}
	}
</script>

<svelte:head><title>{$t('ui.login.documentTitle')}</title></svelte:head>

<main class="flex min-h-dvh items-center justify-center bg-background px-4 py-8">
	<div class="fixed right-4 top-4 z-10">
		<AppearancePopover />
	</div>
	<Card class="section-surface w-full max-w-md px-2 py-3 sm:px-4 sm:py-5">
		<CardHeader class="text-center">
			<LogoMark class="mx-auto mb-2 size-10 text-accent-strong" />
			<CardTitle>{$t('ui.login.title')}</CardTitle>
			<CardDescription>
				{$t('ui.login.description')}
			</CardDescription>
		</CardHeader>
		<CardContent>
			<form class="space-y-4" onsubmit={signIn}>
				<Field id="admin-token" label={$t('ui.login.tokenLabel')}>
					<SecretInput
						id="admin-token"
						name="admin_token"
						bind:value={token}
						autocomplete="off"
						required
					/>
				</Field>
				{#if failure}
					<p
						class="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive"
					>
						{failure}
					</p>
				{/if}
				<Button type="submit" class="w-full" disabled={submitting}>
					<Icon name={submitting ? 'loader' : 'lock'} size={14} spin={submitting} />
					{submitting ? $t('ui.login.submitting') : $t('ui.login.submit')}
				</Button>
			</form>
		</CardContent>
	</Card>
</main>
