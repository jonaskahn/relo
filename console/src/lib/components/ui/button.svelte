<script lang="ts" module>
	import { type VariantProps, tv } from 'tailwind-variants';

	export const buttonVariants = tv({
		base: "focus-visible:border-ring focus-visible:ring-ring/50 aria-invalid:border-danger aria-invalid:ring-danger/20 inline-flex shrink-0 items-center justify-center gap-2 whitespace-nowrap rounded-control text-sm font-medium outline-none transition-[background-color,color,border-color,box-shadow,transform] duration-150 active:scale-[0.985] focus-visible:ring-[3px] disabled:pointer-events-none disabled:opacity-40 disabled:active:scale-100 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-']):not([data-icon-size])]:size-4",
		variants: {
			variant: {
				default:
					'bg-primary text-primary-foreground shadow-[0_4px_14px_var(--accent-glow)] hover:bg-accent-strong',
				destructive: 'border border-line bg-surface text-danger hover:bg-hover',
				outline: 'border border-line bg-surface text-ink hover:bg-hover',
				secondary: 'bg-secondary text-secondary-foreground hover:bg-secondary/80',
				ghost: 'text-muted-foreground hover:bg-hover hover:text-ink',
				chip: 'border border-line bg-surface text-muted-foreground hover:border-line-strong hover:text-ink aria-pressed:border-accent-border aria-pressed:bg-accent-soft aria-pressed:text-accent-ink data-[state=on]:border-accent-border data-[state=on]:bg-accent-soft data-[state=on]:text-accent-ink'
			},
			size: {
				default: 'h-control px-4 has-[>svg]:px-3',
				sm: 'h-control gap-1.5 px-3 has-[>svg]:px-2.5',
				lg: 'h-control px-5 has-[>svg]:px-4',
				icon: 'size-control'
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
