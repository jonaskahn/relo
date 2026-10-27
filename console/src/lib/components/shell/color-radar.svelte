<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from 'svelte-i18n';

	import { hexToHsv, hueColour, hsvToHex, parseHex, type Hsv } from '$lib/color';
	import type { HexColour } from '$lib/color';
	import { cn } from '$lib/utils';

	interface Props {
		/** The colour the slot holds, which is also what a change saves. */
		value: HexColour;
		/** Runs when a pick is finished: on release, on an arrow key, and on a typed colour. */
		onchange: (colour: HexColour) => void;
	}

	let { value, onchange }: Props = $props();

	// The radar opens on the colour it is given and then owns its own cursor: the
	// effect below only follows a colour changed somewhere else.
	let cursor = $state<Hsv>(untrack(() => hexToHsv(value)));
	let field = $state<string>(untrack(() => value));
	let invalid = $state(false);
	let dragging = $state(false);

	const colour = $derived(hsvToHex(cursor));

	// A colour changed anywhere else lands in the radar, which is what keeps the
	// cursor on the accent the console is actually wearing.
	let held = untrack(() => value);
	$effect(() => {
		if (value !== held) {
			held = value;
			cursor = hexToHsv(value);
			field = value;
			invalid = false;
		}
	});

	function clamp(part: number): number {
		return Math.min(1, Math.max(0, part));
	}

	/** Moves the cursor and reports the colour: keyboard picks save at once,
	 *  while a drag saves when it ends, so one drag is one write. */
	function move(pick: Partial<Hsv>, save: boolean): void {
		cursor = {
			hue: (((pick.hue ?? cursor.hue) % 360) + 360) % 360,
			saturation: clamp(pick.saturation ?? cursor.saturation),
			value: clamp(pick.value ?? cursor.value)
		};
		const hex = hsvToHex(cursor);
		field = hex;
		invalid = false;
		if (save) onchange(hex);
	}

	function pointerBox(event: PointerEvent): DOMRect {
		return (event.currentTarget as HTMLElement).getBoundingClientRect();
	}

	function pickSquare(event: PointerEvent): void {
		const box = pointerBox(event);
		const across = (event.clientX - box.left) / box.width;
		const down = (event.clientY - box.top) / box.height;
		move({ saturation: across, value: 1 - down }, false);
	}

	function pickHue(event: PointerEvent): void {
		const box = pointerBox(event);
		const across = (event.clientX - box.left) / box.width;
		move({ hue: Math.round(across * 359) }, false);
	}

	function start(event: PointerEvent): void {
		dragging = true;
		(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
		pickSquare(event);
	}

	function startHue(event: PointerEvent): void {
		dragging = true;
		(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
		pickHue(event);
	}

	function dragSquare(event: PointerEvent): void {
		if (!dragging) return;
		pickSquare(event);
	}

	function dragHue(event: PointerEvent): void {
		if (!dragging) return;
		pickHue(event);
	}

	function stop(): void {
		if (!dragging) return;
		dragging = false;
		onchange(colour);
	}

	/** The square steps saturation across and value up; the rail steps the hue. */
	function keyStep(event: KeyboardEvent, rail: boolean): void {
		const step = event.shiftKey ? 1 : 12;
		if (rail) {
			if (!event.key.startsWith('Arrow')) return;
			event.preventDefault();
			const direction = event.key === 'ArrowRight' || event.key === 'ArrowUp' ? 1 : -1;
			move({ hue: cursor.hue + direction * step }, true);
			return;
		}
		const fine = event.shiftKey ? 0.01 : 0.05;
		const part =
			event.key === 'ArrowRight'
				? { saturation: cursor.saturation + fine }
				: event.key === 'ArrowLeft'
					? { saturation: cursor.saturation - fine }
					: event.key === 'ArrowUp'
						? { value: cursor.value + fine }
						: event.key === 'ArrowDown'
							? { value: cursor.value - fine }
							: null;
		if (!part) return;
		event.preventDefault();
		move(part, true);
	}

	/** A typed colour applies on its own terms: Enter or a field change saves it,
	 *  and anything that is not #rrggbb stays in the field with the rule beside it. */
	function commitField(): void {
		const parsed = parseHex(field);
		if (!parsed) {
			invalid = true;
			return;
		}
		invalid = false;
		field = parsed;
		cursor = hexToHsv(parsed);
		onchange(parsed);
	}
</script>

<div id="accent-radar" class="mt-3 flex flex-col gap-3 rounded-panel border p-3">
	<div
		class="radar-square relative h-44 w-full cursor-crosshair rounded-control border focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
		style="--radar-hue: {hueColour(cursor.hue)}"
		role="slider"
		tabindex="0"
		aria-label={$t('ui.accent.radarSquare')}
		aria-valuemin={0}
		aria-valuemax={100}
		aria-valuenow={Math.round(cursor.saturation * 100)}
		aria-valuetext={colour}
		onpointerdown={start}
		onpointermove={dragSquare}
		onpointerup={stop}
		onpointercancel={stop}
		onkeydown={(event) => keyStep(event, false)}
	>
		<span
			class="pointer-events-none absolute size-4 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-surface"
			style="left: {cursor.saturation * 100}%; top: {(1 - cursor.value) *
				100}%; background: {colour}; box-shadow: 0 0 0 2px var(--surface);"
		></span>
	</div>

	<div
		class="radar-rail relative h-3.5 w-full cursor-pointer rounded-full border focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
		role="slider"
		tabindex="0"
		aria-label={$t('ui.accent.radarHue')}
		aria-valuemin={0}
		aria-valuemax={359}
		aria-valuenow={Math.round(cursor.hue)}
		onpointerdown={startHue}
		onpointermove={dragHue}
		onpointerup={stop}
		onpointercancel={stop}
		onkeydown={(event) => keyStep(event, true)}
	>
		<span
			class="pointer-events-none absolute top-1/2 size-4 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-surface"
			style="left: {(cursor.hue / 359) * 100}%; background: {hueColour(
				cursor.hue
			)}; box-shadow: 0 0 0 2px var(--surface);"
		></span>
	</div>

	<div class="flex items-center gap-2.5">
		<span
			class="size-7 shrink-0 rounded-full border"
			style="background: {colour};"
			aria-hidden="true"
		></span>
		<label class="flex items-center gap-2 text-xs text-muted-foreground">
			{$t('ui.accent.radarHex')}
			<input
				class={cn(
					'h-control-sm mono-data w-28 rounded-control border bg-control px-2.5 text-xs text-ink',
					invalid ? 'border-danger' : 'border-line'
				)}
				value={field}
				spellcheck="false"
				autocomplete="off"
				aria-invalid={invalid}
				oninput={(event) => {
					field = (event.currentTarget as HTMLInputElement).value;
					invalid = false;
				}}
				onchange={commitField}
				onkeydown={(event) => {
					if (event.key !== 'Enter') return;
					event.preventDefault();
					commitField();
				}}
			/>
		</label>
	</div>

	{#if invalid}
		<p class="text-xs text-destructive" role="alert">{$t('ui.accent.radarInvalid')}</p>
	{/if}
</div>
