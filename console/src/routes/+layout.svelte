<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import { consoleState } from '$lib/console-state.svelte';
	import { session } from '$lib/session.svelte';
	import type { SessionResponse } from '$lib/types';
	import Icon from '$lib/components/ui/icon.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import CommandPalette from '$lib/components/shell/command-palette.svelte';
	import ManageDaemonDialog from '$lib/components/shell/manage-daemon-dialog.svelte';
	import Sidebar from '$lib/components/shell/sidebar.svelte';
	import Topbar from '$lib/components/shell/topbar.svelte';
	import { navDrawer } from '$lib/nav-drawer.svelte';
	import { isWorkspaceRoute } from '$lib/nav-shell';
	import type { Snippet } from 'svelte';
	import { pageMotion } from '$lib/page-motion';
	import { cn } from '$lib/utils';
	import { Toaster } from 'svelte-sonner';

	interface Props {
		children: Snippet;
	}

	let { children }: Props = $props();
	let authenticated = $state<boolean | null>(null);
	let sessionUnavailable = $state(false);
	// scrolled follows the main scroll region, so the top bar can separate
	// itself from content that moved under it.
	let scrolled = $state(false);
	const isSignInPage = $derived(page.url.pathname === '/login');
	// An error page stands on its own: the rail, the top bar and the palette
	// are the ways into pages that worked, so a page that did not shows none
	// of them and asks for the one action that can help.
	const errored = $derived(page.error !== null);
	// A workspace page fills the viewport and scrolls its own panes, which
	// is what a two-pane screen needs.
	const workspace = $derived(isWorkspaceRoute(page.url.pathname));
	// Chat keeps the console rail docked and may start it compact when the
	// saved menu is full. Other pages use the default dock behaviour.
	navDrawer.setWorkspaceAutoCompact(() => workspace);

	async function checkSession() {
		authenticated = null;
		sessionUnavailable = false;
		try {
			const state = await api<SessionResponse>('/auth/session');
			// A daemon that does not answer the field is one from before the
			// sign-in became optional, and that daemon always asks for it.
			session.setLoginRequired(state.login_required ?? true);
			authenticated = state.authenticated;
			if (state.authenticated || !state.login_required)
				void consoleState.loadAppearance().catch(() => {});
		} catch {
			authenticated = false;
			sessionUnavailable = true;
			return;
		}
		if (!session.loginRequired && isSignInPage) {
			await goto('/', { replaceState: true });
			return;
		}
		// An error page is the one surface that has to survive a session that
		// has ended: a 401 says so and offers the sign-in itself, and the
		// redirect that would have replaced it would answer neither.
		if (!authenticated && !isSignInPage && !errored) {
			const target = page.url.pathname + page.url.search;
			await goto('/login?return_to=' + encodeURIComponent(target), { replaceState: true });
		}
	}

	onMount(() => {
		void checkSession();
		// The shell follows the dock breakpoint so the sidebar can decide between
		// the docked rail and the overlay drawer.
		const docked = window.matchMedia('(min-width: 1024px)');
		const follow = (event: MediaQueryListEvent) => navDrawer.setWide(event.matches);
		navDrawer.setWide(docked.matches);
		docked.addEventListener('change', follow);
		return () => docked.removeEventListener('change', follow);
	});
</script>

{#if authenticated === null}
	<div class="flex h-dvh items-center justify-center text-muted-foreground text-sm">
		{$t('ui.session.checking')}
	</div>
{:else if sessionUnavailable}
	<div class="flex h-dvh flex-col items-center justify-center gap-4 px-4 text-center">
		<div>
			<h1 class="font-semibold">{$t('ui.session.unavailableTitle')}</h1>
			<p class="mt-1 text-sm text-muted-foreground">{$t('ui.session.unavailableBody')}</p>
		</div>
		<Button onclick={() => void checkSession()}>
			<Icon name="refresh" size={14} />
			{$t('ui.common.retry')}
		</Button>
	</div>
{:else if isSignInPage && session.loginRequired}
	{@render children()}
{:else if authenticated || !session.loginRequired || errored}
	<div class="flex h-dvh overflow-hidden bg-canvas">
		{#if !errored}
			<!-- A docked rail takes its width from the page, so the content beside it
			     is pushed rather than covered. Compact keeps an icon column; full
			     takes the wider column. A page that asked for the overlay leaves
			     no column. -->
			<div
				aria-hidden="true"
				class={cn(
					'hidden shrink-0 transition-[width] duration-200 ease-in-out lg:block',
					!navDrawer.pinned && 'lg:w-0',
					navDrawer.pinned && navDrawer.compact && 'lg:w-16',
					navDrawer.pinned && !navDrawer.compact && 'lg:w-[224px]'
				)}
			></div>
			<Sidebar />
		{/if}
		<div class="flex min-w-0 flex-1 flex-col overflow-hidden">
			{#if !errored}<Topbar {scrolled} />{/if}
			<main
				class={!errored && workspace ? 'min-h-0 flex-1 overflow-hidden' : 'flex-1 overflow-y-auto'}
				onscroll={(event) => (scrolled = event.currentTarget.scrollTop > 0)}
			>
				{#if errored}
					{@render children()}
				{:else}
					{#key page.url.pathname}
						<div class:h-full={workspace} class:min-h-0={workspace} in:pageMotion>
							{@render children()}
						</div>
					{/key}
				{/if}
			</main>
		</div>
		{#if !errored}
			<CommandPalette />
			<ManageDaemonDialog />
		{/if}
	</div>
	{#if !errored}<Toaster position="bottom-right" />{/if}
{:else}
	<div class="flex h-dvh items-center justify-center text-muted-foreground text-sm">
		{$t('ui.session.checking')}
	</div>
{/if}
