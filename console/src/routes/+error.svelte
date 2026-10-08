<script lang="ts">
	import { t } from 'svelte-i18n';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import ErrorArt from '$lib/components/error/error-art.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import Button from '$lib/components/ui/button.svelte';

	// +error is where the console answers what it could not do. The Go server
	// serves the shell for every address, so an old or mistyped link lands here
	// in the browser rather than on a bare server error, and the shell itself
	// stands aside: the rail and the top bar are the ways into pages that
	// worked.
	//
	// One boundary answers every status, and each is given its own mark and
	// the one action that would help: the sign-in for a session that ended,
	// home for a page that is not there, another attempt for a failure of the
	// daemon's own.
	//
	// The status and the error are read from the page rather than taken as
	// props: the root component hands an error page its data, form and params,
	// and nothing else, so props would be undefined here.
	type Action = 'signin' | 'dashboard' | 'retry';
	type Art = 'missing' | 'locked' | 'barred' | 'fault';

	const ERRORS: Record<string, { title: string; body: string; action: Action; art: Art }> = {
		unauthorized: {
			title: 'ui.error.unauthorizedTitle',
			body: 'ui.error.unauthorizedBody',
			action: 'signin',
			art: 'locked'
		},
		forbidden: {
			title: 'ui.error.forbiddenTitle',
			body: 'ui.error.forbiddenBody',
			action: 'dashboard',
			art: 'barred'
		},
		missing: {
			title: 'ui.error.notFoundTitle',
			body: 'ui.error.notFoundBody',
			action: 'dashboard',
			art: 'missing'
		},
		server: {
			title: 'ui.error.serverTitle',
			body: 'ui.error.serverBody',
			action: 'retry',
			art: 'fault'
		},
		other: { title: 'ui.error.title', body: 'ui.error.body', action: 'retry', art: 'fault' }
	};

	const ACTIONS: Record<Action, string> = {
		signin: 'ui.error.signIn',
		dashboard: 'ui.error.backToHome',
		retry: 'ui.common.retry'
	};

	const status = $derived(page.status);
	const error = $derived(page.error);

	const kind = $derived(
		status === 401
			? 'unauthorized'
			: status === 403
				? 'forbidden'
				: status === 404
					? 'missing'
					: status >= 500
						? 'server'
						: 'other'
	);
	const copy = $derived(ERRORS[kind]);

	function act() {
		if (copy.action === 'signin') {
			void goto('/login?return_to=' + encodeURIComponent(page.url.pathname + page.url.search));
			return;
		}
		if (copy.action === 'retry') {
			location.reload();
			return;
		}
		void goto('/');
	}
</script>

<svelte:head><title>{$t(copy.title)} · Relo</title></svelte:head>

<div
	class="page-frame flex min-h-full flex-col items-center justify-center gap-5 py-16 text-center"
>
	<ErrorArt kind={copy.art} />
	<p class="mono-data text-sm text-muted-foreground">{status}</p>
	<p class="max-w-sm text-sm text-muted-foreground">
		{$t(copy.body, { values: { path: page.url.pathname } })}
	</p>
	{#if kind !== 'missing' && error?.message}
		<p class="mono-data max-w-sm text-xs text-muted-foreground">{error.message}</p>
	{/if}
	<div class="mt-1">
		<Button onclick={act}>
			<Icon
				name={copy.action === 'retry'
					? 'refresh'
					: copy.action === 'dashboard'
						? 'layout-dashboard'
						: 'lock'}
				size={14}
			/>
			{$t(ACTIONS[copy.action])}
		</Button>
	</div>
</div>
