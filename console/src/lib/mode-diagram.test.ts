import { describe, expect, it } from 'vitest';

import { modeDiagram, type DiagramLabels, type NodeId } from './mode-diagram';

const labels: DiagramLabels = {
	client: { name: 'Claude Code', sub: 'settings file' },
	intercept: { name: 'Intercept', sub: '127.0.0.1' },
	relo: { name: 'Relo', sub: 'data plane' },
	providers: { name: 'Providers', sub: 'your models' },
	anthropic: { name: 'Anthropic', sub: 'your login' }
};

const ids = (items: { id: string }[]) => items.map((item) => item.id).sort();

describe('modeDiagram', () => {
	it('sends every request to Relo in environment mode and leaves Anthropic out of the path', () => {
		const diagram = modeDiagram({ mode: 'env', width: 640, labels });
		expect(ids(diagram.nodes)).toEqual(['anthropic', 'client', 'providers', 'relo']);
		expect(ids(diagram.edges)).toEqual(['in', 'off', 'out']);
		expect(diagram.nodes.find((node) => node.id === 'anthropic')?.dim).toBe(true);
		expect(diagram.edges.find((edge) => edge.id === 'off')?.flows).toBe(false);
		expect(diagram.edges.every((edge) => edge.chip === null)).toBe(true);
	});

	it('forks at the intercept in proxy mode: bound models to Relo, the rest to Anthropic', () => {
		const diagram = modeDiagram({ mode: 'proxy', width: 640, labels });
		expect(ids(diagram.nodes)).toEqual(['anthropic', 'client', 'intercept', 'providers', 'relo']);
		expect(ids(diagram.edges)).toEqual(['bound', 'in', 'native', 'out']);
		expect(diagram.nodes.find((node) => node.id === 'anthropic')?.dim).toBe(false);
		const branches = diagram.edges.filter((edge) => edge.chip !== null).map((edge) => edge.kind);
		expect(branches.sort()).toEqual(['bound', 'native']);
		expect(diagram.edges.every((edge) => edge.flows)).toBe(true);
	});

	it('keeps every node in place across modes, so only the lines move', () => {
		const spot = (mode: 'env' | 'proxy', id: NodeId) => {
			const node = modeDiagram({ mode, width: 640, labels }).nodes.find((n) => n.id === id);
			return node && { x: node.x, y: node.y, width: node.width };
		};
		for (const id of ['client', 'relo', 'providers', 'anthropic'] as const) {
			expect(spot('env', id)).toEqual(spot('proxy', id));
		}
	});

	it('lays the path out in a row on a wide canvas and in a column on a narrow one', () => {
		const wide = modeDiagram({ mode: 'proxy', width: 640, labels });
		const narrow = modeDiagram({ mode: 'proxy', width: 300, labels });
		expect(wide.orientation).toBe('row');
		expect(narrow.orientation).toBe('column');
		const x = (diagram: typeof wide, id: NodeId) => diagram.nodes.find((n) => n.id === id)!;
		expect(x(wide, 'client').x).toBeLessThan(x(wide, 'relo').x);
		expect(x(wide, 'client').y).toBeGreaterThan(x(wide, 'relo').y - 1);
		expect(x(narrow, 'client').y).toBeLessThan(x(narrow, 'relo').y);
	});

	it('keeps every node inside the canvas', () => {
		for (const width of [280, 360, 480, 560, 720]) {
			for (const mode of ['env', 'proxy'] as const) {
				const diagram = modeDiagram({ mode, width, labels });
				for (const node of diagram.nodes) {
					expect(node.x).toBeGreaterThanOrEqual(0);
					expect(node.y).toBeGreaterThanOrEqual(0);
					expect(node.x + node.width).toBeLessThanOrEqual(diagram.width);
					expect(node.y + node.height).toBeLessThanOrEqual(diagram.height);
				}
			}
		}
	});

	it('widens a node for the longer words of another language, and may stack to fit them', () => {
		const long: DiagramLabels = {
			...labels,
			client: { name: 'Claude Code', sub: 'Einstellungsdatei der Anwendung' }
		};
		const diagram = modeDiagram({ mode: 'env', width: 560, labels: long });
		const client = diagram.nodes.find((n) => n.id === 'client')!;
		expect(client.width).toBeGreaterThan(
			modeDiagram({ mode: 'env', width: 560, labels }).nodes[0].width
		);
		expect(['row', 'column']).toContain(diagram.orientation);
	});

	it('draws each line from a node to the next as one path', () => {
		const diagram = modeDiagram({ mode: 'proxy', width: 640, labels });
		for (const edge of diagram.edges) {
			expect(edge.path).toMatch(/^M [\d.]+ [\d.]+ C ([\d.]+ ){5}[\d.]+$/);
		}
	});
});
