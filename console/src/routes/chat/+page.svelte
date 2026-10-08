<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { t } from 'svelte-i18n';
	import { fade, fly, slide } from 'svelte/transition';

	import { api } from '$lib/api';
	import { ChatSender } from '$lib/chat-send.svelte';
	import {
		activeThread,
		clearChatStore,
		defaultChatSettings,
		emptyStore,
		needsNewThread,
		newThread,
		readChatPanels,
		readChatStore,
		retryableTurn,
		settingsDiffer,
		settingsReady,
		sortedThreads,
		withActive,
		withThread,
		withoutThread,
		writeChatPanels,
		writeChatStore,
		type ChatPanels,
		type ChatSettings,
		type ChatStore,
		type ChatStorageNotice
	} from '$lib/chat-threads';
	import ChatComposer from '$lib/components/chat/chat-composer.svelte';
	import ChatRail from '$lib/components/chat/chat-rail.svelte';
	import ChatSettingsPanel from '$lib/components/chat/chat-settings-panel.svelte';
	import ChatTranscript from '$lib/components/chat/chat-transcript.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import { navDrawer } from '$lib/nav-drawer.svelte';
	import type { Group, Model, Provider } from '$lib/types';

	// The Chat page is the tester an operator opens before pointing a real
	// client at Relo. It stores its conversations in this browser, so the
	// daemon keeps no prompt and no reply: each turn is one ordinary request
	// through the same relay a client uses, and it lands in the log as
	// internal traffic.
	//
	// The page is three panes: the conversations this browser kept on the left,
	// the answer in the middle, and the target being tested on the right. Each
	// side pane docks beside the conversation on a wide screen and opens over it
	// below the dock breakpoint, the same way the console's own rail does.
	let store = $state<ChatStore>(emptyStore());
	let notice = $state<ChatStorageNotice>('');
	let draft = $state('');

	let providers = $state.raw<Provider[]>([]);
	let routes = $state.raw<Group[]>([]);
	let models = $state.raw<Model[]>([]);
	// modelsProviderId is the connection the model list belongs to, so a
	// switch to a conversation on another connection knows to read again.
	let modelsProviderId = $state('');
	let loading = $state(true);
	let catalogFailed = $state('');

	const chatSend = new ChatSender();
	// The sender writes into this page's conversation store and composer, and
	// reads them back the same way, so a stream is followed without the page
	// handing it a copy that would go stale.
	chatSend.bind({
		store: () => store,
		setStore: (next) => (store = next),
		persist: () => persist(),
		clearDraft: () => (draft = ''),
		cancelDraftSave: () => {
			if (draftTimer) {
				clearTimeout(draftTimer);
				draftTimer = undefined;
			}
		}
	});

	// The docked panes are the operator's own layout, remembered across visits.
	// The first visit opens the conversations and leaves the target closed
	// unless the window has room for three columns.
	const firstLayout = readChatPanels(layoutDefaults());
	let railDocked = $state(firstLayout.threads);
	let targetDocked = $state(firstLayout.settings);
	// Below the dock breakpoint each pane opens over the conversation instead,
	// and only one of them is ever over it.
	let railOpen = $state(false);
	let targetOpen = $state(false);

	// Panes move on the shared motion budget: a docked pane re-solves its
	// width, a sheet slides in from its edge, and both swap instantly under
	// reduced motion.
	let reducedMotion = $state(false);
	const paneMs = $derived(reducedMotion ? 0 : 200);

	let scroller = $state<HTMLElement | null>(null);
	let atLatest = $state(true);
	let railToggle = $state<HTMLButtonElement | null>(null);
	let targetToggle = $state<HTMLButtonElement | null>(null);
	// The three panels are handed their focus: the page decides where a
	// keyboard goes when a pane opens or closes.
	let railPanel = $state<{ focusClose: () => void } | null>(null);
	let target = $state<{ focusClose: () => void; focusTarget: () => void } | null>(null);
	let composer = $state<{ focus: () => void } | null>(null);

	let modelRequest = 0;
	let draftTimer: ReturnType<typeof setTimeout> | undefined;

	// The opening prompts test what this page is for: that Relo answers, which
	// model answered, and that a longer reply arrives whole.
	const suggestions = [
		'Explain what Relo does in two sentences.',
		'Name the model that is answering this message.',
		'Count from 1 to 10.'
	];

	const wide = $derived(navDrawer.wide);
	const rail = $derived(sortedThreads(store).filter((entry) => entry.turns.length > 0));
	const thread = $derived(activeThread(store));
	const settings = $derived(thread === null ? defaultChatSettings() : thread.settings);
	const configured = $derived(
		providers
			.filter((provider) => provider.configured)
			.sort((left, right) => left.label.localeCompare(right.label))
	);
	const publishedRoutes = $derived(routes.filter((route) => route.enabled));
	const ready = $derived(settingsReady(settings));
	const locked = $derived(thread !== null && needsNewThread(thread));
	const empty = $derived(thread === null || thread.turns.length === 0);
	const canSend = $derived(ready && !chatSend.sending && !loading && draft.trim() !== '');
	// A suggestion is sent whole, so the chips in the opening state wait for a
	// ready target before they appear.
	const canSuggest = $derived(ready && !chatSend.sending && !loading);
	// One union rather than two flags: the composer offers a stop only for
	// the conversation being answered, and only disables its send for any
	// request in flight.
	const composerActivity = $derived(
		!chatSend.sending
			? 'idle'
			: chatSend.inflightThreadId === store.activeThreadId
				? 'this'
				: 'other'
	);

	const usesRoute = $derived(settings.mode === 'format' && settings.target === 'route');
	const providerLabel = $derived(
		providers.find((provider) => provider.id === settings.providerId)?.label ?? ''
	);
	const modelLabel = $derived(
		models.find((model) => model.model_id === settings.modelId)?.name || settings.modelId
	);
	const routeLabel = $derived(
		routes.find((route) => route.id === settings.routeId)?.label || settings.routeId
	);
	const formatLabel = $derived(
		settings.format === 'openai-responses'
			? $t('ui.pages.chatPage.formatResponses')
			: settings.format === 'anthropic'
				? $t('ui.pages.chatPage.formatMessages')
				: $t('ui.pages.chatPage.formatChat')
	);
	// The header names what a request would call, so the answer above it is
	// always read with the target it came from.
	const targetTitle = $derived(
		loading
			? $t('ui.common.loading')
			: !ready
				? $t('ui.pages.chatPage.testSettings')
				: usesRoute
					? routeLabel
					: modelLabel
	);
	const targetDetail = $derived(
		loading || !ready ? '' : settings.mode === 'format' ? formatLabel : providerLabel
	);
	// The header names the conversation on screen, so its title leads and the
	// target it calls sits in the composer beside the button that sends.
	const threadTitle = $derived(
		thread === null || thread.title === '' ? $t('ui.pages.chatPage.untitled') : thread.title
	);
	const railCloseIcon = $derived(wide ? 'layout-sidebar' : 'x');
	const targetCloseIcon = $derived(wide ? 'layout-sidebar-right' : 'x');
	const railCloseLabel = $derived(
		wide ? $t('ui.pages.chatPage.hideThreads') : $t('ui.common.close')
	);
	const targetCloseLabel = $derived(
		wide ? $t('ui.pages.chatPage.hideSettings') : $t('ui.common.close')
	);

	// The target opens only where three columns fit without squeezing the answer.
	function layoutDefaults(): ChatPanels {
		const room = typeof window !== 'undefined' && window.matchMedia('(min-width: 1280px)').matches;
		return { threads: true, settings: room };
	}

	function saveLayout() {
		writeChatPanels({ threads: railDocked, settings: targetDocked });
	}

	function suggest(text: string) {
		if (!canSuggest) return;
		void chatSend.send(text);
	}

	// The composer's text belongs to the conversation on screen, so switching
	// conversations swaps the draft with it. The stored copy is the truth, so
	// every path that moves the active conversation runs this.
	function followActiveThread() {
		const active = activeThread(store);
		draft = active === null ? '' : active.draft;
		// The draft that was being typed belongs to the conversation just left,
		// so a save still waiting on its timer is written now.
		if (draftTimer) {
			clearTimeout(draftTimer);
			draftTimer = undefined;
			persist();
		}
	}

	// Growing the window past the dock breakpoint replaces the drawers with the
	// docked panes, so one left open does not stay over the conversation when
	// the window shrinks back.
	function trackDock(_node: HTMLElement) {
		const docked = window.matchMedia('(min-width: 1024px)');
		const follow = () => {
			if (!docked.matches) return;
			railOpen = false;
			targetOpen = false;
		};
		docked.addEventListener('change', follow);
		return () => docked.removeEventListener('change', follow);
	}

	// The newest line stays in view when a turn is added or finishes. The
	// scroll is instant rather than smooth, so it holds under reduced motion.
	$effect(() => {
		if (scroller === null) return;
		const count = thread === null ? 0 : thread.turns.length;
		void count;
		void (thread === null ? '' : (thread.turns[count - 1]?.id ?? ''));
		void chatSend.sending;
		scroller.scrollTop = scroller.scrollHeight;
		atLatest = true;
	});

	// A reply still being written is followed too, but only while the reader is
	// at the latest line: someone who scrolled up to read keeps their place and
	// returns with the button the page already shows.
	$effect(() => {
		if (scroller === null || !chatSend.sending || !atLatest) return;
		const turns = thread === null ? [] : thread.turns;
		const last = turns[turns.length - 1];
		if (last === undefined || last.result.status !== 'pending') return;
		void last.result.text.length;
		scroller.scrollTop = scroller.scrollHeight;
	});

	onMount(() => {
		const motion = window.matchMedia('(prefers-reduced-motion: reduce)');
		const syncMotion = () => (reducedMotion = motion.matches);
		syncMotion();
		motion.addEventListener('change', syncMotion);
		const loaded = readChatStore();
		store = loaded.store;
		notice = loaded.notice;
		void loadCatalog();
		return () => {
			motion.removeEventListener('change', syncMotion);
			if (draftTimer) clearTimeout(draftTimer);
			// The sender's own timers and any request still running go with the
			// page; the history is written first.
			chatSend.abort();
			persist();
		};
	});

	async function loadCatalog() {
		loading = true;
		catalogFailed = '';
		// The two catalogs are separate reads, so they go out together rather
		// than one after the other.
		const [connections, routeList] = await Promise.allSettled([
			api<{ items: Provider[] }>('/connections'),
			api<{ items: Group[] }>('/routes')
		]);
		if (connections.status === 'fulfilled') providers = connections.value.items ?? [];
		else catalogFailed = message(connections.reason);
		routes = routeList.status === 'fulfilled' ? (routeList.value.items ?? []) : [];
		seedActiveThread();
		const current = activeThread(store);
		await loadModels(current === null ? '' : current.settings.providerId);
		loading = false;
	}

	// A conversation that has not sent anything is pointed at a connection, a
	// model, and a route that exist.
	function seedActiveThread() {
		store = withFallbackThread(store, defaultChatSettings());
		const current = activeThread(store);
		if (current === null || current.turns.length > 0) return;
		const seeded = seedSettings(current.settings);
		if (settingsDiffer(seeded, current.settings)) commitSettings(seeded);
		followActiveThread();
	}

	// A conversation with nothing in it is not stored, so the last one deleted
	// still leaves the settings the operator was testing with.
	function withFallbackThread(next: ChatStore, settings: ChatSettings): ChatStore {
		if (activeThread(next) !== null) return next;
		const fresh = newThread(seedSettings(settings));
		return withActive(withThread(next, fresh), fresh.id);
	}

	function seedSettings(settings: ChatSettings): ChatSettings {
		const providerId = configured.some((provider) => provider.id === settings.providerId)
			? settings.providerId
			: (configured[0]?.id ?? '');
		const routeId = publishedRoutes.some((route) => route.id === settings.routeId)
			? settings.routeId
			: (publishedRoutes[0]?.id ?? '');
		return { ...settings, providerId, routeId };
	}

	// A conversation that already sent a turn keeps the target it was tested
	// with, so a change starts a new conversation and the old one stays.
	function commitSettings(next: ChatSettings) {
		const current = activeThread(store);
		if (current === null) return;
		if (needsNewThread(current)) {
			const fresh = newThread(next);
			store = withActive(withThread(store, fresh), fresh.id);
			followActiveThread();
			railOpen = false;
			targetOpen = false;
			persist();
			return;
		}
		store = withThread(store, { ...current, settings: next });
		persist();
	}

	function changeSettings(next: ChatSettings) {
		const current = activeThread(store);
		if (current === null) return;
		const movedProvider = next.providerId !== current.settings.providerId;
		commitSettings(next);
		if (movedProvider) void ensureModels();
	}

	async function ensureModels() {
		const current = activeThread(store);
		if (current === null) return;
		const providerId = current.settings.providerId;
		if (providerId === modelsProviderId) return;
		const listed = await loadModels(providerId);
		const after = activeThread(store);
		if (after === null || after.settings.providerId !== providerId) return;
		if (listed.some((model) => model.model_id === after.settings.modelId && model.enabled)) return;
		commitSettings({ ...after.settings, modelId: firstEnabled(listed) });
	}

	function firstEnabled(listed: Model[]): string {
		return listed.find((model) => model.enabled)?.model_id ?? '';
	}

	async function loadModels(providerId: string): Promise<Model[]> {
		const token = ++modelRequest;
		let listed: Model[] = [];
		if (providerId !== '') {
			try {
				const data = await api<{ items: Model[] }>(
					'/models?provider=' + encodeURIComponent(providerId) + '&limit=500'
				);
				listed = data.items ?? [];
			} catch {
				listed = [];
			}
		}
		if (token !== modelRequest) return listed;
		models = listed;
		modelsProviderId = providerId;
		return listed;
	}

	// Where a keyboard lands once the conversation on screen changes: in the
	// composer where there is room for it, and back on the list it was chosen
	// from on a narrow screen.
	function focusAfterSwitch(returnToRail: boolean) {
		void tick().then(() => {
			if (wide) composer?.focus();
			else if (returnToRail) railToggle?.focus();
		});
	}

	function selectThread(id: string) {
		const wasDrawer = !wide && railOpen;
		store = withActive(store, id);
		followActiveThread();
		railOpen = false;
		void ensureModels();
		focusAfterSwitch(wasDrawer);
	}

	function newChat() {
		const current = activeThread(store);
		const settings = current === null ? seedSettings(defaultChatSettings()) : current.settings;
		// A conversation with nothing in it is not something to keep, so the
		// blank one that "New chat" leaves behind is dropped as the next one
		// takes its place.
		const kept: ChatStore = {
			...store,
			threads: store.threads.filter((entry) => entry.turns.length > 0)
		};
		const fresh = newThread(settings);
		const wasDrawer = !wide && railOpen;
		store = withActive(withThread(kept, fresh), fresh.id);
		followActiveThread();
		persist();
		railOpen = false;
		focusAfterSwitch(wasDrawer);
	}

	function renameThread(id: string, title: string) {
		const target = store.threads.find((entry) => entry.id === id);
		if (target === undefined) return;
		store = withThread(store, { ...target, title });
		persist();
	}

	function deleteThread(id: string) {
		if (chatSend.inflightThreadId === id) chatSend.abort();
		const gone = store.threads.find((entry) => entry.id === id);
		store = withFallbackThread(
			withoutThread(store, id),
			gone === undefined ? defaultChatSettings() : gone.settings
		);
		followActiveThread();
		persist();
	}

	function clearAll() {
		chatSend.abort();
		const current = activeThread(store);
		store = withFallbackThread(
			emptyStore(),
			current === null ? defaultChatSettings() : current.settings
		);
		followActiveThread();
		if (clearChatStore()) notice = '';
		else if (notice === '') notice = 'blocked';
		railOpen = false;
		targetOpen = false;
	}

	// A stored value the console could not read is left where it is, so a
	// recoverable mistake is never overwritten.
	function persist() {
		if (notice !== '') return;
		if (!writeChatStore(store)) notice = 'blocked';
	}

	function sendDraft() {
		void chatSend.send(draft);
	}

	function retry() {
		const current = activeThread(store);
		if (current === null) return;
		const turn = retryableTurn(current);
		if (turn === null) return;
		void chatSend.send(turn.prompt, turn.id);
	}

	function message(error: unknown): string {
		return error instanceof Error ? error.message : String(error);
	}

	// A reader who scrolled up keeps their place; the button waits until they
	// are far enough from the end that returning matters.
	function onscroll() {
		if (scroller === null) return;
		atLatest = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < 200;
	}

	function scrollToLatest() {
		if (scroller === null) return;
		scroller.scrollTop = scroller.scrollHeight;
		atLatest = true;
	}

	// The two controls act on whichever pane the size calls for: the docked one
	// takes its width from the page, and the narrow-screen one opens over the
	// conversation with the caret handed to its own close button.
	function toggleRail() {
		if (wide) {
			const opening = !railDocked;
			railDocked = opening;
			saveLayout();
			void tick().then(() => (opening ? railPanel?.focusClose() : railToggle?.focus()));
			return;
		}
		targetOpen = false;
		railOpen = true;
		void tick().then(() => railPanel?.focusClose());
	}

	function closeRail() {
		if (wide) {
			railDocked = false;
			saveLayout();
			void tick().then(() => railToggle?.focus());
			return;
		}
		railOpen = false;
		void tick().then(() => railToggle?.focus());
	}

	function toggleTarget() {
		if (wide) {
			const opening = !targetDocked;
			targetDocked = opening;
			saveLayout();
			void tick().then(() => (opening ? target?.focusTarget() : targetToggle?.focus()));
			return;
		}
		railOpen = false;
		targetOpen = true;
		void tick().then(() => target?.focusTarget());
	}

	function closeTarget() {
		if (wide) {
			targetDocked = false;
			saveLayout();
			void tick().then(() => targetToggle?.focus());
			return;
		}
		targetOpen = false;
		void tick().then(() => targetToggle?.focus());
	}

	// The header names the target, so pressing it opens the pane that changes
	// it and puts the caret on the choice itself.
	function revealTarget() {
		if (wide) {
			if (!targetDocked) {
				targetDocked = true;
				saveLayout();
			}
			void tick().then(() => target?.focusTarget());
			return;
		}
		railOpen = false;
		targetOpen = true;
		void tick().then(() => target?.focusTarget());
	}

	function closeDrawers() {
		if (railOpen) {
			closeRail();
			return;
		}
		closeTarget();
	}

	function onKeydown(event: KeyboardEvent) {
		if (event.key !== 'Escape' || wide) return;
		if (railOpen || targetOpen) closeDrawers();
	}
</script>

<svelte:head><title>{$t('ui.pages.chatPage.title')} · Relo</title></svelte:head>

<!-- A drawer over the conversation is modal, so Escape closes it wherever
     the focus happens to be. The handler stands down on a wide screen,
     where the panes are docked and nothing is being dismissed. -->
<svelte:window onkeydown={onKeydown} />

<div {@attach trackDock} class="relative flex h-full min-h-0 gap-4 overflow-hidden bg-canvas p-4">
	{#snippet railPane()}
		<div
			class="flex h-full w-full shrink-0 flex-col overflow-hidden rounded-card border border-line bg-sunken"
		>
			<ChatRail
				bind:this={railPanel}
				threads={rail}
				activeId={store.activeThreadId}
				closeIcon={railCloseIcon}
				closeLabel={railCloseLabel}
				onselect={selectThread}
				onnew={newChat}
				onrename={renameThread}
				ondelete={deleteThread}
				onclear={clearAll}
				onclose={closeRail}
			/>
		</div>
	{/snippet}

	{#snippet targetPane()}
		<div
			class="flex h-full w-full shrink-0 flex-col overflow-hidden rounded-card border border-line bg-sunken"
		>
			<ChatSettingsPanel
				bind:this={target}
				{settings}
				providers={configured}
				{models}
				routes={publishedRoutes}
				{loading}
				{locked}
				closeIcon={targetCloseIcon}
				closeLabel={targetCloseLabel}
				onclose={closeTarget}
				onchange={changeSettings}
			/>
		</div>
	{/snippet}

	{#if !wide && (railOpen || targetOpen)}
		<button
			type="button"
			data-plain
			aria-label={$t('ui.common.close')}
			class="absolute inset-0 z-20 bg-[var(--scrim)]"
			transition:fade={{ duration: paneMs }}
			onclick={closeDrawers}
		></button>
	{/if}

	{#if wide && railDocked}
		<aside class="flex h-full w-72 shrink-0" transition:slide={{ axis: 'x', duration: paneMs }}>
			{@render railPane()}
		</aside>
	{:else if !wide && railOpen}
		<aside
			class="absolute inset-y-3 left-3 z-30 flex w-[min(20rem,calc(100vw-2rem))] shrink-0 shadow-[var(--shadow-modal)]"
			transition:fly={{ x: -340, duration: paneMs }}
		>
			{@render railPane()}
		</aside>
	{/if}

	<div
		class="flex min-w-0 flex-1 flex-col overflow-hidden rounded-card border border-line bg-sunken"
		inert={!wide && (railOpen || targetOpen)}
	>
		<header class="flex h-14 shrink-0 items-center gap-2 px-4">
			{#if !wide || !railDocked}
				<Button
					bind:ref={railToggle}
					variant="ghost"
					size="icon"
					aria-label={$t('ui.pages.chatPage.showThreads')}
					title={$t('ui.pages.chatPage.showThreads')}
					onclick={toggleRail}
				>
					<Icon name="layout-sidebar" size={16} />
				</Button>
			{/if}

			<h1 class="min-w-0 flex-1 truncate text-[15px] font-semibold text-ink">{threadTitle}</h1>

			<Button
				variant="outline"
				size="icon"
				aria-label={$t('ui.pages.chatPage.newChat')}
				title={$t('ui.pages.chatPage.newChat')}
				onclick={newChat}
			>
				<Icon name="edit" size={16} />
			</Button>

			{#if !wide || !targetDocked}
				<Button
					bind:ref={targetToggle}
					variant="ghost"
					size="icon"
					aria-label={$t('ui.pages.chatPage.showSettings')}
					title={$t('ui.pages.chatPage.showSettings')}
					onclick={toggleTarget}
				>
					<Icon name="layout-sidebar-right" size={16} />
				</Button>
			{/if}
		</header>

		{#if notice !== '' || catalogFailed !== ''}
			<div class="mx-auto w-full max-w-3xl shrink-0 space-y-2 px-4 pb-3">
				{#if catalogFailed !== ''}
					<p
						class="flex items-start gap-2 rounded-card border border-danger/30 bg-danger/5 px-4 py-3 text-sm text-danger"
						role="alert"
					>
						<Icon name="circle-alert" size={15} class="mt-0.5 shrink-0" />
						<span class="min-w-0 flex-1">
							{$t('ui.pages.chatPage.loadFailed', { values: { detail: catalogFailed } })}
						</span>
					</p>
				{/if}
				{#if notice !== ''}
					<div
						class="flex flex-wrap items-center gap-3 rounded-card border border-warn/40 bg-warn/10 px-4 py-3 text-sm text-warn"
					>
						<Icon name="alert-triangle" size={15} class="shrink-0" />
						<p class="min-w-0 flex-1">
							{notice === 'corrupt'
								? $t('ui.pages.chatPage.storageCorrupt')
								: $t('ui.pages.chatPage.storageBlocked')}
						</p>
						{#if notice === 'corrupt'}
							<Button variant="outline" size="sm" onclick={clearAll}>
								<Icon name="trash" size={14} />
								{$t('ui.pages.chatPage.storageClear')}
							</Button>
						{/if}
					</div>
				{/if}
			</div>
		{/if}

		<div class="relative min-h-0 flex-1">
			<div bind:this={scroller} {onscroll} class="h-full overflow-y-auto px-4">
				{#if empty}
					<!-- The opening words sit in the scroll area with the suggestions
					     under them; the composer keeps the foot of the column. -->
					<div
						class="flex min-h-full flex-col items-center justify-center gap-4 px-2 py-6 text-center"
					>
						<h2 class="text-2xl font-semibold text-ink">
							{$t('ui.pages.chatPage.greeting')}
						</h2>
						<p class="max-w-md text-sm leading-relaxed text-muted-foreground">
							{$t('ui.pages.chatPage.greetingHint')}
						</p>
						<div class="flex flex-wrap items-center justify-center gap-2">
							{#each suggestions as suggestion (suggestion)}
								<Button
									variant="outline"
									size="sm"
									disabled={!canSuggest}
									onclick={() => suggest(suggestion)}
								>
									<Icon name="sparkles" size={14} />
									{suggestion}
								</Button>
							{/each}
						</div>
					</div>
				{:else}
					<div class="py-2 pb-6">
						<ChatTranscript {thread} sending={chatSend.sending} onretry={retry} />
					</div>
				{/if}
			</div>
		</div>

		<!-- The composer sits in the flow under the answer: it never floats
		     over the conversation. -->
		<div class="shrink-0 px-4 pt-3 pb-4">
			{#if !atLatest && !empty}
				<div class="mx-auto mb-2 flex w-full max-w-3xl justify-center">
					<Button
						variant="outline"
						size="icon"
						aria-label={$t('ui.pages.chatPage.scrollToBottom')}
						title={$t('ui.pages.chatPage.scrollToBottom')}
						onclick={scrollToLatest}
					>
						<Icon name="arrow-down" size={16} />
					</Button>
				</div>
			{/if}

			<div class="mx-auto w-full max-w-3xl">
				<ChatComposer
					bind:this={composer}
					bind:value={draft}
					disabled={loading}
					activity={composerActivity}
					sendable={canSend}
					placeholder={$t('ui.pages.chatPage.composerPlaceholder')}
					{targetTitle}
					{targetDetail}
					{locked}
					onsend={sendDraft}
					onstop={() => chatSend.stop()}
					ontarget={revealTarget}
				/>
				<p class="mt-2 text-center text-xs text-faint">
					{$t('ui.pages.chatPage.localNote')}
				</p>
			</div>
		</div>
	</div>

	<!-- The target this conversation is tested with. Docked beside the answer
	     on a wide screen, and a sheet sliding over it everywhere else. -->
	{#if wide && targetDocked}
		<aside class="flex h-full w-80 shrink-0" transition:slide={{ axis: 'x', duration: paneMs }}>
			{@render targetPane()}
		</aside>
	{:else if !wide && targetOpen}
		<aside
			class="absolute inset-y-3 right-3 z-30 flex w-[min(20rem,calc(100vw-2rem))] shrink-0 shadow-[var(--shadow-modal)]"
			transition:fly={{ x: 340, duration: paneMs }}
		>
			{@render targetPane()}
		</aside>
	{/if}
</div>
