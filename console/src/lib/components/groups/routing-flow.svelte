<script lang="ts">
	import { t } from 'svelte-i18n';

	import { formatCountdown } from '$lib/dashboard';
	import { providerLogoSrc } from '$lib/provider-logo';
	import {
		hasQuota,
		memberCoverage,
		memberState,
		windowSpan,
		type MemberState,
		type MemberView
	} from '$lib/route-inspector';

	interface Props {
		groupLabel: string;
		requestLabel: string;
		members: readonly MemberView[];
		current: number;
		currentAccount?: string;
		next: number;
		hidden: number;
		nowMs?: number;
	}

	let {
		groupLabel,
		requestLabel,
		members,
		current,
		currentAccount = '',
		next,
		hidden,
		nowMs = Date.now()
	}: Props = $props();

	const WIDTH = 1120;
	const PAD = 24;
	const TOP = 12;
	const REQUEST_X = 24;
	const NODE_WIDTH = 128;
	const GROUP_X = 184;
	const GROUP_RIGHT = GROUP_X + NODE_WIDTH;
	const TRUNK_X = 336;
	const CARD_X = 368;
	const CARD_WIDTH = WIDTH - CARD_X - PAD;
	const CARD_HEIGHT = 88;
	const ROW_HEIGHT = 100;
	const OVERFLOW_HEIGHT = 52;
	const NODE_HEIGHT = 64;
	const QUOTA_X = CARD_X + 440;

	const height = $derived(
		TOP + members.length * ROW_HEIGHT + (hidden > 0 ? OVERFLOW_HEIGHT + 16 : 8)
	);
	const centerY = $derived(
		members.length === 0 ? NODE_HEIGHT : (cardCenterY(0) + cardCenterY(members.length - 1)) / 2
	);
	const nodeY = $derived(centerY - NODE_HEIGHT / 2);
	const trunkTop = $derived(members.length === 0 ? centerY : cardCenterY(0));
	const trunkBottom = $derived(
		hidden > 0 ? cardY(members.length) + OVERFLOW_HEIGHT / 2 : cardCenterY(members.length - 1)
	);

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
		return rowY(index);
	}

	function cardCenterY(index: number): number {
		return cardY(index) + CARD_HEIGHT / 2;
	}

	function branch(index: number, accent: 'current' | 'next' | null = null): string {
		const y = cardCenterY(index);
		if (accent === null) return `M ${TRUNK_X} ${y} H ${CARD_X}`;
		return `M ${GROUP_RIGHT} ${centerY} H ${TRUNK_X} V ${y} H ${CARD_X}`;
	}

	function overflowBranch(): string {
		const y = cardY(members.length) + OVERFLOW_HEIGHT / 2;
		return `M ${TRUNK_X} ${y} H ${CARD_X}`;
	}

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

	function pillWidth(label: string): number {
		return label.length * 5.5 + 18;
	}
</script>

<div class="overflow-x-auto pb-1">
	<svg
		class="block h-auto min-w-[1120px] rounded-panel"
		width={WIDTH}
		viewBox={`0 0 ${WIDTH} ${height}`}
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
				viewBox="0 0 8 8"
				refX="7"
				refY="4"
				markerWidth="6"
				markerHeight="6"
				orient="auto-start-reverse"
			>
				<path d="M 0 0 L 8 4 L 0 8 z" fill="var(--muted)"></path>
			</marker>
			<marker
				id="route-arrow-accent"
				viewBox="0 0 8 8"
				refX="7"
				refY="4"
				markerWidth="6"
				markerHeight="6"
				orient="auto-start-reverse"
			>
				<path d="M 0 0 L 8 4 L 0 8 z" fill="var(--accent)"></path>
			</marker>
		</defs>

		<rect x="0" y="0" width={WIDTH} {height} rx="14" fill="var(--sunken)"></rect>
		<rect x="0" y="0" width={WIDTH} {height} rx="14" fill="url(#route-dots)"></rect>

		{#if members.length > 0}
			<path
				d={`M ${GROUP_RIGHT} ${centerY} H ${TRUNK_X}`}
				stroke="var(--muted)"
				stroke-width="1.5"
				fill="none"
			></path>
			<path
				d={`M ${TRUNK_X} ${trunkTop} V ${trunkBottom}`}
				stroke="var(--muted)"
				stroke-width="1.5"
				stroke-dasharray="2 6"
				stroke-linecap="round"
				fill="none"
			></path>
			{#each members as _, index (members[index].member.provider_id + '/' + members[index].member.model_id)}
				<path
					d={branch(index)}
					stroke="var(--muted)"
					stroke-width="1.5"
					stroke-dasharray="2 6"
					stroke-linecap="round"
					fill="none"
					marker-end="url(#route-arrow-idle)"
				></path>
			{/each}
			{#if hidden > 0}
				<path
					d={overflowBranch()}
					stroke="var(--muted)"
					stroke-width="1.5"
					stroke-dasharray="2 6"
					stroke-linecap="round"
					fill="none"
					marker-end="url(#route-arrow-idle)"
				></path>
			{/if}
			{#if current >= 0}
				<path
					d={branch(current, 'current')}
					stroke="var(--accent)"
					stroke-width="2"
					fill="none"
					marker-end="url(#route-arrow-accent)"
				></path>
			{/if}
			{#if next >= 0 && next !== current}
				<path
					d={branch(next, 'next')}
					stroke="var(--accent)"
					stroke-width="2"
					stroke-dasharray="3 6"
					fill="none"
					marker-end="url(#route-arrow-accent)"
				></path>
			{/if}
		{/if}

		<rect
			x={REQUEST_X}
			y={nodeY}
			width={NODE_WIDTH}
			height={NODE_HEIGHT}
			rx="14"
			fill="var(--surface)"
			stroke="var(--line)"
		></rect>
		<text x={REQUEST_X + 14} y={nodeY + 24} fill="var(--muted)" class="text-[11px]"
			>{$t('ui.pages.groupsPage.inspector.request')}</text
		>
		<text x={REQUEST_X + 14} y={nodeY + 45} fill="var(--ink)" class="text-[13px] font-semibold">
			{fit(requestLabel, 16)}
		</text>
		<path
			d={`M ${REQUEST_X + NODE_WIDTH} ${centerY} H ${GROUP_X}`}
			stroke="var(--muted)"
			stroke-width="1.5"
			fill="none"
		></path>
		<rect x={GROUP_X} y={nodeY} width={NODE_WIDTH} height={NODE_HEIGHT} rx="14" fill="var(--ink)"
		></rect>
		<text x={GROUP_X + 14} y={nodeY + 24} fill="var(--canvas)" opacity="0.72" class="text-[11px]"
			>{$t('ui.sidebar.groups')}</text
		>
		<text x={GROUP_X + 14} y={nodeY + 47} fill="var(--canvas)" class="text-[15px] font-semibold">
			{fit(groupLabel, 14)}
		</text>

		{#each members as member, index (member.member.provider_id + '/' + member.member.model_id)}
			{@const y = cardY(index)}
			{@const state = memberState(member.accounts, nowMs, member.windows)}
			{@const stateText = $t(stateKeys[state])}
			{@const tone = stateTone(state)}
			{@const coverage = memberCoverage(member.member)}
			{@const stateX = CARD_X + 210}
			{@const stateWidth = pillWidth(stateText)}
			{@const markerText =
				index === current
					? $t('ui.pages.groupsPage.inspector.current')
					: index === next
						? $t('ui.pages.groupsPage.inspector.next')
						: ''}
			{@const markerWidth = markerText === '' ? 0 : pillWidth(markerText)}
			<g>
				<rect
					x={CARD_X}
					{y}
					width={CARD_WIDTH}
					height={CARD_HEIGHT}
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
					cx={CARD_X + 28}
					cy={y + 28}
					r="12"
					fill={index === current
						? 'var(--accent)'
						: index === next
							? 'var(--accent-soft)'
							: 'var(--sunken)'}
				></circle>
				<text
					x={CARD_X + 28}
					y={y + 32}
					text-anchor="middle"
					fill={index === current ? 'var(--on-accent)' : 'var(--ink)'}
					class="text-[11px] font-semibold">{index + 1}</text
				>
				{#if providerLogoSrc(member.member.provider_id)}
					<image
						href={providerLogoSrc(member.member.provider_id)}
						x={CARD_X + 50}
						y={y + 16}
						width="24"
						height="24"
						preserveAspectRatio="xMidYMid meet"
					></image>
				{:else}
					<text
						x={CARD_X + 62}
						y={y + 33}
						text-anchor="middle"
						fill="var(--ink)"
						class="text-[12px] font-semibold">{member.providerLabel.slice(0, 1).toUpperCase()}</text
					>
				{/if}
				<text x={CARD_X + 86} y={y + 27} fill="var(--ink)" class="text-[14px] font-semibold">
					{fit(member.modelName || member.member.model_id, 33)}
				</text>
				{#if markerText !== ''}
					<rect
						x={CARD_X + 330}
						y={y + 14}
						width={markerWidth}
						height="20"
						rx="10"
						fill={index === current ? 'var(--accent)' : 'var(--accent-soft)'}
					></rect>
					<text
						x={CARD_X + 330 + markerWidth / 2}
						y={y + 28}
						text-anchor="middle"
						fill={index === current ? 'var(--on-accent)' : 'var(--accent-ink)'}
						class="text-[10px] font-semibold">{markerText}</text
					>
				{/if}
				<text x={CARD_X + 86} y={y + 47} fill="var(--muted)" class="text-[11px]">
					{fit(member.providerLabel, 18)}
				</text>
				<rect
					x={stateX}
					y={y + 34}
					width={stateWidth}
					height="18"
					rx="9"
					fill={tone.fill}
					fill-opacity="0.12"
				></rect>
				<circle cx={stateX + 9} cy={y + 43} r="2.5" fill={tone.ink}></circle>
				<text x={stateX + 16} y={y + 47} fill={tone.ink} class="text-[10px] font-semibold">
					{stateText}
				</text>
				<text
					x={stateX + stateWidth + 8}
					y={y + 47}
					fill="var(--muted)"
					class="mono-data text-[10px]"
				>
					{coverage.serving}/{coverage.active}
				</text>
				{#if index === current && currentAccount !== ''}
					<text x={CARD_X + 86} y={y + 64} fill="var(--muted)" class="mono-data text-[10px]">
						{fit(currentAccount, 34)}
					</text>
				{/if}
				{#if hasQuota(member.windows)}
					{#each member.windows.slice(0, 2) as window, windowIndex (window.credential_id + window.window)}
						{@const percent = Math.max(0, Math.round(window.used_percent))}
						{@const quotaY = y + 21 + windowIndex * 24}
						<text x={QUOTA_X} y={quotaY + 9} fill="var(--muted)" class="text-[10px]">
							{fit(quotaLabel(window), 8)}
						</text>
						<rect x={QUOTA_X + 54} y={quotaY + 2} width="104" height="6" rx="3" fill="var(--sunken)"
						></rect>
						<rect
							x={QUOTA_X + 54}
							y={quotaY + 2}
							width={(104 * Math.min(100, percent)) / 100}
							height="6"
							rx="3"
							fill={usedTone(percent)}
						></rect>
						<text
							x={QUOTA_X + 204}
							y={quotaY + 9}
							text-anchor="end"
							fill="var(--ink)"
							class="mono-data text-[10px] font-semibold">{percent}%</text
						>
						<text
							x={QUOTA_X + 272}
							y={quotaY + 9}
							text-anchor="end"
							fill="var(--muted)"
							class="mono-data text-[10px]">{resetIn(window.reset_at_ms)}</text
						>
					{/each}
				{:else}
					<text x={QUOTA_X} y={y + 46} fill="var(--muted)" class="text-[12px]">
						{$t('ui.pages.groupsPage.inspector.noQuota')}
					</text>
				{/if}
			</g>
		{/each}

		{#if hidden > 0}
			{@const y = cardY(members.length)}
			<rect
				x={CARD_X}
				{y}
				width={CARD_WIDTH}
				height={OVERFLOW_HEIGHT}
				rx="12"
				fill="var(--surface)"
				stroke="var(--line)"
			></rect>
			<text x={CARD_X + 18} y={y + 31} fill="var(--muted)" class="text-[12px]">
				{$t('ui.pages.groupsPage.inspector.overflow', { values: { count: hidden } })}
			</text>
		{/if}
	</svg>
</div>

<div class="mt-3 flex flex-wrap items-center gap-x-5 gap-y-2 text-xs text-muted-foreground">
	<span class="flex items-center gap-2"
		><span class="h-2 w-5 rounded-sm" style="background: var(--accent)"></span>{$t(
			'ui.pages.groupsPage.inspector.legendCurrent'
		)}</span
	>
	<span class="flex items-center gap-2"
		><span
			class="h-2 w-5 rounded-sm"
			style="background: transparent; box-shadow: inset 0 0 0 2px var(--accent)"
		></span>{$t('ui.pages.groupsPage.inspector.legendNext')}</span
	>
	<span class="flex items-center gap-2"
		><span class="h-2 w-5 rounded-sm" style="background: var(--warn)"></span>{$t(
			'ui.pages.groupsPage.inspector.legendLimited'
		)}</span
	>
	<span class="flex items-center gap-2"
		><span class="h-2 w-5 rounded-sm" style="background: var(--faint)"></span>{$t(
			'ui.pages.groupsPage.inspector.legendUnavailable'
		)}</span
	>
</div>
