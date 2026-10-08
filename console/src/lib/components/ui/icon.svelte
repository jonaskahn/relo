<script module lang="ts">
	// One outline icon set, vendored from Tabler Icons (MIT) so the console
	// ships no new dependency and no icon is drawn by hand. Every path is a
	// 24x24 outline at stroke 2, which is the one weight the console uses.
	export type IconName =
		| 'alert-triangle'
		| 'arrow-down'
		| 'arrow-left'
		| 'arrow-right'
		| 'arrow-back-up'
		| 'arrow-up'
		| 'activity'
		| 'ban'
		| 'bolt'
		| 'brand-github'
		| 'chart-bar'
		| 'check'
		| 'chevron-down'
		| 'chevron-right'
		| 'circle-alert'
		| 'circle-check'
		| 'circle-off'
		| 'circle-x'
		| 'cloud-download'
		| 'cloud'
		| 'coffee'
		| 'copy-plus'
		| 'copy'
		| 'cpu'
		| 'database'
		| 'device-desktop'
		| 'dots'
		| 'edit'
		| 'external-link'
		| 'eye'
		| 'eye-off'
		| 'filter'
		| 'gauge'
		| 'info-circle'
		| 'key'
		| 'language'
		| 'layout-dashboard'
		| 'layers'
		| 'layout-sidebar'
		| 'layout-sidebar-right'
		| 'list-details'
		| 'list'
		| 'loader'
		| 'message-square'
		| 'lock'
		| 'logout'
		| 'menu-2'
		| 'minus'
		| 'moon'
		| 'palette'
		| 'pencil'
		| 'player-pause'
		| 'player-play'
		| 'player-stop'
		| 'plug-connected'
		| 'plus'
		| 'power'
		| 'refresh'
		| 'route'
		| 'ruler-2'
		| 'search'
		| 'selector'
		| 'server'
		| 'settings'
		| 'sparkles'
		| 'sun'
		| 'tag'
		| 'toggle-left'
		| 'trash'
		| 'user'
		| 'currency-dollar'
		| 'clock'
		| 'trending-up'
		| 'grip-vertical'
		| 'wrench'
		| 'x'
		| 'world';

	// paths holds one outline per icon, which is the whole icon set.
	export const paths: Record<IconName, string> = {
		activity: 'M3 12h4l3 8l4 -16l3 8h4',
		'alert-triangle':
			'M12 9v4M10.363 3.591l-8.106 13.534a1.914 1.914 0 0 0 1.636 2.871h16.214a1.914 1.914 0 0 0 1.636 -2.87l-8.106 -13.536a1.914 1.914 0 0 0 -3.274 0zM12 16h.01',
		'arrow-left': 'M5 12l14 0M5 12l6 6M5 12l6 -6',
		'arrow-right': 'M5 12h14M13 6l6 6l-6 6',
		'arrow-back-up': 'M9 14l-4 -4l4 -4M5 10h11a4 4 0 1 1 0 8h-1',
		ban: 'M12 12m-9 0a9 9 0 1 0 18 0a9 9 0 1 0 -18 0M5.7 5.7l12.6 12.6',
		'brand-github':
			'M9 19c-4.3 1.4 -4.3 -2.5 -6 -3m12 5v-3.5c0 -1 .1 -1.4 -.5 -2c2.8 -.3 5.5 -1.4 5.5 -6a4.6 4.6 0 0 0 -1.3 -3.2a4.2 4.2 0 0 0 -.1 -3.2s-1.1 -.3 -3.5 1.3a12.3 12.3 0 0 0 -6.2 0c-2.4 -1.6 -3.5 -1.3 -3.5 -1.3a4.2 4.2 0 0 0 -.1 3.2a4.6 4.6 0 0 0 -1.3 3.2c0 4.6 2.7 5.7 5.5 6c-.6 .6 -.6 1.2 -.5 2v3.5',
		check: 'M5 12l5 5l10 -10',
		'chevron-down': 'M6 9l6 6l6 -6',
		'chevron-right': 'M9 6l6 6l-6 6',
		'circle-alert': 'M12 12m-9 0a9 9 0 1 0 18 0a9 9 0 1 0 -18 0M12 8v4M12 16h.01',
		'circle-check': 'M12 12m-9 0a9 9 0 1 0 18 0a9 9 0 1 0 -18 0M9 12l2 2l4 -4',
		'circle-off': 'M12 12m-9 0a9 9 0 1 0 18 0a9 9 0 1 0 -18 0M5.7 5.7l12.6 12.6',
		'circle-x': 'M12 12m-9 0a9 9 0 1 0 18 0a9 9 0 1 0 -18 0M10 10l4 4M14 10l-4 4',
		cloud:
			'M6.657 18c-2.572 0 -4.657 -2.007 -4.657 -4.483c0 -2.475 2.085 -4.482 4.657 -4.482c.393 -1.762 1.794 -3.2 3.675 -3.773c1.88 -.572 3.956 -.193 5.444 1c1.488 1.19 2.162 3.007 1.77 4.769h.99c1.913 0 3.464 1.56 3.464 3.486c0 1.927 -1.551 3.487 -3.465 3.487h-11.878',
		'cloud-download':
			'M19 18a3.5 3.5 0 0 0 0 -7h-1a5 4.5 0 0 0 -11 -2a4.6 4.4 0 0 0 -2.1 8.4M12 13l0 9M9 19l3 3l3 -3',
		coffee:
			'M3 14c.83 .642 2.077 1.017 3.5 1c1.423 .017 2.67 -.358 3.5 -1c.83 -.642 2.077 -1.017 3.5 -1c1.423 -.017 2.67 .358 3.5 1M8 3a2.4 2.4 0 0 0 -1 2a2.4 2.4 0 0 0 1 2M12 3a2.4 2.4 0 0 0 -1 2a2.4 2.4 0 0 0 1 2M3 10h14v5a6 6 0 0 1 -6 6h-2a6 6 0 0 1 -6 -6v-5M16.746 16.726a3 3 0 1 0 .252 -5.555',
		copy: 'M7 7m0 2.667a2.667 2.667 0 0 1 2.667 -2.667h8.666a2.667 2.667 0 0 1 2.667 2.667v8.666a2.667 2.667 0 0 1 -2.667 2.667h-8.666a2.667 2.667 0 0 1 -2.667 -2.667zM4.012 16.737a2.005 2.005 0 0 1 -1.012 -1.737v-10c0 -1.1 .9 -2 2 -2h10c.75 0 1.158 .385 1.5 1',
		dots: 'M5 12m-1 0a1 1 0 1 0 2 0a1 1 0 1 0 -2 0M12 12m-1 0a1 1 0 1 0 2 0a1 1 0 1 0 -2 0M19 12m-1 0a1 1 0 1 0 2 0a1 1 0 1 0 -2 0',
		edit: 'M7 7h-1a2 2 0 0 0 -2 2v9a2 2 0 0 0 2 2h9a2 2 0 0 0 2 -2v-1M20.385 6.585a2.1 2.1 0 0 0 -2.97 -2.97l-8.415 8.385v3h3l8.385 -8.415zM16 5l3 3',
		'external-link':
			'M12 6h-6a2 2 0 0 0 -2 2v10a2 2 0 0 0 2 2h10a2 2 0 0 0 2 -2v-6M11 13l9 -9M15 4h5v5',
		eye: 'M10 12a2 2 0 1 0 4 0a2 2 0 0 0 -4 0M21 12c-2.4 4 -5.4 6 -9 6c-3.6 0 -6.6 -2 -9 -6c2.4 -4 5.4 -6 9 -6c3.6 0 6.6 2 9 6',
		'eye-off':
			'M3 3l18 18M10.584 10.587a2 2 0 0 0 2.828 2.83M9.363 5.365a9.466 9.466 0 0 1 2.637 -.365c3.6 0 6.6 2 9 6c-.702 1.17 -1.47 2.163 -2.293 2.981m-2.404 1.63c-1.28 .5 -2.71 .759 -4.303 .759c-3.6 0 -6.6 -2 -9 -6c.995 -1.658 2.11 -2.94 3.342 -3.85',
		'info-circle': 'M12 12m-9 0a9 9 0 1 0 18 0a9 9 0 1 0 -18 0M12 9h.01M11 12h1v4h1',
		key: 'M16.555 3.843l3.602 3.602a2.877 2.877 0 0 1 0 4.069l-2.643 2.643a2.877 2.877 0 0 1 -4.069 0l-.301 -.301l-6.558 6.558a2 2 0 0 1 -1.239 .578l-.175 .008h-1.172a1 1 0 0 1 -.993 -.883l-.007 -.117v-1.172a2 2 0 0 1 .467 -1.284l.119 -.13l6.558 -6.558l-.3 -.3a2.877 2.877 0 0 1 0 -4.07l2.643 -2.643a2.877 2.877 0 0 1 4.069 0zM15 9h.01',
		layers: 'M12 3l8 4.5l-8 4.5l-8 -4.5zM4 12l8 4.5l8 -4.5M4 16.5l8 4.5l8 -4.5',
		'layout-sidebar':
			'M4 4m0 2a2 2 0 0 1 2 -2h12a2 2 0 0 1 2 2v12a2 2 0 0 1 -2 2h-12a2 2 0 0 1 -2 -2zM9 4l0 16',
		'layout-sidebar-right':
			'M4 4m0 2a2 2 0 0 1 2 -2h12a2 2 0 0 1 2 2v12a2 2 0 0 1 -2 2h-12a2 2 0 0 1 -2 -2zM15 4l0 16',
		loader:
			'M12 6l0 -3M16.25 7.75l2.15 -2.15M18 12l3 0M16.25 16.25l2.15 2.15M12 18l0 3M7.75 16.25l-2.15 2.15M6 12l-3 0M7.75 7.75l-2.15 -2.15',
		lock: 'M5 13a2 2 0 0 1 2 -2h10a2 2 0 0 1 2 2v6a2 2 0 0 1 -2 2h-10a2 2 0 0 1 -2 -2zM11 16a1 1 0 1 0 2 0a1 1 0 0 0 -2 0M8 11v-4a4 4 0 1 1 8 0v4',
		minus: 'M5 12l14 0',
		pencil: 'M4 20h4l10.5 -10.5a2.828 2.828 0 1 0 -4 -4l-10.5 10.5v4M13.5 6.5l4 4',
		'player-pause':
			'M6 5m0 1a1 1 0 0 1 1 -1h2a1 1 0 0 1 1 1v12a1 1 0 0 1 -1 1h-2a1 1 0 0 1 -1 -1zM14 5m0 1a1 1 0 0 1 1 -1h2a1 1 0 0 1 1 1v12a1 1 0 0 1 -1 1h-2a1 1 0 0 1 -1 -1z',
		'player-play': 'M7 4v16l13 -8z',
		'player-stop':
			'M5 5m0 2a2 2 0 0 1 2 -2h10a2 2 0 0 1 2 2v10a2 2 0 0 1 -2 2h-10a2 2 0 0 1 -2 -2z',
		plus: 'M12 5l0 14M5 12l14 0',
		refresh: 'M20 11a8.1 8.1 0 0 0 -15.5 -2m-.5 -4v4h4M4 13a8.1 8.1 0 0 0 15.5 2m.5 4v-4h-4',
		search: 'M10 10m-7 0a7 7 0 1 0 14 0a7 7 0 1 0 -14 0M21 21l-6 -6',
		server:
			'M3 4m0 3a3 3 0 0 1 3 -3h12a3 3 0 0 1 3 3v2a3 3 0 0 1 -3 3h-12a3 3 0 0 1 -3 -3zM3 15m0 3a3 3 0 0 1 3 -3h12a3 3 0 0 1 3 3v2a3 3 0 0 1 -3 3h-12a3 3 0 0 1 -3 -3zM7 8l0 .01M7 19l0 .01',
		tag: 'M7.5 7.5m-1 0a1 1 0 1 0 2 0a1 1 0 1 0 -2 0M3 6v5.172a2 2 0 0 0 .586 1.414l7.71 7.71a2.41 2.41 0 0 0 3.408 0l5.592 -5.592a2.41 2.41 0 0 0 0 -3.408l-7.71 -7.71a2 2 0 0 0 -1.414 -.586h-5.172a3 3 0 0 0 -3 3z',
		'toggle-left':
			'M8 12a2 2 0 1 0 4 0a2 2 0 1 0 -4 0M2 12a6 6 0 0 1 6 -6h8a6 6 0 0 1 0 12h-8a6 6 0 0 1 -6 -6z',
		trash:
			'M4 7l16 0M10 11l0 6M14 11l0 6M5 7l1 12a2 2 0 0 0 2 2h8a2 2 0 0 0 2 -2l1 -12M9 7v-3a1 1 0 0 1 1 -1h4a1 1 0 0 1 1 1v3',
		user: 'M8 7a4 4 0 1 0 8 0a4 4 0 0 0 -8 0M6 21v-2a4 4 0 0 1 4 -4h4a4 4 0 0 1 4 4v2',
		'currency-dollar':
			'M16.7 8a3 3 0 0 0 -2.7 -2h-4a3 3 0 0 0 0 6h4a3 3 0 0 1 0 6h-4a3 3 0 0 1 -2.7 -2M12 3v3M12 18v3',
		clock: 'M12 12m-9 0a9 9 0 1 0 18 0a9 9 0 1 0 -18 0M12 7v5l3 3',
		'trending-up': 'M3 17l6 -6l4 4l8 -8M14 7h7v7',
		'grip-vertical':
			'M9 5m-1 0a1 1 0 1 0 2 0a1 1 0 1 0 -2 0M9 12m-1 0a1 1 0 1 0 2 0a1 1 0 1 0 -2 0M9 19m-1 0a1 1 0 1 0 2 0a1 1 0 1 0 -2 0M15 5m-1 0a1 1 0 1 0 2 0a1 1 0 1 0 -2 0M15 12m-1 0a1 1 0 1 0 2 0a1 1 0 1 0 -2 0M15 19m-1 0a1 1 0 1 0 2 0a1 1 0 1 0 -2 0',
		// The glyphs below are drawn in the same 24x24 stroke-2 idiom as the
		// vendored Tabler set above, so the console keeps one icon weight.
		'arrow-down': 'M12 5l0 14M6 13l6 6l6 -6',
		'arrow-up': 'M12 5l0 14M6 11l6 -6l6 6',
		bolt: 'M13 3l0 7l6 0l-8 11l0 -7l-6 0z',
		'chart-bar': 'M4 20h16M8 20v-6M12 20v-10M16 20v-4',
		'copy-plus':
			'M7 7m0 2.667a2.667 2.667 0 0 1 2.667 -2.667h8.666a2.667 2.667 0 0 1 2.667 2.667v8.666a2.667 2.667 0 0 1 -2.667 2.667h-8.666a2.667 2.667 0 0 1 -2.667 -2.667zM12 13h4M14 11v4M4.012 16.737a2.005 2.005 0 0 1 -1.012 -1.737v-10c0 -1.1 .9 -2 2 -2h10',
		cpu: 'M5 5h14v14h-14zM9 9h6v6h-6zM9 2v3M15 2v3M9 19v3M15 19v3M2 9h3M2 15h3M19 9h3M19 15h3',
		database:
			'M12 6m-8 0a8 3 0 1 0 16 0a8 3 0 1 0 -16 0M4 6v12c0 1.66 3.58 3 8 3s8 -1.34 8 -3v-12M4 12c0 1.66 3.58 3 8 3s8 -1.34 8 -3',
		'device-desktop':
			'M3 5a1 1 0 0 1 1 -1h16a1 1 0 0 1 1 1v10a1 1 0 0 1 -1 1h-16a1 1 0 0 1 -1 -1zM8 20h8M12 16v4',
		filter: 'M4 5h16l-6 7v6l-4 2v-8z',
		gauge: 'M12 12m-9 0a9 9 0 1 0 18 0a9 9 0 1 0 -18 0M12 12l3.5 -3.5',
		language: 'M4 5h7M9 3v2c0 4.42 -2.24 8 -5 8M5 9c0 2.21 2.24 4 5 4M12 20l4 -9l4 9M13.5 17h5',
		'layout-dashboard': 'M4 4h6v7h-6zM4 15h6v5h-6zM14 10h6v10h-6zM14 4h6v3h-6z',
		'list-details': 'M9 6h11M9 12h11M9 18h11M4 6h.01M4 12h.01M4 18h.01',
		list: 'M9 6h11M9 12h11M9 18h11M5 6h.01M5 12h.01M5 18h.01',
		logout:
			'M14 8v-2a2 2 0 0 0 -2 -2h-5a2 2 0 0 0 -2 2v12a2 2 0 0 0 2 2h5a2 2 0 0 0 2 -2v-2M9 12h12M18 9l3 3l-3 3',
		'menu-2': 'M4 6l16 0M4 12l16 0M4 18l16 0',
		'message-square': 'M4 4h16v11h-6l-4 4v-4h-6zM8 9h8M8 13h5',
		moon: 'M12 3a6 6 0 0 0 9 9a9 9 0 1 1 -9 -9',
		palette:
			'M12 21a9 9 0 0 1 0 -18c4.97 0 9 3.58 9 8c0 1.66 -1.34 3 -3 3h-1.5a2 2 0 0 0 -1.41 3.41a1.5 1.5 0 0 1 -1.09 2.59zM8 9h.01M12 7h.01M16 9h.01',
		'plug-connected': 'M9 3v6M15 3v6M6 9h12v3a6 6 0 0 1 -12 0zM12 18v3',
		power: 'M12 3v9M7.5 6.5a7 7 0 1 0 9 0',
		route:
			'M4 18a2 2 0 1 0 4 0a2 2 0 0 0 -4 0M16 6a2 2 0 1 0 4 0a2 2 0 0 0 -4 0M6 16v-4a3 3 0 0 1 3 -3h6a3 3 0 0 0 3 -3',
		'ruler-2': 'M12 3v18M9 7l3 -4l3 4M9 17l3 4l3 -4',
		selector: 'M8 9l4 -4l4 4M8 15l4 4l4 -4',
		settings:
			'M12 12m-3 0a3 3 0 1 0 6 0a3 3 0 1 0 -6 0M12 3v2M12 19v2M3 12h2M19 12h2M5.6 5.6l1.4 1.4M17 17l1.4 1.4M18.4 5.6l-1.4 1.4M7 17l-1.4 1.4',
		sparkles:
			'M12 3l1.5 4.5l4.5 1.5l-4.5 1.5l-1.5 4.5l-1.5 -4.5l-4.5 -1.5l4.5 -1.5zM19 15l.8 2.2l2.2 .8l-2.2 .8l-.8 2.2l-.8 -2.2l-2.2 -.8l2.2 -.8z',
		sun: 'M12 12m-4 0a4 4 0 1 0 8 0a4 4 0 1 0 -8 0M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4 -1.4M17.7 6.3l1.4 -1.4',
		world:
			'M12 12m-9 0a9 9 0 1 0 18 0a9 9 0 1 0 -18 0M3 12h18M12 3c2.5 2.5 2.5 15 0 18M12 3c-2.5 2.5 -2.5 15 0 18',
		wrench: 'M7 10h3v-3l-3.5 -3.5a6 6 0 0 1 8 8l6 6a2 2 0 0 1 -3 3l-6 -6a6 6 0 0 1 -8 -8l3.5 3.5',
		x: 'M18 6l-12 12M6 6l12 12'
	};

	/** Reports whether a name is one the vendored set draws, so a name that arrives from a value
	 *  which has drifted renders nothing instead of an empty path. */
	export function isIconName(value: string): value is IconName {
		return value in paths;
	}
</script>

<script lang="ts">
	interface Props {
		name: IconName;
		size?: number;
		class?: string;
		spin?: boolean;
	}

	let { name, size = 16, class: className = '', spin = false }: Props = $props();
</script>

<svg
	data-icon-size={size}
	class={className + (spin ? ' animate-spin' : '')}
	width={className.includes('size-full') ? '100%' : size}
	height={className.includes('size-full') ? '100%' : size}
	viewBox="0 0 24 24"
	fill="none"
	stroke="currentColor"
	stroke-width="2"
	stroke-linecap="round"
	stroke-linejoin="round"
	aria-hidden="true"
>
	<path d={paths[name]}></path>
</svg>
