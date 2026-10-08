<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';

	import { groupThreads, type ChatThread } from '$lib/chat-threads';
	import ConfirmDialog from '$lib/components/ui/confirm-dialog.svelte';
	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import { DropdownMenu } from 'bits-ui';
	import MenuPanel from '$lib/components/ui/menu-panel.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import { relativeTime } from '$lib/format';

	// ChatRail is the list of conversations stored in this browser: the one
	// surface that starts a conversation, names one, or drops one. It sits
	// itself under the window's own headings, so a list that outgrows a day is
	// still read by when it was last touched. The two destructive actions wait
	// behind a menu and a confirmation, because a conversation cannot be
	// recovered once its thread is gone.
	interface Props {
		threads: ChatThread[];
		activeId: string;
		closeIcon: IconName;
		closeLabel: string;
		onselect: (id: string) => void;
		onnew: () => void;
		onrename: (id: string, title: string) => void;
		ondelete: (id: string) => void;
		onclear: () => void;
		onclose: () => void;
	}

	let {
		threads,
		activeId,
		closeIcon,
		closeLabel,
		onselect,
		onnew,
		onrename,
		ondelete,
		onclear,
		onclose
	}: Props = $props();

	const GROUP_LABEL = {
		today: 'ui.pages.chatPage.groupToday',
		yesterday: 'ui.pages.chatPage.groupYesterday',
		week: 'ui.pages.chatPage.groupWeek',
		older: 'ui.pages.chatPage.groupOlder'
	} as const;

	let query = $state('');
	let editingId = $state('');
	let titleDraft = $state('');
	let closeButton = $state<HTMLButtonElement | null>(null);
	let menuId = $state('');
	let confirmKind = $state<'' | 'delete' | 'clear'>('');
	let confirmOpen = $state(false);
	let pendingId = $state('');
	let coarse = $state(false);

	// The search narrows the list by title and keeps its own grouping, so a
	// filtered list reads the same way a whole one does.
	const shown = $derived(
		threads.filter((entry) => entry.title.toLowerCase().includes(query.trim().toLowerCase()))
	);
	const groups = $derived(groupThreads(shown));

	onMount(() => {
		const pointer = window.matchMedia('(pointer: coarse)');
		const sync = () => (coarse = pointer.matches);
		sync();
		pointer.addEventListener('change', sync);
		return () => pointer.removeEventListener('change', sync);
	});

	// focusClose hands the caret to the control that closes this list, which is
	// where the page sends focus when the panel opens.
	export function focusClose() {
		closeButton?.focus();
	}

	// A rename is an edit in place, so the field takes focus the moment it
	// appears; without it the operator would have to reach for the field the
	// menu just opened.
	function focusRename(node: HTMLInputElement) {
		node.focus();
	}

	// A row's menu is in the open on the conversation being read and on a touch
	// surface; elsewhere it waits for the pointer, without the row shifting.
	function menuClass(id: string): string {
		if (coarse || id === menuId || id === activeId) return '';
		return 'opacity-0 transition-opacity duration-150 group-focus-within:opacity-100 group-hover:opacity-100';
	}

	function rowTitle(thread: ChatThread): string {
		return (
			$t('ui.pages.chatPage.turnCount', { values: { count: thread.turns.length } }) +
			' · ' +
			relativeTime(thread.updatedAt)
		);
	}

	function startRename(thread: ChatThread) {
		editingId = thread.id;
		titleDraft = thread.title;
	}

	function commitRename(id: string) {
		if (editingId !== id) return;
		const title = titleDraft.trim();
		editingId = '';
		onrename(id, title);
	}

	function onRenameKeydown(event: KeyboardEvent, id: string) {
		if (event.key === 'Enter') commitRename(id);
		if (event.key === 'Escape') editingId = '';
	}

	// The menu would hand focus back to the row it was opened from, which would
	// take it off the field a rename is being typed into.
	function onCloseAutoFocus(event: Event) {
		if (editingId !== '') event.preventDefault();
	}

	function askDelete(id: string) {
		pendingId = id;
		confirmKind = 'delete';
		confirmOpen = true;
	}

	function askClear() {
		confirmKind = 'clear';
		confirmOpen = true;
	}

	function confirm() {
		const kind = confirmKind;
		const id = pendingId;
		confirmKind = '';
		pendingId = '';
		confirmOpen = false;
		if (kind === 'delete') ondelete(id);
		if (kind === 'clear') onclear();
	}
</script>

<div class="flex h-full min-h-0 flex-col">
	<div class="flex shrink-0 items-center gap-1 px-4 pt-4">
		<h2 class="min-w-0 flex-1 truncate text-[15px] font-semibold text-ink">
			{$t('ui.pages.chatPage.threads')}
		</h2>
		<span class="mono-data text-faint" aria-hidden="true">{threads.length}</span>
		<DropdownMenu.Root>
			<DropdownMenu.Trigger>
				{#snippet child({ props })}
					<Button variant="ghost" size="icon" {...props} aria-label={$t('ui.common.actions')}>
						<Icon name="dots" size={16} />
					</Button>
				{/snippet}
			</DropdownMenu.Trigger>
			<DropdownMenu.Portal>
				<MenuPanel align="end" class="z-50 min-w-44 surface-pop p-1">
					<DropdownMenu.Item
						class="flex min-h-control items-center gap-2 rounded-control px-2 py-1.5 text-sm text-danger outline-none data-highlighted:bg-hover disabled:pointer-events-none disabled:opacity-50"
						disabled={threads.length === 0}
						onSelect={askClear}
					>
						<Icon name="trash" size={14} />
						{$t('ui.pages.chatPage.clearAll')}
					</DropdownMenu.Item>
				</MenuPanel>
			</DropdownMenu.Portal>
		</DropdownMenu.Root>
		<Button
			bind:ref={closeButton}
			variant="ghost"
			size="icon"
			aria-label={closeLabel}
			title={closeLabel}
			onclick={onclose}
		>
			<Icon name={closeIcon} size={16} />
		</Button>
	</div>

	<div class="shrink-0 px-3 pt-3">
		<Button variant="outline" class="w-full justify-center gap-2" onclick={onnew}>
			<Icon name="edit" size={15} />
			{$t('ui.pages.chatPage.newChat')}
		</Button>
	</div>

	<div class="shrink-0 px-3 pt-2">
		<Input
			type="search"
			bind:value={query}
			placeholder={$t('ui.pages.chatPage.searchThreads')}
			aria-label={$t('ui.pages.chatPage.searchThreads')}
		/>
	</div>

	<div class="min-h-0 flex-1 overflow-y-auto px-3 pt-3 pb-4">
		{#if threads.length === 0}
			<div
				class="empty-panel rounded-panel border-[1.5px] border-dashed border-accent-border bg-accent-soft text-sm"
			>
				<p>{$t('ui.pages.chatPage.threadsEmpty')}</p>
			</div>
		{:else if shown.length === 0}
			<p class="px-2 py-8 text-center text-xs text-muted-foreground">
				{$t('ui.pages.chatPage.searchNoMatch')}
			</p>
		{:else}
			{#each groups as group (group.key)}
				<div class="mb-2">
					<h3 class="px-2 pb-1 text-xs font-medium text-faint">
						{$t(GROUP_LABEL[group.key])}
					</h3>
					<ul class="space-y-0.5">
						{#each group.threads as thread (thread.id)}
							<li
								class="group flex items-center gap-0.5 rounded-control pr-0.5 {thread.id ===
								activeId
									? 'bg-accent-soft shadow-[0_4px_14px_var(--accent-glow)]'
									: 'hover:bg-hover'}"
							>
								{#if editingId === thread.id}
									<input
										{@attach focusRename}
										bind:value={titleDraft}
										class="my-1 ml-1 h-control min-w-0 flex-1 rounded-control border border-line bg-surface px-2 text-sm text-ink outline-none"
										aria-label={$t('ui.pages.chatPage.rename')}
										onkeydown={(event) => onRenameKeydown(event, thread.id)}
										onblur={() => commitRename(thread.id)}
									/>
								{:else}
									<button
										type="button"
										data-plain
										class="min-w-0 flex-1 rounded-control px-2.5 py-2 text-left"
										aria-current={thread.id === activeId ? 'true' : undefined}
										title={rowTitle(thread)}
										onclick={() => onselect(thread.id)}
									>
										<span
											class="block truncate text-sm font-medium {thread.id === activeId
												? 'text-accent-ink'
												: 'text-ink'}"
										>
											{thread.title || $t('ui.pages.chatPage.untitled')}
										</span>
										<span class="mono-data block truncate text-faint">
											{relativeTime(thread.updatedAt)}
										</span>
									</button>
									<div class="shrink-0 {menuClass(thread.id)}">
										<DropdownMenu.Root
											onOpenChange={(open) => {
												if (open) menuId = thread.id;
												else if (menuId === thread.id) menuId = '';
											}}
										>
											<DropdownMenu.Trigger>
												{#snippet child({ props })}
													<Button
														variant="ghost"
														size="icon"
														{...props}
														aria-label={$t('ui.common.actions')}
													>
														<Icon name="dots" size={14} />
													</Button>
												{/snippet}
											</DropdownMenu.Trigger>
											<DropdownMenu.Portal>
												<MenuPanel
													align="start"
													class="z-50 min-w-40 surface-pop p-1"
													{onCloseAutoFocus}
												>
													<DropdownMenu.Item
														class="flex min-h-control items-center gap-2 rounded-control px-2 py-1.5 text-sm outline-none data-highlighted:bg-hover"
														onSelect={() => startRename(thread)}
													>
														<Icon name="pencil" size={14} />
														{$t('ui.pages.chatPage.rename')}
													</DropdownMenu.Item>
													<DropdownMenu.Item
														class="flex min-h-control items-center gap-2 rounded-control px-2 py-1.5 text-sm text-danger outline-none data-highlighted:bg-hover"
														onSelect={() => askDelete(thread.id)}
													>
														<Icon name="trash" size={14} />
														{$t('ui.pages.chatPage.delete')}
													</DropdownMenu.Item>
												</MenuPanel>
											</DropdownMenu.Portal>
										</DropdownMenu.Root>
									</div>
								{/if}
							</li>
						{/each}
					</ul>
				</div>
			{/each}
		{/if}
	</div>
</div>

<ConfirmDialog
	bind:open={confirmOpen}
	title={confirmKind === 'delete'
		? $t('ui.pages.chatPage.deleteTitle')
		: $t('ui.pages.chatPage.clearAllTitle')}
	body={confirmKind === 'delete'
		? $t('ui.pages.chatPage.deleteBody')
		: $t('ui.pages.chatPage.clearAllBody')}
	confirmLabel={confirmKind === 'delete'
		? $t('ui.pages.chatPage.delete')
		: $t('ui.pages.chatPage.clearAll')}
	tone="destructive"
	icon="trash"
	onconfirm={confirm}
/>
