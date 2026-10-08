import { describe, expect, it } from 'vitest';

import {
	CONTEXT_PRESETS,
	contextLabel,
	contextMark,
	contextTargets,
	hasContextOverride,
	parseContextInput,
	roundTokenCount
} from './context-window';

describe('CONTEXT_PRESETS', () => {
	it('offers the sizes an operator reaches for, in order', () => {
		expect(CONTEXT_PRESETS).toEqual([
			128_000, 200_000, 256_000, 300_000, 400_000, 512_000, 800_000, 1_000_000
		]);
	});
});

describe('contextLabel', () => {
	it('keeps a unit for a round number', () => {
		expect(contextLabel(200_000)).toBe('200K');
		expect(contextLabel(1_000_000)).toBe('1M');
		expect(contextLabel(1_500_000)).toBe('1.5M');
	});
	it('writes millions with one useful decimal', () => {
		expect(contextLabel(1_045_000)).toBe('1M');
		expect(contextLabel(1_100_000)).toBe('1.1M');
		expect(contextLabel(1_457_000)).toBe('1.5M');
	});
	it('writes anything else out in full', () => {
		expect(contextLabel(262_144)).toBe((262_144).toLocaleString());
	});
	it('states nothing for an unset value', () => {
		expect(contextLabel(null)).toBe('');
	});
});

describe('roundTokenCount', () => {
	it('lands on the nearest thousand', () => {
		expect(roundTokenCount(262_144)).toBe(262_000);
		expect(roundTokenCount(1_456_789)).toBe(1_457_000);
		expect(roundTokenCount(1_999)).toBe(2_000);
		expect(roundTokenCount(4_096)).toBe(4_000);
		expect(roundTokenCount(200_000)).toBe(200_000);
	});
	it('keeps a converted suffix on K or M', () => {
		const parsed = parseContextInput('1.5M');
		expect(parsed.ok && contextLabel(roundTokenCount(parsed.value))).toBe('1.5M');
		const thousands = parseContextInput('262.144k');
		expect(thousands.ok && contextLabel(roundTokenCount(thousands.value))).toBe('262K');
	});
});

describe('parseContextInput', () => {
	it('reads a bare token count', () => {
		expect(parseContextInput('262144')).toEqual({ ok: true, value: 262_144 });
	});
	it('reads a decimal suffix', () => {
		expect(parseContextInput('300k')).toEqual({ ok: true, value: 300_000 });
		expect(parseContextInput('1.5M')).toEqual({ ok: true, value: 1_500_000 });
	});
	it('refuses what it cannot read', () => {
		expect(parseContextInput('')).toEqual({ ok: false, reason: 'empty' });
		expect(parseContextInput('abc')).toEqual({ ok: false, reason: 'unreadable' });
	});
	it('refuses a value outside the range', () => {
		expect(parseContextInput('999')).toEqual({ ok: false, reason: 'range' });
		expect(parseContextInput('200M')).toEqual({ ok: false, reason: 'range' });
	});
});

describe('hasContextOverride', () => {
	it('reports a hand-set value', () => {
		expect(hasContextOverride({ override: 200_000, provider: null, modelsdev: null })).toBe(true);
		expect(hasContextOverride({ override: null, provider: 200_000, modelsdev: null })).toBe(false);
		expect(hasContextOverride(undefined)).toBe(false);
	});
});

describe('contextMark', () => {
	it('prints nothing while the layers decide', () => {
		expect(contextMark({ override: null, provider: 200_000, modelsdev: null }, 262_144)).toBe('');
		expect(contextMark(undefined, 262_144)).toBe('');
	});
	it('marks the operator’s own override', () => {
		expect(contextMark({ override: 200_000, provider: null, modelsdev: null }, 262_144)).toBe('*');
	});
	it('warns when the override is above the maximum input the model states', () => {
		expect(contextMark({ override: 1_000_000, provider: null, modelsdev: null }, 262_144)).toBe(
			'!'
		);
	});
	it('cannot be above a maximum the model does not state', () => {
		expect(contextMark({ override: 1_000_000, provider: null, modelsdev: null }, null)).toBe('*');
	});
});

describe('contextTargets', () => {
	function model(model_id: string, listed: boolean) {
		return {
			model_id,
			context_window: null,
			override: null,
			inherited: null,
			max_input: null,
			listed
		};
	}

	it('sizes the models the account is known to serve', () => {
		const models = [model('gpt-5', true), model('gpt-4o', false), model('o3', true)];

		expect(contextTargets(models)).toEqual(['gpt-5', 'o3']);
	});

	it('falls back to the whole list when the account serves none of them', () => {
		expect(contextTargets([model('gpt-5', false), model('o3', false)])).toEqual(['gpt-5', 'o3']);
	});

	it('names nothing for an account with no models', () => {
		expect(contextTargets([])).toEqual([]);
	});
});
