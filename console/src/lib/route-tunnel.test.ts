import { describe, expect, it } from 'vitest';

import { providerOf } from './provider-fixtures';
import {
	PHONE_RAIL_LIMIT,
	RAIL_LIMIT,
	routeTunnel,
	tunnelBend,
	tunnelGeometry,
	type RouteTunnel,
	type TunnelContext,
	type TunnelRoom,
	type TunnelTraffic
} from './route-tunnel';
import type { AccessKey, Provider, UsageLogRow, UsageRollupRow } from './types';

function rollup(key: string, requests: number): UsageRollupRow {
	return {
		Key: key,
		Requests: requests,
		Errors: 0,
		InputTokens: 100,
		OutputTokens: 20,
		CacheReadTokens: 5,
		CacheWriteTokens: 5,
		CostMicros: 1000,
		UnpricedRequests: 0,
		DurationMs: 0,
		Attempts: 0,
		RetriedRequests: 0,
		DurationMaxMs: 0
	};
}

function row(overrides: Partial<UsageLogRow> = {}): UsageLogRow {
	return {
		ID: 1,
		RequestID: 'r1',
		TimestampMs: 1000,
		Provider: 'openai',
		Model: 'gpt-5',
		RequestedModel: 'gpt-5',
		GroupID: null,
		CredentialLabel: 'work',
		Surface: 'chat',
		Status: 200,
		DurationMs: 1500,
		InputTokens: 1000,
		OutputTokens: 500,
		CacheReadTokens: 200,
		CacheWriteTokens: 100,
		EstimatedCostMicros: 1_200_000,
		RouteProvider: 'openai',
		RouteReason: '',
		ClientKeyID: 'key-1',
		ClientKeyName: 'Codex',
		ClientApp: 'codex',
		Origin: 'external',
		...overrides
	};
}

function key(name: string, client?: string): AccessKey {
	return {
		id: 'id-' + name,
		name,
		kind: 'agent',
		client,
		token_hint: 'sk-relo-…',
		generation: 1,
		status: 'active',
		created_at_ms: 0,
		updated_at_ms: 0
	};
}

function connections(...ids: string[]): Provider[] {
	return ids.map((id) => providerOf({ id, template_id: id, label: id }));
}

function context(overrides: Partial<TunnelContext> = {}): TunnelContext {
	return {
		keys: [key('Codex', 'codex'), key('Claude Code', 'claude-code')],
		providers: connections('openai', 'anthropic'),
		console: 'Console',
		limit: RAIL_LIMIT,
		...overrides
	};
}

function traffic(overrides: Partial<TunnelTraffic> = {}): TunnelTraffic {
	return {
		clients: [rollup('Codex', 10)],
		connections: [rollup('openai', 10)],
		since: 0,
		...overrides
	};
}

/** The tunnel as the card draws it, in a room of the test's choosing. */
function drawn(tunnel: RouteTunnel, room: Partial<TunnelRoom> = {}) {
	return tunnelGeometry(tunnel, { width: 900, height: 0, vertical: false, ...room });
}

describe('routeTunnel', () => {
	it('stands the agents on one rail and the connections on the other', () => {
		const tunnel = routeTunnel(traffic(), context());

		expect(tunnel.agents).toEqual([
			{ id: 'client:codex', label: 'Codex', markId: 'codex', count: 10 }
		]);
		expect(tunnel.connections).toEqual([
			{ id: 'openai', label: 'openai', markId: 'openai', count: 10 }
		]);
	});

	it('counts the requests the window held for every agent and connection', () => {
		const tunnel = routeTunnel(
			traffic({
				clients: [rollup('Codex', 30), rollup('Claude Code', 12)],
				connections: [rollup('openai', 25), rollup('anthropic', 17)]
			}),
			context()
		);

		expect(tunnel.agents.map((agent) => agent.id + ' ' + agent.count).sort()).toEqual([
			'client:claude-code 12',
			'client:codex 30'
		]);
		expect(tunnel.connections.map((entry) => entry.id + ' ' + entry.count).sort()).toEqual([
			'anthropic 17',
			'openai 25'
		]);
	});

	it('sums the keys of one coding client into one agent', () => {
		const tunnel = routeTunnel(
			traffic({ clients: [rollup('Codex', 8), rollup('Codex laptop', 5)] }),
			context({ keys: [key('Codex', 'codex'), key('Codex laptop', 'codex')] })
		);

		expect(tunnel.agents).toEqual([
			{ id: 'client:codex', label: 'Codex', markId: 'codex', count: 13 }
		]);
	});

	it('names a key that belongs to no client by that key', () => {
		const tunnel = routeTunnel(
			traffic({ clients: [rollup('laptop', 4)] }),
			context({ keys: [key('laptop')] })
		);

		expect(tunnel.agents).toEqual([{ id: 'key:laptop', label: 'laptop', markId: '', count: 4 }]);
	});

	it('names the console its own test traffic', () => {
		const tunnel = routeTunnel(traffic({ clients: [rollup('', 6)] }), context());

		expect(tunnel.agents).toEqual([{ id: 'console', label: 'Console', markId: '', count: 6 }]);
	});

	it('names a connection that is not configured any more by its own id', () => {
		const tunnel = routeTunnel(
			traffic({ connections: [rollup('gone', 3)] }),
			context({ providers: connections('openai') })
		);

		expect(tunnel.connections).toEqual([{ id: 'gone', label: 'gone', markId: 'gone', count: 3 }]);
	});

	it('keeps the ten busiest agents and connections of a window', () => {
		const clients = Array.from({ length: RAIL_LIMIT + 4 }, (_, index) =>
			rollup('client-' + index, index + 1)
		);
		const providers = Array.from({ length: RAIL_LIMIT + 4 }, (_, index) => 'provider-' + index);
		const tunnel = routeTunnel(
			traffic({
				clients,
				connections: providers.map((id, index) => rollup(id, index + 1))
			}),
			context({ providers: connections(...providers) })
		);

		expect(tunnel.agents).toHaveLength(RAIL_LIMIT);
		expect(tunnel.connections).toHaveLength(RAIL_LIMIT);
		const agents = tunnel.agents.map((agent) => agent.id);
		expect(agents).toContain('key:client-13');
		expect(agents).not.toContain('key:client-0');
	});

	it('keeps five nodes a rail on a phone', () => {
		const clients = Array.from({ length: RAIL_LIMIT }, (_, index) =>
			rollup('client-' + index, index + 1)
		);
		const tunnel = routeTunnel(traffic({ clients }), context({ limit: PHONE_RAIL_LIMIT }));

		expect(tunnel.agents).toHaveLength(PHONE_RAIL_LIMIT);
	});

	it('follows the newest request of the window', () => {
		const tunnel = routeTunnel(
			traffic({ newest: row({ ClientApp: 'claude-code', Provider: 'anthropic' }) }),
			context()
		);

		expect(tunnel.active).toEqual({ agent: 'client:claude-code', connection: 'anthropic' });
	});

	it('has no route when the newest request predates the window', () => {
		const tunnel = routeTunnel(
			traffic({ newest: row({ TimestampMs: 10 }), since: 1000 }),
			context()
		);

		expect(tunnel.active).toBeNull();
	});

	it('keeps the newest request on the rails even when it is not among the busiest', () => {
		const clients = Array.from({ length: RAIL_LIMIT }, (_, index) =>
			rollup('client-' + index, index + 10)
		);
		const tunnel = routeTunnel(
			traffic({
				clients: [...clients, rollup('Quiet', 1)],
				newest: row({ ClientApp: 'quiet', Provider: 'openai' })
			}),
			context({ keys: [key('Quiet', 'quiet')] })
		);

		expect(tunnel.agents.map((agent) => agent.id)).toContain('client:quiet');
		expect(tunnel.agents).toHaveLength(RAIL_LIMIT);
	});

	it('orders a rail by its ids, not by the traffic', () => {
		const first = routeTunnel(
			traffic({ clients: [rollup('Codex', 10), rollup('Claude Code', 1)] }),
			context()
		);
		const second = routeTunnel(
			traffic({ clients: [rollup('Claude Code', 1), rollup('Codex', 10)] }),
			context()
		);

		expect(first.agents.map((agent) => agent.id)).toEqual(second.agents.map((agent) => agent.id));
	});

	it('has no route when nothing was logged', () => {
		const tunnel = routeTunnel(traffic({ clients: [], connections: [] }), context());

		expect(tunnel.agents).toEqual([]);
		expect(tunnel.connections).toEqual([]);
		expect(tunnel.active).toBeNull();
	});
});

describe('tunnelGeometry', () => {
	const tunnel = routeTunnel(
		traffic({
			clients: [rollup('Codex', 10), rollup('Claude Code', 6)],
			connections: [rollup('openai', 10), rollup('anthropic', 6)],
			newest: row()
		}),
		context()
	);

	it('puts the agents on the left and the connections on the right', () => {
		const geometry = drawn(tunnel);
		const hub = geometry.hub;

		expect(geometry.agents[0].tile.x).toBeLessThan(hub.x);
		expect(geometry.connections[0].tile.x).toBeGreaterThan(hub.x + hub.width);
		expect(geometry.agents[0].label.anchor).toBe('start');
		expect(geometry.connections[0].label.anchor).toBe('end');
	});

	it('lands every line on the hub', () => {
		const geometry = drawn(tunnel);
		const agent = geometry.agents[0].stroke;
		const connection = geometry.connections[0].stroke;

		expect(agent.hub.x).toBe(geometry.hub.x);
		expect(connection.hub.x).toBe(geometry.hub.x + geometry.hub.width);
	});

	it('runs a line flat beside its name before it curves', () => {
		const geometry = drawn(tunnel);
		const agent = geometry.agents[0].stroke;

		expect(agent.flat.x).toBeGreaterThan(agent.from.x);
		expect(geometry.legs[0].path).toContain('H ');
		expect(geometry.legs[0].path).toContain('C ');
	});

	it('stacks the rails on a phone and names them the same way', () => {
		const geometry = drawn(tunnel, { width: 360, vertical: true });

		expect(geometry.agents[0].tile.y).toBeLessThan(geometry.hub.y);
		expect(geometry.connections[0].tile.y).toBeGreaterThan(geometry.hub.y);
		expect(geometry.agents[0].label.anchor).toBe('start');
		expect(geometry.connections[0].label.anchor).toBe('start');
	});

	it('runs a phone its lines into one spine instead of folding them back', () => {
		const geometry = drawn(tunnel, { width: 360, vertical: true });
		const spine = geometry.hub.x + geometry.hub.width / 2;

		for (const rail of [geometry.agents, geometry.connections]) {
			for (const placement of rail) {
				expect(placement.stroke.flat.x).toBeLessThan(spine);
				expect(placement.stroke.flat.x).toBeGreaterThan(placement.stroke.from.x);
			}
		}
	});

	it('draws one line per node and one for the newest route', () => {
		const geometry = drawn(tunnel);

		expect(geometry.legs).toHaveLength(4);
		expect(geometry.legs.every((leg) => leg.path.startsWith('M '))).toBe(true);
		expect(geometry.active.path).not.toBe('');
	});

	it('thicker lines carry more requests', () => {
		const busy = routeTunnel(
			traffic({
				clients: [rollup('Codex', 30)],
				connections: [rollup('openai', 30), rollup('anthropic', 2)]
			}),
			context()
		);
		const geometry = drawn(busy);
		const agent = geometry.legs.find((leg) => leg.id === 'agent:client:codex');
		const quiet = geometry.legs.find((leg) => leg.id === 'connection:anthropic');

		expect(agent?.width).toBeGreaterThan(quiet?.width ?? 0);
	});

	it('compresses the rows as the rails grow', () => {
		const clients = Array.from({ length: RAIL_LIMIT }, (_, index) =>
			rollup('client-' + index, index + 1)
		);
		const providers = Array.from({ length: RAIL_LIMIT }, (_, index) => 'provider-' + index);
		const many = routeTunnel(
			traffic({
				clients,
				connections: providers.map((id) => rollup(id, 1))
			}),
			context({ providers: connections(...providers) })
		);
		const geometry = drawn(many);

		const span = (geometry.agents.at(-1)?.tile.y ?? 0) - (geometry.agents[0]?.tile.y ?? 0);
		const pitch = span / (RAIL_LIMIT - 1);
		expect(pitch).toBeGreaterThanOrEqual(26);
		expect(pitch).toBeLessThan(64);
		expect(geometry.height).toBeLessThanOrEqual(420);
	});

	it('keeps a scattered row clear of its neighbour', () => {
		const clients = Array.from({ length: RAIL_LIMIT }, (_, index) =>
			rollup('client-' + index, index + 1)
		);
		const providers = Array.from({ length: RAIL_LIMIT }, (_, index) => 'provider-' + index);
		const many = routeTunnel(
			traffic({
				clients,
				connections: providers.map((id) => rollup(id, 1))
			}),
			context({ providers: connections(...providers) })
		);
		const geometry = drawn(many);

		for (const rail of [geometry.agents, geometry.connections]) {
			const heights = rail.map((placement) => placement.tile.y);
			const gaps = heights.slice(1).map((height, index) => height - heights[index]);
			expect(Math.min(...gaps)).toBeGreaterThanOrEqual(10);
		}
	});

	it('mirrors the two rails against each other', () => {
		const clients = Array.from({ length: RAIL_LIMIT }, (_, index) =>
			rollup('client-' + index, index + 1)
		);
		const providers = Array.from({ length: RAIL_LIMIT }, (_, index) => 'provider-' + index);
		const many = routeTunnel(
			traffic({
				clients,
				connections: providers.map((id) => rollup(id, 1))
			}),
			context({ providers: connections(...providers) })
		);
		const geometry = drawn(many);

		const agentHeights = geometry.agents.map((placement) => placement.tile.y);
		const connectionHeights = geometry.connections.map((placement) => placement.tile.y);
		expect(connectionHeights).toEqual(agentHeights);
	});

	it('takes the card its height from the rows it draws', () => {
		const few = routeTunnel(
			traffic({ clients: [rollup('Codex', 10)], connections: [rollup('openai', 10)] }),
			context()
		);
		const clients = Array.from({ length: RAIL_LIMIT }, (_, index) =>
			rollup('client-' + index, index + 1)
		);
		const many = routeTunnel(
			traffic({
				clients,
				connections: clients.map((_, index) => rollup('provider-' + index, 1))
			}),
			context({ providers: connections(...clients.map((_, index) => 'provider-' + index)) })
		);

		expect(drawn(few).height).toBeLessThan(drawn(many).height);
	});

	it('fills the height the card gives it', () => {
		const geometry = drawn(tunnel, { height: 500 });

		expect(geometry.height).toBe(500);
	});

	it('keeps the room its rows need when the card is short', () => {
		const clients = Array.from({ length: RAIL_LIMIT }, (_, index) =>
			rollup('client-' + index, index + 1)
		);
		const many = routeTunnel(
			traffic({
				clients,
				connections: clients.map((_, index) => rollup('provider-' + index, 1))
			}),
			context({ providers: connections(...clients.map((_, index) => 'provider-' + index)) })
		);
		const geometry = drawn(many, { height: 100 });

		expect(geometry.height).toBeGreaterThan(100);
	});

	it('spreads a short rail across the card', () => {
		const geometry = drawn(tunnel);

		const span = (geometry.agents.at(-1)?.tile.y ?? 0) - (geometry.agents[0]?.tile.y ?? 0);
		expect(span).toBeGreaterThan(100);
	});

	it('cuts a name that does not fit its rail and keeps it whole in the node', () => {
		const long = routeTunnel(traffic({ clients: [rollup('Codex', 10)] }), context());
		const geometry = drawn(long, { width: 300, vertical: true });

		expect(geometry.agents[0].text.length).toBeLessThan(geometry.agents[0].node.label.length + 1);
		expect(geometry.agents[0].node.label).toBe('Codex');
	});
});

describe('tunnelBend', () => {
	it('leaves and arrives flat', () => {
		const curve = tunnelBend({ x: 0, y: 10 }, { x: 100, y: 40 }, 'x', 'x');

		expect(curve.startsWith('C ')).toBe(true);
		expect(curve.endsWith('100 40')).toBe(true);
	});

	it('arrives from above when the hub is below', () => {
		const curve = tunnelBend({ x: 20, y: 10 }, { x: 100, y: 90 }, 'x', 'y');

		// The last control point sits above the hub, so the curve comes down into it.
		expect(curve).toContain('90');
	});
});
