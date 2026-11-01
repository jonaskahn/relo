<script lang="ts">
	import { t } from 'svelte-i18n';

	import { formatCountdown } from '$lib/dashboard';
	import {
		hasQuota,
		memberCoverage,
		memberState,
		routingLayout,
		statePillWidth,
		windowSpan,
		type MemberState,
		type MemberView
	} from '$lib/route-inspector';
	import TunnelMark from '$lib/components/ui/tunnel-mark.svelte';

	interface Props {
		groupLabel: string;
		members: readonly MemberView[];
		current: number;
		currentAccount?: string;
		next: number;
		hidden: number;
		nowMs?: number;
	}

	let {
		groupLabel,
		members,
		current,
		currentAccount = '',
		next,
		hidden,
		nowMs = Date.now()
	}: Props = $props();

	const DEFAULT_WIDTH = 1120;
	let host: HTMLDivElement | null = $state(null);
	let measuredWidth = $state(0);
	const layout = $derived(
		routingLayout({ width: measuredWidth || DEFAULT_WIDTH, members: members.length, hidden })
	);

	$effect(() => {
		if (host === null || typeof ResizeObserver === 'undefined') return;
		const measure = () => {
			const nextWidth = Math.floor(host?.clientWidth ?? 0);
			if (nextWidth > 0) measuredWidth = nextWidth;
		};
		measure();
		const observer = new ResizeObserver(measure);
		observer.observe(host);
		return () => observer.disconnect();
	});

	const stateKeys: Record<MemberState, string> = {
		ready: 'ui.pages.groupsPage.inspector.ready',
		limited: 'ui.pages.groupsPage.inspector.limited',
		paused: 'ui.pages.groupsPage.inspector.paused',
		needsSignIn: 'ui.pages.groupsPage.inspector.needsSignIn',
		unavailable: 'ui.pages.groupsPage.inspector.unavailable',
		cooling: 'ui.pages.groupsPage.inspector.cooling'
	};

	const currentPath = $derived(
		current >= 0 && current < layout.branches.length ? (layout.branches[current]?.path ?? '') : ''
	);
	const currentMotionPath = $derived(
		current >= 0 && current < layout.branches.length
			? (layout.branches[current]?.motionPath ?? '')
			: ''
	);
	const nextPath = $derived(
		next >= 0 && next < layout.branches.length ? (layout.branches[next]?.path ?? '') : ''
	);

	function usedTone(percent: number): string {
		if (percent >= 100) return 'var(--danger)';
		if (percent >= 90) return 'var(--warn)';
		return 'var(--accent)';
	}

	function resetIn(resetAtMs: number): string {
		return resetAtMs > 0 ? formatCountdown(resetAtMs - nowMs) : '';
	}

	function quotaLabel(window: MemberView['windows'][number]): string {
		const label = window.window.trim();
		const span = windowSpan(window.window_seconds);
		if (/^\d+(\.\d+)?[smhd]$/i.test(label)) return span || label;
		return label || span;
	}

	function fit(value: string, length: number): string {
		return value.length > length ? value.slice(0, length - 1) + '…' : value;
	}

	function charactersIn(width: number): number {
		return Math.max(6, Math.floor(width / 7));
	}

	function stateTone(state: MemberState): { ink: string; fill: string } {
		switch (state) {
			case 'ready':
				return { ink: 'var(--ok)', fill: 'var(--ok)' };
			case 'limited':
			case 'needsSignIn':
				return { ink: 'var(--warn)', fill: 'var(--warn)' };
			case 'cooling':
				return { ink: 'var(--accent-ink)', fill: 'var(--accent)' };
			default:
				return { ink: 'var(--muted)', fill: 'var(--muted)' };
		}
	}

	function markerFor(index: number): string {
		if (index === current) return $t('ui.pages.groupsPage.inspector.current');
		if (index === next) return $t('ui.pages.groupsPage.inspector.next');
		return '';
	}

	function quotaIsRight(cardWidth: number): boolean {
		return layout.kind === 'wide' || cardWidth >= 620;
	}
</script>

<div bind:this={host} class="min-w-0 max-w-full pb-1">
	<svg
		class="block h-auto w-full max-w-full rounded-panel"
		width={layout.width}
		viewBox={`0 0 ${layout.width} ${layout.height}`}
		role="img"
		aria-labelledby="route-flow-title route-flow-desc"
	>
		<title id="route-flow-title">{$t('ui.pages.groupsPage.inspector.svgTitle')}</title>
		<desc id="route-flow-desc">{$t('ui.pages.groupsPage.inspector.svgDescription')}</desc>
		<defs>
			<pattern id="route-dots" width="20" height="20" patternUnits="userSpaceOnUse">
				<circle cx="1" cy="1" r="1" fill="var(--line-strong)"></circle>
			</pattern>
			<marker
				id="route-arrow-idle"
				viewBox="0 0 5 5"
				refX="5"
				refY="2.5"
				markerWidth="5"
				markerHeight="5"
				markerUnits="userSpaceOnUse"
				orient="auto-start-reverse"
			>
				<path d="M 0 0 L 5 2.5 L 0 5 z" fill="var(--muted)"></path>
			</marker>
			<marker
				id="route-arrow-accent"
				viewBox="0 0 5 5"
				refX="5"
				refY="2.5"
				markerWidth="5"
				markerHeight="5"
				markerUnits="userSpaceOnUse"
				orient="auto-start-reverse"
			>
				<path d="M 0 0 L 5 2.5 L 0 5 z" fill="var(--accent)"></path>
			</marker>
		</defs>

		<rect x="0" y="0" width={layout.width} height={layout.height} rx="14" fill="var(--sunken)"
		></rect>
		<rect x="0" y="0" width={layout.width} height={layout.height} rx="14" fill="url(#route-dots)"
		></rect>

		{#if members.length > 0}
			<g aria-hidden="true">
				{#each layout.branches as branch (branch.index)}
					<path
						d={branch.path}
						stroke="var(--line-strong)"
						stroke-width="1.5"
						stroke-dasharray="2 6"
						stroke-linecap="round"
						fill="none"
						marker-end="url(#route-arrow-idle)"
					></path>
				{/each}
				{#if layout.overflowBranch}
					<path
						d={layout.overflowBranch}
						stroke="var(--line-strong)"
						stroke-width="1.5"
						stroke-dasharray="2 6"
						stroke-linecap="round"
						fill="none"
						marker-end="url(#route-arrow-idle)"
					></path>
				{/if}
				{#if currentPath}
					<path
						data-route-current
						d={currentPath}
						stroke="var(--accent)"
						stroke-width="2"
						fill="none"
						marker-end="url(#route-arrow-accent)"
					></path>
				{/if}
				{#if nextPath && next !== current}
					<path
						d={nextPath}
						stroke="var(--accent)"
						stroke-width="2"
						stroke-dasharray="3 6"
						fill="none"
						marker-end="url(#route-arrow-accent)"
					></path>
				{/if}
				{#if currentPath}
					{#key current}
						<circle class="route-moving-dot" r="4" fill="var(--accent)">
							<animateMotion path={currentMotionPath} dur="2.4s" repeatCount="indefinite" />
						</circle>
					{/key}
				{/if}
			</g>
		{/if}

		<rect
			x={layout.request.x}
			y={layout.request.y}
			width={layout.request.width}
			height={layout.request.height}
			rx="14"
			fill="var(--surface)"
			stroke="var(--line)"
		></rect>
		<text
			x={layout.request.x + layout.request.width / 2}
			y={layout.request.y + layout.request.height / 2 + 4}
			text-anchor="middle"
			fill="var(--ink)"
			class="text-[13px] font-semibold"
		>
			{$t('ui.sidebar.agents')}
		</text>
		<path
			d={layout.requestConnector}
			stroke="var(--muted)"
			stroke-width="1.5"
			fill="none"
			marker-end="url(#route-arrow-idle)"
		></path>
		<rect
			x={layout.group.x}
			y={layout.group.y}
			width={layout.group.width}
			height={layout.group.height}
			rx="14"
			fill="var(--ink)"
		></rect>
		<text
			x={layout.group.x + 14}
			y={layout.group.y + 24}
			fill="var(--canvas)"
			opacity="0.72"
			class="text-[11px]">{$t('ui.sidebar.groups')}</text
		>
		<text
			x={layout.group.x + 14}
			y={layout.group.y + 47}
			fill="var(--canvas)"
			class="text-[15px] font-semibold"
		>
			<title>{groupLabel}</title>
			{fit(groupLabel, charactersIn(layout.group.width - 28))}
		</text>

		{#each members as member, index (member.member.provider_id + '/' + member.member.model_id)}
			{@const card = layout.cards[index]}
			{@const state = memberState(member.accounts, nowMs, member.windows)}
			{@const stateText = $t(stateKeys[state])}
			{@const stateWidth = statePillWidth(stateText)}
			{@const tone = stateTone(state)}
			{@const coverage = memberCoverage(member.member)}
			{@const markerText = markerFor(index)}
			{@const markerWidth = markerText === '' ? 0 : statePillWidth(markerText)}
			{@const markerX = card.x + card.width - 14 - markerWidth}
			{@const chipX = card.x + 76}
			{@const chipY = card.y + (layout.kind === 'wide' ? 50 : 58)}
			{@const rightQuota = quotaIsRight(card.width)}
			{@const quotaX = rightQuota ? card.x + card.width - 286 : card.x + 14}
			{@const modelTitle = member.modelName || member.member.model_id}
			{@const modelLimit = charactersIn(markerX - (card.x + 76) - 8)}
			<g>
				<rect
					x={card.x}
					y={card.y}
					width={card.width}
					height={card.height}
					rx="14"
					fill="var(--surface)"
					stroke={index === current
						? 'var(--accent)'
						: index === next
							? 'var(--accent-border)'
							: 'var(--line)'}
					stroke-width={index === current ? 2 : 1}
				></rect>
				<circle
					cx={card.x + 28}
					cy={card.y + 28}
					r="12"
					fill={index === current
						? 'var(--accent)'
						: index === next
							? 'var(--accent-soft)'
							: 'var(--sunken)'}
				></circle>
				<text
					x={card.x + 28}
					y={card.y + 32}
					text-anchor="middle"
					fill={index === current ? 'var(--on-accent)' : 'var(--ink)'}
					class="text-[11px] font-semibold">{index + 1}</text
				>
				<TunnelMark
					markId={member.member.provider_id}
					label={member.providerLabel}
					x={card.x + 54}
					y={card.y + 28}
					size={24}
				/>
				<text x={card.x + 76} y={card.y + 27} fill="var(--ink)" class="text-[14px] font-semibold">
					<title>{modelTitle}</title>
					{fit(modelTitle, modelLimit)}
				</text>
				{#if markerText !== ''}
					<rect
						x={markerX}
						y={card.y + 14}
						width={markerWidth}
						height="20"
						rx="10"
						fill={index === current ? 'var(--accent)' : 'var(--accent-soft)'}
					></rect>
					<text
						x={markerX + markerWidth / 2}
						y={card.y + 28}
						text-anchor="middle"
						fill={index === current ? 'var(--on-accent)' : 'var(--accent-ink)'}
						class="text-[10px] font-semibold">{markerText}</text
					>
				{/if}
				<text x={card.x + 76} y={card.y + 45} fill="var(--muted)" class="text-[11px]">
					<title>{member.providerLabel}</title>
					{fit(member.providerLabel, charactersIn(card.width - 90))}
				</text>
				<g>
					<rect
						x={chipX}
						y={chipY}
						width={stateWidth}
						height="18"
						rx="9"
						fill={tone.fill}
						fill-opacity="0.12"
					></rect>
					<circle cx={chipX + 8} cy={chipY + 9} r="2.5" fill={tone.ink}></circle>
					<text x={chipX + 16} y={chipY + 13} fill={tone.ink} class="text-[10px] font-semibold">
						{stateText}
					</text>
				</g>
				<text
					x={chipX + stateWidth + 8}
					y={chipY + 13}
					fill="var(--muted)"
					class="mono-data text-[10px]"
				>
					{coverage.serving}/{coverage.active}
				</text>
				{#if index === current && currentAccount !== ''}
					<text x={chipX} y={chipY + 30} fill="var(--muted)" class="mono-data text-[10px]">
						<title>{currentAccount}</title>
						{fit(currentAccount, 34)}
					</text>
				{/if}
				{#if hasQuota(member.windows)}
					{#each member.windows.slice(0, 2) as window, windowIndex (window.credential_id + window.window)}
						{@const percent = Math.max(0, Math.round(window.used_percent))}
						{@const quotaY = rightQuota
							? card.y + 18 + windowIndex * 24
							: card.y + 106 + windowIndex * 26}
						{@const barX = rightQuota ? quotaX + 54 : quotaX + 46}
						{@const barWidth = rightQuota ? 104 : Math.max(40, card.width - 230)}
						{@const percentX = rightQuota ? quotaX + 204 : quotaX + card.width - 100}
						{@const resetX = rightQuota ? quotaX + 272 : quotaX + card.width - 14}
						<text x={quotaX} y={quotaY + 9} fill="var(--muted)" class="text-[10px]">
							{fit(quotaLabel(window), 8)}
						</text>
						<rect x={barX} y={quotaY + 2} width={barWidth} height="6" rx="3" fill="var(--sunken)"
						></rect>
						<rect
							x={barX}
							y={quotaY + 2}
							width={(barWidth * Math.min(100, percent)) / 100}
							height="6"
							rx="3"
							fill={usedTone(percent)}
						></rect>
						<text
							x={percentX}
							y={quotaY + 9}
							text-anchor="end"
							fill="var(--ink)"
							class="mono-data text-[10px] font-semibold">{percent}%</text
						>
						<text
							x={resetX}
							y={quotaY + 9}
							text-anchor="end"
							fill="var(--muted)"
							class="mono-data text-[10px]">{resetIn(window.reset_at_ms)}</text
						>
					{/each}
				{:else}
					<text
						x={rightQuota ? quotaX : card.x + 76}
						y={rightQuota ? card.y + 46 : card.y + 116}
						fill="var(--muted)"
						class="text-[12px]"
					>
						{$t('ui.pages.groupsPage.inspector.noQuota')}
					</text>
				{/if}
			</g>
		{/each}

		{#if layout.overflow}
			<rect
				x={layout.overflow.x}
				y={layout.overflow.y}
				width={layout.overflow.width}
				height={layout.overflow.height}
				rx="12"
				fill="var(--surface)"
				stroke="var(--line)"
			></rect>
			<text
				x={layout.overflow.x + 18}
				y={layout.overflow.y + 31}
				fill="var(--muted)"
				class="text-[12px]"
			>
				{$t('ui.pages.groupsPage.inspector.overflow', { values: { count: hidden } })}
			</text>
		{/if}
	</svg>
</div>

<div class="mt-3 flex flex-wrap items-center gap-x-5 gap-y-2 text-xs text-muted-foreground">
	<span class="flex items-center gap-2"
		><span class="h-2 w-5 rounded-sm" style="background: var(--accent)"></span>{$t(
			'ui.pages.groupsPage.inspector.legendCurrent'
		)}
	</span>
	<span class="flex items-center gap-2"
		><span
			class="h-2 w-5 rounded-sm"
			style="background: transparent; box-shadow: inset 0 0 0 2px var(--accent)"
		></span>{$t('ui.pages.groupsPage.inspector.legendNext')}</span
	>
	<span class="flex items-center gap-2"
		><span class="h-2 w-5 rounded-sm" style="background: var(--warn)"></span>{$t(
			'ui.pages.groupsPage.inspector.legendLimited'
		)}
	</span>
	<span class="flex items-center gap-2"
		><span class="h-2 w-5 rounded-sm" style="background: var(--faint)"></span>{$t(
			'ui.pages.groupsPage.inspector.legendUnavailable'
		)}
	</span>
</div>

<style>
	@media (prefers-reduced-motion: reduce) {
		.route-moving-dot {
			display: none;
		}
	}
</style>
