<script lang="ts" module>
	import { type VariantProps, tv } from 'tailwind-variants';

	// A button is the thing an operator presses, so it carries no border: it
	// floats at float-2, which is what sets it apart from the field it rests
	// on. Fields, tabs and chips sit a level lower, at float-1.
	export const buttonVariants = tv({
		base: "focus-visible:ring-ring/50 aria-invalid:ring-danger/20 inline-flex shrink-0 items-center justify-center gap-2 whitespace-nowrap rounded-control text-sm font-medium outline-none transition-[background-color,color,box-shadow,transform] duration-150 active:scale-[0.985] active:shadow-float-2-press focus-visible:ring-[3px] disabled:active:scale-100 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-']):not([data-icon-size])]:size-4",
		variants: {
			variant: {
				default:
					'shadow-float-2 bg-primary text-primary-foreground hover:bg-accent-strong hover:shadow-float-2-hover',
				destructive:
					'shadow-float-2 bg-surface text-danger hover:bg-hover hover:shadow-float-2-hover',
				outline:
					'shadow-float-2 bg-surface text-ink hover:bg-control-hover hover:shadow-float-2-hover',
				secondary:
					'shadow-float-2 bg-surface text-ink hover:bg-control-hover hover:shadow-float-2-hover',
				// Ghost is the low-emphasis variant on purpose: flat, so it never
				// competes with the floating controls beside it.
				ghost: 'text-muted-foreground hover:bg-hover hover:text-ink',
				chip: 'shadow-float-1 bg-control text-muted-foreground hover:bg-control-hover hover:text-ink aria-pressed:bg-accent-soft aria-pressed:text-accent-ink aria-pressed:shadow-none data-[state=on]:bg-accent-soft data-[state=on]:text-accent-ink data-[state=on]:shadow-none'
			},
			size: {
				default: 'h-control px-4 has-[>svg]:px-3',
				sm: 'h-control gap-1.5 px-3 has-[>svg]:px-2.5',
				lg: 'h-control px-5 has-[>svg]:px-4',
				// An icon-only button is a square: a 10px corner on a
				// 38px square reads as unfinished next to a round one.
				icon: 'size-control rounded-control'
			}
		},
		defaultVariants: {
			variant: 'default',
			size: 'default'
		}
	});

	export type ButtonVariant = VariantProps<typeof buttonVariants>['variant'];
	export type ButtonSize = VariantProps<typeof buttonVariants>['size'];
</script>

<script lang="ts">
	import type { HTMLButtonAttributes } from 'svelte/elements';

	import { cn } from '$lib/utils';

	// A button does an action and an anchor navigates, so this control never
	// renders one: a caller that needs a link writes the anchor itself.
	interface Props extends HTMLButtonAttributes {
		variant?: ButtonVariant;
		size?: ButtonSize;
		// ref is the rendered element, so a caller that has to hand focus back
		// to this control — the button that opened a drawer — can hold it the
		// same way a Bits UI trigger does.
		ref?: HTMLElement | null;
		class?: string;
	}

	let {
		variant = 'default',
		size = 'default',
		ref = $bindable(null),
		class: className,
		children,
		...rest
	}: Props = $props();

	const classes = $derived(cn(buttonVariants({ variant, size }), className));
</script>

<button bind:this={ref} data-slot="button" class={classes} {...rest}>
	{@render children?.()}
</button>
