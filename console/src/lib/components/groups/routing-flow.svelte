<script lang="ts">
	import { t } from 'svelte-i18n';

	import { providerLogoSrc } from '$lib/provider-logo';
	import {
		hasQuota,
		memberCoverage,
		memberState,
		resetMinutes,
		windowMinutes,
		type MemberState,
		type MemberView,
		type RouteChain
	} from '$lib/route-inspector';

	interface Props {
		groupLabel: string;
		requestLabel: string;
		members: readonly MemberView[];
		current: number;
		next: number;
		hidden: number;
		chains: readonly RouteChain[];
		nowMs?: number;
	}

	let {
		groupLabel,
		requestLabel,
		members,
		current,
		next,
		hidden,
		chains,
		nowMs = Date.now()
	}: Props = $props();

	const CARD_X = 490;
	const CARD_WIDTH = 560;
	const ROW_HEIGHT = 112;
	const TOP = 24;
	const chainTop = $derived(TOP + members.length * ROW_HEIGHT + (hidden > 0 ? ROW_HEIGHT : 0) + 22);
	const height = $derived(chainTop + Math.max(1, chains.length) * 24 + 20);

	const stateKeys: Record<MemberState, string> = {
		ready: 'ui.pages.groupsPage.inspector.ready',
		limited: 'ui.pages.groupsPage.inspector.limited',
		paused: 'ui.pages.groupsPage.inspector.paused',
		needsSignIn: 'ui.pages.groupsPage.inspector.needsSignIn',
		unavailable: 'ui.pages.groupsPage.inspector.unavailable',
		cooling: 'ui.pages.groupsPage.inspector.cooling'
	};

	function rowY(index: number): number {
		return TOP + index * ROW_HEIGHT;
	}

	function cardY(index: number): number {
		return rowY(index) + 12;
	}

	function connector(index: number): string {
		const y = cardY(index) + 38;
		return `M 414 46 H 452 V ${y} H ${CARD_X}`;
	}

	function usedTone(percent: number): string {
		if (percent >= 100) return 'var(--danger)';
		if (percent >= 90) return 'var(--warn)';
		return 'var(--accent)';
	}

	function fit(value: string, length: number): string {
		return value.length > length ? value.slice(0, length - 1) + '…' : value;
	}
</script>

<div class="overflow-x-auto pb-1">
	<svg
		class="block h-auto min-w-[1080px]"
		width="1080"
		viewBox={`0 0 1080 ${height}`}
		role="img"
		aria-labelledby="route-flow-title route-flow-desc"
	>
		<title id="route-flow-title">{$t('ui.pages.groupsPage.inspector.svgTitle')}</title>
		<desc id="route-flow-desc">{$t('ui.pages.groupsPage.inspector.svgDescription')}</desc>
		<defs>
			<marker
				id="route-arrow"
				viewBox="0 0 8 8"
				refX="7"
				refY="4"
				markerWidth="6"
				markerHeight="6"
				orient="auto-start-reverse"
			>
				<path d="M 0 0 L 8 4 L 0 8 z" fill="var(--muted)"></path>
			</marker>
		</defs>

		<rect x="24" y="24" width="180" height="44" rx="12" class="fill-surface stroke-line"></rect>
		<text x="38" y="43" class="fill-muted text-[11px]"
			>{$t('ui.pages.groupsPage.inspector.request')}</text
		>
		<text x="38" y="60" class="fill-ink text-[13px] font-semibold">{fit(requestLabel, 24)}</text>
		<path d="M 204 46 H 250" class="stroke-muted" stroke-width="1.5" marker-end="url(#route-arrow)"
		></path>
		<rect x="254" y="24" width="160" height="44" rx="12" class="fill-surface stroke-line"></rect>
		<text x="268" y="43" class="fill-muted text-[11px]">{$t('ui.sidebar.groups')}</text>
		<text x="268" y="60" class="fill-ink text-[13px] font-semibold">{fit(groupLabel, 20)}</text>

		{#each members as member, index (member.member.provider_id + '/' + member.member.model_id)}
			<path
				d={connector(index)}
				class="stroke-muted"
				fill="none"
				stroke-width="1.5"
				marker-end="url(#route-arrow)"
			></path>
		{/each}
		{#if hidden > 0}
			<path
				d={connector(members.length)}
				class="stroke-muted"
				fill="none"
				stroke-width="1.5"
				marker-end="url(#route-arrow)"
			></path>
		{/if}

		{#each members as member, index (member.member.provider_id + '/' + member.member.model_id)}
			{@const y = cardY(index)}
			{@const state = memberState(member.accounts, nowMs, member.windows)}
			{@const coverage = memberCoverage(member.member)}
			<g>
				<rect
					x={CARD_X}
					{y}
					width={CARD_WIDTH}
					height="88"
					rx="12"
					class="fill-surface"
					stroke={index === current ? 'var(--accent)' : 'var(--line)'}
					stroke-width={index === current ? 2 : 1}
				></rect>
				<text x={CARD_X + 18} y={y + 22} class="fill-muted text-[11px] font-semibold"
					>#{index + 1}</text
				>
				{#if providerLogoSrc(member.member.provider_id)}
					<image
						href={providerLogoSrc(member.member.provider_id)}
						x={CARD_X + 44}
						y={y + 10}
						width="24"
						height="24"
						preserveAspectRatio="xMidYMid meet"
					></image>
				{:else}
					<text x={CARD_X + 48} y={y + 28} class="fill-ink text-[13px] font-semibold">
						{member.providerLabel.slice(0, 1).toUpperCase()}
					</text>
				{/if}
				<text x={CARD_X + 78} y={y + 21} class="fill-ink text-[13px] font-semibold">
					{fit(member.modelName || member.member.model_id, 38)}
				</text>
				<text x={CARD_X + 78} y={y + 39} class="fill-muted text-[11px]">
					{member.providerLabel} · {$t(stateKeys[state])} · {coverage.serving}/{coverage.active}
				</text>
				{#if index === current}
					<text
						x={CARD_X + CARD_WIDTH - 20}
						y={y + 22}
						text-anchor="end"
						class="fill-accent text-[11px] font-semibold"
					>
						{$t('ui.pages.groupsPage.inspector.current')}
					</text>
				{:else if index === next}
					<text
						x={CARD_X + CARD_WIDTH - 20}
						y={y + 22}
						text-anchor="end"
						class="fill-accent text-[11px] font-semibold"
					>
						{$t('ui.pages.groupsPage.inspector.next')}
					</text>
				{/if}
				{#if hasQuota(member.windows)}
					{#each member.windows.slice(0, 2) as window, windowIndex (window.credential_id + window.window)}
						{@const percent = Math.max(0, Math.round(window.used_percent))}
						{@const barY = y + 48 + windowIndex * 17}
						<text x={CARD_X + 18} y={barY + 8} class="fill-muted text-[10px]">
							{window.window} · {percent}% · {windowMinutes(window)}m · {resetMinutes(
								window.reset_at_ms,
								nowMs
							)}m
						</text>
						<rect x={CARD_X + 350} y={barY} width="170" height="7" rx="3.5" class="fill-sunken"
						></rect>
						<rect
							x={CARD_X + 350}
							y={barY}
							width={(170 * Math.min(100, percent)) / 100}
							height="7"
							rx="3.5"
							fill={usedTone(percent)}
						></rect>
					{/each}
				{:else}
					<text x={CARD_X + 18} y={y + 60} class="fill-muted text-[11px]">
						{$t('ui.pages.groupsPage.inspector.noQuota')}
					</text>
				{/if}
			</g>
		{/each}

		{#if hidden > 0}
			{@const y = cardY(members.length)}
			<rect x={CARD_X} {y} width={CARD_WIDTH} height="52" rx="12" class="fill-surface stroke-line"
			></rect>
			<text x={CARD_X + 18} y={y + 32} class="fill-muted text-[12px]">
				{$t('ui.pages.groupsPage.inspector.overflow', { values: { count: hidden } })}
			</text>
		{/if}

		{#if chains.length > 0}
			<text x="24" y={chainTop} class="fill-muted text-[11px] font-semibold">
				{$t('ui.pages.groupsPage.inspector.recent')}
			</text>
			{#each chains as chain, index (chain.eventID)}
				<text x="24" y={chainTop + 22 + index * 22} class="fill-ink text-[11px]">
					{chain.chain}
				</text>
			{/each}
		{/if}
	</svg>
</div>

<div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-2 text-xs text-muted-foreground">
	<span class="flex items-center gap-1.5"
		><span class="h-2 w-4 rounded-sm bg-accent"></span>{$t(
			'ui.pages.groupsPage.inspector.legendCurrent'
		)}</span
	>
	<span class="flex items-center gap-1.5"
		><span class="h-2 w-4 rounded-sm bg-accent-soft ring-1 ring-accent"></span>{$t(
			'ui.pages.groupsPage.inspector.legendNext'
		)}</span
	>
	<span class="flex items-center gap-1.5"
		><span class="h-2 w-4 rounded-sm bg-warn"></span>{$t(
			'ui.pages.groupsPage.inspector.legendLimited'
		)}</span
	>
	<span class="flex items-center gap-1.5"
		><span class="h-2 w-4 rounded-sm bg-faint"></span>{$t(
			'ui.pages.groupsPage.inspector.legendUnavailable'
		)}</span
	>
</div>
