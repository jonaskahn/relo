<script lang="ts">
	import { t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';
	import { api } from '$lib/api';
	import type { Group, GroupMember, Model, Provider } from '$lib/types';
	import CenteredModal from '$lib/components/ui/centered-modal.svelte';
	import ConfirmDialog from '$lib/components/ui/confirm-dialog.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import RouteStepHeader from './route-step-header.svelte';
	import StepBasics from './step-basics.svelte';
	import StepMembers from './step-members.svelte';
	import StepPreview from './step-preview.svelte';
	import {
		ROUTE_STEPS,
		eligibleMemberCount,
		emptyRouteDraft,
		memberCapabilities,
		routeDraftFrom,
		routeSaveBody,
		routeSnapshot,
		validateBasics,
		validateMembers,
		type RouteDraft,
		type RouteStep
	} from '$lib/route-stepper';

	interface Props {
		open?: boolean;
		providers: Provider[];
		configured: Provider[];
		editing?: Group | null;
		onsaved: () => void;
	}

	let { open = $bindable(false), providers, configured, editing = null, onsaved }: Props = $props();

	let draft = $state<RouteDraft>(emptyRouteDraft());
	let meta = $state<Record<string, Model>>({});
	let problems = $state<Record<string, string>>({});
	let failure = $state('');
	let saving = $state(false);
	let discardOpen = $state(false);
	let snapshot = $state('');
	let step = $state<RouteStep>('basics');
	const creating = $derived(editing === null);
	const dirty = $derived(JSON.stringify(routeSnapshot(draft)) !== snapshot);
	const eligible = $derived(eligibleMemberCount(draft.members));
	const capabilities = $derived(memberCapabilities(draft, meta));

	$effect(() => {
		if (open) reset();
	});

	function reset() {
		const next = editing === null ? emptyRouteDraft() : routeDraftFrom(editing);
		draft = next;
		meta = {};
		problems = {};
		failure = '';
		saving = false;
		step = 'basics';
		snapshot = JSON.stringify(routeSnapshot(next));
		if (editing !== null) void readMemberModels(next.members);
	}

	function patch(next: Partial<RouteDraft>) {
		draft = { ...draft, ...next };
	}

	function setMembers(members: GroupMember[]) {
		draft = { ...draft, members };
	}

	function moveMembers(from: number, to: number) {
		const members = draft.members.slice();
		const [item] = members.splice(from, 1);
		members.splice(to, 0, item);
		setMembers(members);
	}

	function learn(models: Model[]) {
		if (models.length === 0) return;
		const next = { ...meta };
		for (const model of models) next[model.provider_id + '/' + model.model_id] = model;
		meta = next;
	}

	async function readMemberModels(members: GroupMember[]) {
		const ids = [...new Set(members.map((member) => member.provider_id).filter((id) => id !== ''))];
		const results = await Promise.allSettled(
			ids.map((id) =>
				api<{ items: Model[] }>('/models?limit=500&provider=' + encodeURIComponent(id))
			)
		);
		learn(
			results.flatMap((result) => (result.status === 'fulfilled' ? (result.value.items ?? []) : []))
		);
	}

	function go(target: RouteStep) {
		step = target;
	}

	function back() {
		const index = ROUTE_STEPS.indexOf(step);
		if (index <= 0) {
			requestClose();
			return;
		}
		step = ROUTE_STEPS[index - 1];
	}

	function advance() {
		if (step === 'basics') {
			problems = validateBasics(draft);
			if (Object.keys(problems).length > 0) {
				document.getElementById('route-id')?.focus();
				return;
			}
			step = 'members';
			return;
		}
		if (step === 'members') {
			problems = validateMembers(draft, meta);
			if (Object.keys(problems).length > 0) {
				document.getElementById('route-model-search')?.focus();
				return;
			}
			step = 'review';
		}
	}

	async function save() {
		problems = { ...validateBasics(draft), ...validateMembers(draft, meta) };
		if (Object.keys(problems).length > 0) {
			step = problems.id ? 'basics' : 'members';
			return;
		}
		saving = true;
		failure = '';
		try {
			await api('/routes/' + encodeURIComponent(draft.id.trim()), {
				method: 'PUT',
				body: JSON.stringify(routeSaveBody(draft))
			});
			open = false;
			reset();
			onsaved();
			toast.success($t('ui.pages.groupsPage.saved'));
		} catch (error) {
			failure = error instanceof Error ? error.message : String(error);
		} finally {
			saving = false;
		}
	}

	function requestClose() {
		if (dirty) {
			discardOpen = true;
			open = true;
			return;
		}
		open = false;
		reset();
	}

	function discard() {
		discardOpen = false;
		open = false;
		reset();
	}
</script>

<CenteredModal
	bind:open
	size="xl"
	title={creating ? $t('ui.pages.groupsPage.create') : $t('ui.pages.groupsPage.edit')}
	description={$t('ui.pages.groupsPage.editorHint')}
	class="h-[min(780px,100%)] max-sm:h-[92dvh]"
	chrome={{
		title: 'text-lg',
		header: 'border-0 px-7 pt-5 pb-3',
		footer: 'border-0 bg-background px-7 py-3.5'
	}}
	bodyClass={step === 'members'
		? 'flex min-h-0 flex-col overflow-hidden px-0 py-0 max-sm:px-0'
		: 'px-0 py-0 max-sm:px-0'}
	onOpenChange={(value: boolean) => {
		if (!value) requestClose();
	}}
>
	<div class="flex min-h-0 flex-1 flex-col">
		<RouteStepHeader {step} onjump={go} />
		<div
			class={step === 'members'
				? 'flex min-h-0 flex-1 flex-col overflow-hidden px-4 pb-5 sm:px-7'
				: 'min-h-0 flex-1 overflow-y-auto px-4 pb-6 sm:px-7'}
		>
			{#if step === 'basics'}
				<StepBasics {draft} {creating} {problems} ondraft={patch} />
			{:else if step === 'members'}
				<StepMembers
					{draft}
					{configured}
					{providers}
					{meta}
					{problems}
					{capabilities}
					onmembers={setMembers}
					onmove={moveMembers}
					onlearn={learn}
				/>
			{:else}
				<StepPreview {draft} {providers} {meta} {capabilities} />
			{/if}
			{#if failure}
				<p class="mt-4 flex items-center gap-2 text-sm text-destructive" role="alert">
					<Icon name="alert-triangle" size={15} />{failure}
				</p>
			{/if}
		</div>
	</div>
	{#snippet footer()}
		<div class="flex items-center justify-between gap-3">
			<div class="flex min-w-0 items-center gap-2" role="status">
				<span class="min-w-0 truncate text-sm font-semibold"
					>{draft.label.trim() || draft.id.trim() || $t('ui.pages.groupsPage.create')}</span
				>
				<span
					class="hidden shrink-0 items-center rounded-full bg-accent-soft px-2.5 py-0.5 text-xs text-accent-ink sm:inline-flex"
				>
					{$t('ui.pages.groupsPage.strategyHint.' + draft.strategy)}
				</span>
				<span
					class="hidden shrink-0 items-center rounded-full px-2.5 py-0.5 text-xs sm:inline-flex {eligible ===
						0 && draft.members.length > 0
						? 'bg-warn/10 text-warn'
						: 'bg-muted text-muted-foreground'}"
				>
					{$t('ui.pages.groupsPage.memberCount', {
						values: { eligible, total: draft.members.length }
					})}
				</span>
			</div>
			<div class="flex shrink-0 items-center gap-2">
				<Button variant="ghost" onclick={back}>
					<Icon name={step === 'basics' ? 'x' : 'arrow-left'} size={14} />
					{step === 'basics' ? $t('ui.common.cancel') : $t('ui.pages.providersPage.pane.back')}
				</Button>
				{#if step === 'review'}
					<Button disabled={saving} onclick={save}>
						<Icon name={saving ? 'loader' : 'check'} size={14} spin={saving} />
						{$t('ui.pages.groupsPage.saveGroup')}
					</Button>
				{:else}
					<Button onclick={advance}>
						<Icon name="chevron-right" size={14} />
						{$t('ui.pages.groupsPage.next')}
					</Button>
				{/if}
			</div>
		</div>
	{/snippet}
</CenteredModal>

<ConfirmDialog
	bind:open={discardOpen}
	title={$t('ui.pages.groupsPage.dirtyTitle')}
	body={$t('ui.pages.groupsPage.dirtyBody')}
	confirmLabel={$t('ui.pages.groupsPage.dirtyLeave')}
	tone="destructive"
	icon="alert-triangle"
	onconfirm={discard}
/>
