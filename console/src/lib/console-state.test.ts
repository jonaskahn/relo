import { beforeEach, describe, expect, it, vi } from 'vitest';

// The console's state is one holder the whole shell reads, and the appearance
// it saves is shared with every other console through the daemon's config. What
// is under test is that a failed write puts back only what it owns, and that an
// answer which arrives after a later choice is dropped rather than applied.

const mocks = vi.hoisted(() => {
	const store = new Map<string, string>();
	const element = {
		classList: { toggle: () => undefined, contains: () => false },
		dataset: {} as Record<string, string>,
		style: { setProperty: () => undefined }
	};
	vi.stubGlobal('document', { documentElement: element });
	vi.stubGlobal('window', {
		matchMedia: () => ({
			matches: false,
			addEventListener: () => undefined,
			removeEventListener: () => undefined
		})
	});
	vi.stubGlobal('localStorage', {
		getItem: (key: string) => store.get(key) ?? null,
		setItem: (key: string, value: string) => store.set(key, value)
	});
	return { store };
});

const apiMock = vi.hoisted(() => vi.fn());
const toastMock = vi.hoisted(() => ({ error: vi.fn(), success: vi.fn() }));

vi.mock('./api', () => ({ api: apiMock }));
vi.mock('svelte-sonner', () => ({ toast: toastMock }));

const { consoleState } = await import('./console-state.svelte');

beforeEach(() => {
	mocks.store.clear();
	apiMock.mockReset();
	toastMock.error.mockClear();
});

describe('console state', () => {
	it('shows a chosen theme before the daemon has stored it', async () => {
		apiMock.mockResolvedValue({});
		const saved = consoleState.setTheme('dark');
		expect(consoleState.settings.theme).toBe('dark');
		await saved;
	});

	it('puts the theme back when the daemon refuses it', async () => {
		apiMock.mockResolvedValue({});
		await consoleState.setTheme('dark');
		const before = consoleState.settings.theme;

		apiMock.mockRejectedValueOnce(new Error('offline'));
		await consoleState.setTheme('light');

		expect(consoleState.settings.theme).toBe(before);
		expect(toastMock.error).toHaveBeenCalled();
	});

	it('keeps a later choice when an earlier write fails after it', async () => {
		apiMock.mockResolvedValue({});
		await consoleState.setTheme('dark');

		let fail: (reason: unknown) => void = () => undefined;
		apiMock.mockImplementationOnce(
			() =>
				new Promise((_resolve, reject) => {
					fail = reject;
				})
		);
		const first = consoleState.setTheme('light');
		const second = consoleState.setTheme('dark');
		// The queue reaches the daemon a microtask after the choice is made.
		await Promise.resolve();
		fail(new Error('offline'));
		await Promise.all([first, second]);

		// The failed write no longer owns the choice, so it puts nothing back.
		expect(consoleState.settings.theme).toBe('dark');
	});

	it('drops an appearance answer a later choice has already replaced', async () => {
		let answer: (value: unknown) => void = () => undefined;
		apiMock.mockResolvedValue({});
		apiMock.mockImplementationOnce(
			() =>
				new Promise((resolve) => {
					answer = resolve;
				})
		);
		const loading = consoleState.loadAppearance();
		consoleState.setTheme('dark');
		answer({ appearance: { theme: 'light', accent: '#c8102e', quota_display: 'used' } });
		await loading;

		expect(consoleState.settings.theme).toBe('dark');
	});

	it('keeps the chosen theme and accent when the rest is reset', async () => {
		apiMock.mockResolvedValue({});
		await consoleState.setTheme('dark');
		await consoleState.setAccent('lagoon');
		consoleState.setSidebar({ mode: 'full' });

		consoleState.reset();

		expect(consoleState.settings).toMatchObject({
			theme: 'dark',
			accent: 'lagoon',
			sidebar: { mode: 'compact' }
		});
	});

	it('ignores an appearance handed back under a revision that has moved on', async () => {
		apiMock.mockResolvedValue({});
		await consoleState.setTheme('dark');
		const stale = consoleState.appearanceRevision;
		await consoleState.setTheme('light');

		consoleState.useAppearance({ theme: 'dark', accent: '#c8102e', quota_display: 'used' }, stale);

		expect(consoleState.settings.theme).toBe('light');
	});

	it('saves a radar colour as both the accent and the slot it came from', async () => {
		apiMock.mockResolvedValue({});
		await consoleState.setAccent('#1f8a8a');

		expect(consoleState.settings.accent).toBe('#1f8a8a');
		expect(consoleState.settings.customAccent).toBe('#1f8a8a');
		expect(JSON.parse(apiMock.mock.calls[0][1].body)).toEqual({ accent: '#1f8a8a' });
	});

	it('keeps the radar colour in its slot while a preset is the accent', async () => {
		apiMock.mockResolvedValue({});
		await consoleState.setAccent('#1f8a8a');
		await consoleState.setAccent('moss');

		expect(consoleState.settings.accent).toBe('moss');
		expect(consoleState.settings.customAccent).toBe('#1f8a8a');
		const stored = JSON.parse(mocks.store.get('relo.console.v1') ?? '{}');
		expect(stored.customAccent).toBe('#1f8a8a');
	});
});
