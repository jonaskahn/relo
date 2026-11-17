<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';

	import { api } from '$lib/api';
	import CenteredModal from '$lib/components/ui/centered-modal.svelte';
	import ConfirmDialog from '$lib/components/ui/confirm-dialog.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import Field from '$lib/components/ui/field.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import Select from '$lib/components/ui/select.svelte';
	import StatusBadge from '$lib/components/ui/status-badge.svelte';
	import { parseProxyLine, parseProxyPaste, redactProxy, type ProxyLine } from '$lib/proxy-pool';
	import type { ProxyPoolEntry, ProxyPoolPage, ProxyPoolSummary } from '$lib/types';

	interface Props {
		open?: boolean;
		summary: ProxyPoolSummary;
		onchanged?: (next: ProxyPoolSummary) => void;
	}

	let { open = $bindable(false), summary, onchanged }: Props = $props();

	let items = $state<ProxyPoolEntry[]>([]);
	let pool = $state<ProxyPoolSummary>({ ...untrack(() => summary) });
	let loading = $state(false);
	let error = $state('');
	let search = $state('');
	let status = $state('all');
	let cursor = $state('');
	let nextCursor = $state('');
	let paste = $state('');
	let preview = $state<ProxyLine[]>([]);
	let editingId = $state('');
	let editLabel = $state('');
	let editPassword = $state('');
	let deleteTarget = $state<ProxyPoolEntry | null>(null);
	let deleteOpen = $state(false);
	let busy = $state(false);
	let fileInput: HTMLInputElement | null = $state(null);

	const statusOptions = $derived([
		{ value: 'all', label: $t('ui.settingsPage.proxies.statusAll') },
		{ value: 'enabled', label: $t('ui.settingsPage.proxies.statusEnabled') },
		{ value: 'cooling', label: $t('ui.settingsPage.proxies.statusCooling') },
		{ value: 'disabled', label: $t('ui.settingsPage.proxies.statusDisabled') }
	]);

	$effect(() => {
		if (open) void load('');
	});

	async function load(next: string) {
		loading = true;
		error = '';
		try {
			const query = new URLSearchParams({ limit: '50' });
			if (search.trim() !== '') query.set('q', search.trim());
			if (status !== 'all') query.set('status', status);
			if (next !== '') query.set('cursor', next);
			const page = await api<ProxyPoolPage>('/settings/proxies?' + query.toString());
			items = page.items;
			nextCursor = page.next_cursor;
			cursor = next;
			await refreshSummary();
		} catch (failure) {
			error = failure instanceof Error ? failure.message : String(failure);
		} finally {
			loading = false;
		}
	}

	async function refreshSummary() {
		try {
			const settings = await api<{ providers: { proxy_pool: ProxyPoolSummary } }>('/settings');
			pool = settings.providers.proxy_pool;
			onchanged?.(pool);
		} catch {
			// The row list is the source this modal just read; a failed summary
			// refresh is answered by the next page the operator opens.
		}
	}

	function updatePreview() {
		preview = parseProxyPaste(
			paste,
			items.map((item) => `${item.scheme}://${item.username}@${item.host}:${item.port}`)
		);
	}

	async function importLines(lines: ProxyLine[]) {
		const ready = lines.filter((line) => line.status === 'ready');
		if (ready.length === 0) return;
		busy = true;
		error = '';
		try {
			await api('/settings/proxies/import', {
				method: 'POST',
				body: JSON.stringify({ text: ready.map((line) => line.raw).join('\n') })
			});
			paste = '';
			preview = [];
			toast.success($t('ui.settingsPage.proxies.imported', { values: { count: ready.length } }));
			await load('');
		} catch (failure) {
			error = failure instanceof Error ? failure.message : String(failure);
		} finally {
			busy = false;
		}
	}

	async function addOne() {
		const parsed = parseProxyLine(paste.trim());
		if (parsed === null) {
			error = $t('ui.settingsPage.proxies.invalidLine');
			return;
		}
		busy = true;
		error = '';
		try {
			await api('/settings/proxies', {
				method: 'POST',
				body: JSON.stringify({
					label: parsed.label,
					scheme: parsed.scheme,
					host: parsed.host,
					port: parsed.port,
					username: parsed.username,
					password: parsed.password,
					enabled: true,
					rank: items.length
				})
			});
			paste = '';
			preview = [];
			toast.success($t('ui.settingsPage.proxies.added'));
			await load('');
		} catch (failure) {
			error = failure instanceof Error ? failure.message : String(failure);
		} finally {
			busy = false;
		}
	}

	async function readFile(event: Event) {
		const input = event.currentTarget as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;
		paste = await file.text();
		updatePreview();
		input.value = '';
	}

	async function toggle(item: ProxyPoolEntry) {
		busy = true;
		try {
			await api('/settings/proxies/' + encodeURIComponent(item.id), {
				method: 'PATCH',
				body: JSON.stringify({ enabled: !item.enabled })
			});
			toast.success($t('ui.settingsPage.proxies.saved'));
			await load(cursor);
		} catch (failure) {
			error = failure instanceof Error ? failure.message : String(failure);
		} finally {
			busy = false;
		}
	}

	function beginEdit(item: ProxyPoolEntry) {
		editingId = item.id;
		editLabel = item.label;
		editPassword = '';
	}

	async function saveEdit(item: ProxyPoolEntry) {
		busy = true;
		try {
			await api('/settings/proxies/' + encodeURIComponent(item.id), {
				method: 'PATCH',
				body: JSON.stringify({ label: editLabel, password: editPassword })
			});
			editingId = '';
			editPassword = '';
			toast.success($t('ui.settingsPage.proxies.saved'));
			await load(cursor);
		} catch (failure) {
			error = failure instanceof Error ? failure.message : String(failure);
		} finally {
			busy = false;
		}
	}

	async function test(item: ProxyPoolEntry) {
		busy = true;
		try {
			const tested = await api<ProxyPoolEntry>(
				'/settings/proxies/' + encodeURIComponent(item.id) + '/test',
				{
					method: 'POST'
				}
			);
			items = items.map((row) => (row.id === tested.id ? tested : row));
			toast.success($t('ui.settingsPage.proxies.tested'));
			await refreshSummary();
		} catch (failure) {
			error = failure instanceof Error ? failure.message : String(failure);
		} finally {
			busy = false;
		}
	}

	async function remove() {
		if (deleteTarget === null) return;
		busy = true;
		try {
			await api('/settings/proxies/' + encodeURIComponent(deleteTarget.id), { method: 'DELETE' });
			toast.success($t('ui.settingsPage.proxies.deleted'));
			deleteTarget = null;
			deleteOpen = false;
			await load(cursor);
		} catch (failure) {
			error = failure instanceof Error ? failure.message : String(failure);
		} finally {
			busy = false;
		}
	}

	function badgeKind(item: ProxyPoolEntry): 'pass' | 'warn' | 'fail' | 'muted' {
		if (item.state === 'ready') return 'pass';
		if (item.state === 'cooling') return 'warn';
		if (item.state === 'unreachable') return 'fail';
		return 'muted';
	}

	function badgeLabel(item: ProxyPoolEntry): string {
		if (item.state === 'cooling' && item.cooling_until) {
			return $t('ui.settingsPage.proxies.coolingUntil', {
				values: { time: new Date(item.cooling_until).toLocaleTimeString() }
			});
		}
		return $t(
			'ui.settingsPage.proxies.state' + item.state.charAt(0).toUpperCase() + item.state.slice(1)
		);
	}
</script>

<CenteredModal
	bind:open
	size="xl"
	title={$t('ui.settingsPage.proxies.title')}
	description={$t('ui.settingsPage.proxies.description')}
>
	<div class="flex flex-col gap-5">
		<div class="flex flex-col gap-2">
			<Field
				id="proxy-paste"
				label={$t('ui.settingsPage.proxies.pasteLabel')}
				hint={$t('ui.settingsPage.proxies.pasteHint')}
			>
				<textarea
					id="proxy-paste"
					class="min-h-24 w-full rounded-control bg-control p-3 font-mono text-sm text-ink shadow-field outline-none focus-visible:ring-[3px] focus-visible:ring-[var(--ring)]"
					bind:value={paste}
					oninput={updatePreview}
					placeholder={'http://user:pass@127.0.0.1:7890\nsocks5://127.0.0.1:1080'}></textarea>
			</Field>
			<div class="flex flex-wrap items-center gap-2">
				<Button type="button" variant="outline" size="sm" onclick={() => fileInput?.click()}>
					{$t('ui.settingsPage.proxies.importFile')}
				</Button>
				<input
					bind:this={fileInput}
					type="file"
					accept=".txt,text/plain"
					class="hidden"
					onchange={readFile}
				/>
				<Button type="button" size="sm" disabled={busy} onclick={addOne}>
					{$t('ui.settingsPage.proxies.add')}
				</Button>
				{#if preview.length > 0}
					<Button
						type="button"
						variant="outline"
						size="sm"
						disabled={busy}
						onclick={() => importLines(preview)}
					>
						{$t('ui.settingsPage.proxies.commit', {
							values: { count: preview.filter((line) => line.status === 'ready').length }
						})}
					</Button>
				{/if}
			</div>
			{#if preview.length > 0}
				<ul class="flex max-h-32 flex-col gap-1 overflow-y-auto text-xs">
					{#each preview as line (line.line)}
						<li class="flex items-center gap-2">
							<StatusBadge
								kind={line.status === 'ready'
									? 'pass'
									: line.status === 'duplicate'
										? 'warn'
										: 'fail'}
								label={$t(
									'ui.settingsPage.proxies.line' +
										line.status.charAt(0).toUpperCase() +
										line.status.slice(1)
								)}
							/>
							<span class="truncate font-mono">{line.raw}</span>
						</li>
					{/each}
				</ul>
			{/if}
		</div>

		<div class="flex flex-wrap items-center gap-2">
			<Input
				class="mono-data max-w-xs"
				bind:value={search}
				placeholder={$t('ui.settingsPage.proxies.search')}
				aria-label={$t('ui.settingsPage.proxies.search')}
			/>
			<Select
				class="w-40"
				bind:value={status}
				options={statusOptions}
				onValueChange={() => load('')}
			/>
			<span class="text-sm text-muted-foreground">
				{$t('ui.settingsPage.proxies.summary', {
					values: { enabled: pool.enabled, cooling: pool.cooling }
				})}
			</span>
			<Button type="button" variant="ghost" size="sm" disabled={loading} onclick={() => load('')}>
				{$t('ui.settingsPage.proxies.reload')}
			</Button>
		</div>

		{#if error}
			<p class="text-sm text-danger" role="alert">{error}</p>
		{/if}

		{#if items.length === 0 && !loading}
			<div
				class="rounded-card border border-dashed border-border p-6 text-center text-sm text-muted-foreground"
			>
				{$t('ui.settingsPage.proxies.empty')}
			</div>
		{:else}
			<ul class="flex flex-col gap-2">
				{#each items as item (item.id)}
					<li class="rounded-card bg-surface p-3 shadow-float-1">
						{#if editingId === item.id}
							<div class="grid gap-2 sm:grid-cols-[1fr_1fr_auto]">
								<Input bind:value={editLabel} aria-label={$t('ui.settingsPage.proxies.label')} />
								<Input
									type="password"
									bind:value={editPassword}
									placeholder={$t('ui.settingsPage.proxies.passwordKeep')}
									aria-label={$t('ui.settingsPage.proxies.password')}
								/>
								<div class="flex gap-2">
									<Button size="sm" disabled={busy} onclick={() => saveEdit(item)}
										>{$t('ui.common.save')}</Button
									>
									<Button variant="ghost" size="sm" onclick={() => (editingId = '')}
										>{$t('ui.common.cancel')}</Button
									>
								</div>
							</div>
						{:else}
							<div class="flex flex-wrap items-center gap-3">
								<div class="min-w-0 flex-1">
									<p class="truncate text-sm font-medium">{item.label}</p>
									<p class="mono-data truncate text-xs text-muted-foreground">
										{redactProxy({
											scheme: item.scheme,
											host: item.host,
											port: item.port,
											username: item.username,
											hasPassword: item.has_password
										})}
									</p>
								</div>
								<StatusBadge kind={badgeKind(item)} label={badgeLabel(item)} />
								<Button variant="ghost" size="sm" onclick={() => beginEdit(item)}
									>{$t('ui.common.edit')}</Button
								>
								<Button variant="ghost" size="sm" disabled={busy} onclick={() => test(item)}
									>{$t('ui.settingsPage.proxies.test')}</Button
								>
								<Button
									variant="ghost"
									size="sm"
									onclick={() => {
										deleteTarget = item;
										deleteOpen = true;
									}}>{$t('ui.common.delete')}</Button
								>
								<Button
									variant={item.enabled ? 'outline' : 'ghost'}
									size="sm"
									disabled={busy}
									onclick={() => toggle(item)}
								>
									{$t(
										item.enabled
											? 'ui.settingsPage.proxies.disable'
											: 'ui.settingsPage.proxies.enable'
									)}
								</Button>
							</div>
						{/if}
					</li>
				{/each}
			</ul>
			<div class="flex items-center justify-between">
				<Button variant="ghost" size="sm" disabled={cursor === ''} onclick={() => load('')}
					>{$t('ui.settingsPage.proxies.previous')}</Button
				>
				<Button
					variant="ghost"
					size="sm"
					disabled={nextCursor === ''}
					onclick={() => load(nextCursor)}>{$t('ui.settingsPage.proxies.next')}</Button
				>
			</div>
		{/if}
	</div>
</CenteredModal>

<ConfirmDialog
	bind:open={deleteOpen}
	title={$t('ui.settingsPage.proxies.deleteTitle')}
	body={$t('ui.settingsPage.proxies.deleteBody', { values: { label: deleteTarget?.label ?? '' } })}
	confirmLabel={$t('ui.common.delete')}
	tone="destructive"
	{busy}
	onconfirm={remove}
/>
