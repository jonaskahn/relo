// The routing tunnel: the routes one window's requests took through Relo. The agents
// stand on one rail, Relo in the middle, and the connections on the other, and every
// request is a line from an agent through Relo to the connection that served it.
//
// The model is pure, so the geometry can be tested without a canvas: the component draws
// what these functions decide and decides nothing itself.

import { CLIENT_OPTIONS, type ClientOption } from './client-setup';
import type { AccessKey, Provider, UsageLogRow, UsageRollupRow } from './types';

/** How many nodes one rail holds. */
export const RAIL_LIMIT = 10;

/** The narrowest canvas that still holds two rails of pills and the hub between them.
 *  Below it the card draws the rails as lists instead. */
export const MIN_TUNNEL_WIDTH = 680;

/** The id the daemon gives a client the operator named by hand. The console lists that
 *  choice as "other". */
const CUSTOM_CLIENT = 'custom';

/** The tile a node's mark wears, and the mark inside it. */
const TILE = 26;
const TILE_RADIUS = 7;
const MARK = 16;
/** The pill a node stands as: its height, its corners and the air inside it. */
const PILL_HEIGHT = 36;
const PILL_RADIUS = 12;
const PILL_PAD = 7;
const PILL_GAP = 8;
/** The room between a pill and the line that leaves it. */
const LINE_GAP = 12;
/** The room a curve needs between the hub's edge and a pill, so it sweeps rather than
 *  hooks. */
const CURVE_ROOM = 104;
/** The air two rows on a rail always keep, so a scattered row never lands on its
 *  neighbour. */
const MIN_GAP = 10;
/** The space the rows keep from the canvas's edges. */
const EDGE = 12;
/** The space one row takes on a wide canvas. */
const ROW_PITCH = 50;
/** The height a canvas takes for one row, and the cap a full one reaches. */
const MIN_HEIGHT = 180;
const MAX_HEIGHT = 420;
/** The width of a letter of the 13px system font, and of the mono count beside it. */
const LETTER = 7;
const COUNT_LETTER = 7.2;
/** The hub: the box every route passes through. */
const HUB_MIN = 104;
const HUB_MAX = 132;
const HUB_HEIGHT = 116;

/** One end of the tunnel: the name it carries, the mark it wears, and how many of the
 *  window's requests passed through it. */
export interface TunnelNode {
	id: string;
	label: string;
	markId: string;
	count: number;
}

/** The traffic of the window the tunnel draws: the rollups of the keys that spoke and of
 *  the connections that served them, and the newest request that belongs to it. */
export interface TunnelTraffic {
	/** The rollup of the keys that spoke, by the key's name. */
	clients: readonly UsageRollupRow[];
	/** The rollup of the connections that served, by the connection's id. */
	connections: readonly UsageRollupRow[];
	/** The newest logged request, whose route the tunnel follows when it belongs to the
	 *  window. */
	newest?: UsageLogRow | null;
	/** When the window began; an older request belongs to an earlier window. */
	since: number;
}

/** What the tunnel needs to name the nodes of a window. */
export interface TunnelContext {
	/** The keys the operator holds, which name the coding client each key belongs to. */
	keys: readonly AccessKey[];
	/** The connections the operator holds, which name each provider. */
	providers: readonly Provider[];
	/** What the console's own test traffic is called. */
	console: string;
	/** How many nodes one rail holds. */
	limit: number;
}

/** The routes the window's requests took, as the tunnel draws them. */
export interface RouteTunnel {
	agents: TunnelNode[];
	connections: TunnelNode[];
	/** The newest request in the window: the route the tunnel follows. */
	active: { agent: string; connection: string } | null;
	/** The requests the window held, the number the hub states. */
	total: number;
}

/** A point on the tunnel's canvas. */
export interface TunnelPoint {
	x: number;
	y: number;
}

/** The line that carries one node into Relo: the flat run beside its pill, and the curve
 *  that leaves that run. */
export interface TunnelStroke {
	/** Where the line starts, beside the pill. */
	from: TunnelPoint;
	/** Where the flat run ends and the curve begins. */
	flat: TunnelPoint;
	/** Where the curve lands, on the hub. */
	hub: TunnelPoint;
}

/** The tile a node's mark sits in. */
export interface TunnelTile {
	x: number;
	y: number;
	size: number;
	/** The corner the tile's radius gives it. */
	radius: number;
}

/** The pill a node stands as: its box, its mark, its name and its count. */
export interface TunnelPill {
	x: number;
	y: number;
	width: number;
	height: number;
	/** The tile the node's mark sits in. */
	tile: TunnelTile;
	/** Where the mark sits. */
	mark: TunnelPoint;
	/** Where the name starts. */
	textX: number;
	/** Where the count starts. */
	countX: number;
	/** The baseline the name and the count share. */
	baseline: number;
}

/** Where one node stands: its pill, its name, and the line it rides. */
export interface TunnelPlacement {
	node: TunnelNode;
	/** The pill the node stands as. */
	pill: TunnelPill;
	/** The name as it is drawn, cut to the room the rail has. */
	text: string;
	/** The line the node rides. */
	stroke: TunnelStroke;
}

/** The tunnel as it is drawn: the rails, the hub, and every line. */
export interface TunnelGeometry {
	width: number;
	height: number;
	agents: TunnelPlacement[];
	connections: TunnelPlacement[];
	/** The line each rail takes, one per node. */
	legs: { id: string; path: string; count: number; width: number }[];
	/** The line the newest request took, empty when there is none. */
	active: { path: string; agent: string; connection: string };
	/** Relo, the hub every route passes through. */
	hub: { x: number; y: number; width: number; height: number };
	/** The requests the window held. */
	total: number;
}

/** The client the console knows, or the one the operator named by hand. */
function clientOption(id: string): ClientOption | undefined {
	if (id === CUSTOM_CLIENT) return CLIENT_OPTIONS.find((entry) => entry.id === 'other');
	return CLIENT_OPTIONS.find((entry) => entry.id === id);
}

/** The node a coding client stands as: the client the console knows, or the app itself
 *  when it knows none. */
function clientNode(app: string): Omit<TunnelNode, 'count'> {
	const client = clientOption(app);
	if (client) {
		return { id: 'client:' + client.id, label: client.label, markId: client.logoId ?? client.id };
	}
	return { id: 'client:' + app, label: app, markId: app };
}

/** The node a key's traffic stands as: the coding client the key belongs to, the key
 *  itself when it belongs to none, and the console for its own test traffic. */
function keyNode(name: string, context: TunnelContext): Omit<TunnelNode, 'count'> {
	if (name === '') return { id: 'console', label: context.console, markId: '' };
	const key = context.keys.find((entry) => entry.name === name);
	if (key?.client) return clientNode(key.client);
	return { id: 'key:' + name, label: name, markId: '' };
}

/** The node a request was made by: the coding client it named, or the key it used. */
function requestNode(row: UsageLogRow, context: TunnelContext): Omit<TunnelNode, 'count'> {
	if (row.ClientApp) return clientNode(row.ClientApp);
	return keyNode(row.ClientKeyName, context);
}

/** The agents of a window: one node per coding client, carrying the requests every one
 *  of its keys sent. */
function agentNodes(rows: readonly UsageRollupRow[], context: TunnelContext): TunnelNode[] {
	const nodes: TunnelNode[] = [];
	for (const row of rows) {
		if (row.Requests <= 0) continue;
		const node = keyNode(row.Key, context);
		const found = nodes.find((entry) => entry.id === node.id);
		if (found) {
			found.count += row.Requests;
			continue;
		}
		nodes.push({ ...node, count: row.Requests });
	}
	return nodes;
}

/** The connections of a window: one node per connection, carrying the requests it
 *  served. A request that reached no connection took no route, so it is no node. */
function connectionNodes(
	rows: readonly UsageRollupRow[],
	providers: readonly Provider[]
): TunnelNode[] {
	return rows
		.filter((row) => row.Requests > 0 && row.Key !== '')
		.map((row) => {
			const provider = providers.find((entry) => entry.id === row.Key);
			return {
				id: row.Key,
				label: provider?.label ?? row.Key,
				markId: provider?.template_id || provider?.id || row.Key,
				count: row.Requests
			};
		});
}

/** A stable number for an id. A node's place on its rail comes from this and not from how
 *  much traffic it carried, so the picture keeps its shape while the traffic moves under
 *  it. */
function scatter(id: string): number {
	let hash = 0x811c9dc5;
	for (let index = 0; index < id.length; index += 1) {
		hash ^= id.charCodeAt(index);
		hash = Math.imul(hash, 0x01000193);
	}
	return hash >>> 0;
}

/** The nodes one rail draws: the busiest of the window, and the one the newest
 *  request's route runs through, so the live route always has an end to land on. */
function rail(nodes: TunnelNode[], limit: number, pinned: string): TunnelNode[] {
	const kept = nodes
		.slice()
		.sort((left, right) => right.count - left.count || left.id.localeCompare(right.id))
		.slice(0, limit);
	if (pinned !== '' && !kept.some((node) => node.id === pinned)) {
		const node = nodes.find((entry) => entry.id === pinned);
		const quietest = kept.at(-1);
		if (node && quietest && quietest.count > node.count) kept.pop();
		if (node) kept.push(node);
	}
	return kept.sort(
		(left, right) => scatter(left.id) - scatter(right.id) || left.id.localeCompare(right.id)
	);
}

/** Reads a window's traffic as the tunnel draws it: the agents that spoke, the
 *  connections that served them, and the route the newest request in the window took. */
export function routeTunnel(traffic: TunnelTraffic, context: TunnelContext): RouteTunnel {
	const agents = agentNodes(traffic.clients, context);
	const connections = connectionNodes(traffic.connections, context.providers);
	const newest = traffic.newest;
	const active =
		newest && newest.TimestampMs >= traffic.since
			? { agent: requestNode(newest, context).id, connection: newest.Provider }
			: null;
	return {
		agents: rail(agents, context.limit, active?.agent ?? ''),
		connections: rail(connections, context.limit, active?.connection ?? ''),
		active,
		total: traffic.clients.reduce((sum, row) => sum + row.Requests, 0)
	};
}

/** Cuts a name to the room its rail has. A letter of the 13px system font is about 7px
 *  wide, so the room is counted in letters; the full name stays in the tooltip. */
function cut(text: string, width: number): string {
	const max = Math.floor(width / LETTER);
	if (max < 2 || text.length <= max) return text;
	return text.slice(0, Math.max(1, max - 1)).trimEnd() + '…';
}

/** The curve that carries a line from one point to another, leaving flat and arriving flat.
 *  The axes say which way each end leaves: 'x' is flat along the row, 'y' is flat along
 *  the column. */
export function tunnelBend(
	from: TunnelPoint,
	to: TunnelPoint,
	flat: 'x' | 'y',
	arrive: 'x' | 'y'
): string {
	const dx = to.x - from.x;
	const dy = to.y - from.y;
	const first =
		flat === 'x' ? { x: from.x + dx * 0.45, y: from.y } : { x: from.x, y: from.y + dy * 0.45 };
	const second =
		arrive === 'x' ? { x: to.x - dx * 0.45, y: to.y } : { x: to.x, y: to.y - dy * 0.45 };
	return `C ${first.x} ${first.y}, ${second.x} ${second.y}, ${to.x} ${to.y}`;
}

/** The line from a name into the hub: flat beside the name, then one curve into the hub. */
function legPath(stroke: TunnelStroke, arrive: 'x' | 'y'): string {
	return (
		`M ${stroke.from.x} ${stroke.from.y} H ${stroke.flat.x} ` +
		tunnelBend(stroke.flat, stroke.hub, 'x', arrive)
	);
}

/** The line from the hub out to a name. */
function legFromHub(stroke: TunnelStroke, depart: 'x' | 'y'): string {
	return (
		`M ${stroke.hub.x} ${stroke.hub.y} ` +
		tunnelBend(stroke.hub, stroke.flat, depart, 'x') +
		` H ${stroke.from.x}`
	);
}

/** The whole route of one request: in from the agent, across the hub, out to the
 *  connection. */
function routePath(
	agent: TunnelStroke,
	connection: TunnelStroke,
	arrive: 'x' | 'y',
	depart: 'x' | 'y'
): string {
	return (
		legPath(agent, arrive) +
		` L ${connection.hub.x} ${connection.hub.y} ` +
		tunnelBend(connection.hub, connection.flat, depart, 'x') +
		` H ${connection.from.x}`
	);
}

/** How thick a line carrying this many requests is drawn. */
function legWidth(count: number): number {
	return Math.min(3.5, 1.5 + (count - 1) / 10);
}

/** How far a line runs flat beside a pill before it curves into the hub. The run keeps
 *  room for the curve, so the curve sweeps rather than hooks. */
function flatRun(span: number): number {
	return Math.max(0, Math.min(76, Math.max(12, span * 0.62), span - 12));
}

/** How far a row may drift from its place, so a scattered row never lands on its
 *  neighbour. */
function scatterRoom(pitch: number): number {
	return Math.max(0, Math.min((pitch - MIN_GAP) / 2, pitch / 4));
}

/** The heights the rows of a rail stand at: spread over the band the rail has, each row
 *  moved by the rail's scatter. */
interface RailOptions {
	/** How many rows the rail holds. */
	count: number;
	/** The band's top edge, where the first row's pill starts. */
	top: number;
	/** The band's bottom edge, where the last row's pill ends. */
	bottom: number;
	/** The height of a row's pill. */
	pill: number;
}

/** The row heights of one rail. The same slot moves the same amount on both rails, so the
 *  two sides stay mirrored, and the movement is bounded so two rows always keep their
 *  gap. */
function railHeights(options: RailOptions): number[] {
	const span = options.bottom - options.top - options.pill;
	const pitch = options.count > 1 ? span / (options.count - 1) : 0;
	const room = scatterRoom(pitch);
	const first = options.count > 1 ? options.top + options.pill / 2 : options.top + span / 2;
	return Array.from({ length: options.count }, (_, index) => {
		const base = first + index * pitch;
		const moved = base + (scatter('slot-' + index) / 0x100000000 - 0.5) * 2 * room;
		return Math.min(
			options.bottom - options.pill / 2,
			Math.max(options.top + options.pill / 2, moved)
		);
	});
}

/** One row of a rail: the pill, the name, the count, and the line that leaves them. */
interface RowOptions {
	node: TunnelNode;
	/** The row's centre line. */
	y: number;
	/** Which rail the row stands on: the agents' rail hugs the hub from the left, the
	 *  connections' rail from the right. */
	side: 'left' | 'right';
	/** The edge the pill's inner side stands on. */
	edge: number;
	/** The room the name may take. */
	room: number;
	/** The point on the hub the line lands on. */
	hub: TunnelPoint;
}

function row(options: RowOptions): TunnelPlacement {
	const count = String(options.node.count);
	const text = cut(options.node.label, options.room);
	const nameWidth = text.length * LETTER;
	const countWidth = count.length * COUNT_LETTER;
	const width = 2 * PILL_PAD + TILE + PILL_GAP + nameWidth + PILL_GAP + countWidth;
	const x = options.side === 'left' ? options.edge - width : options.edge;
	const baseline = options.y + 4;
	const pill: TunnelPill = {
		x,
		y: options.y - PILL_HEIGHT / 2,
		width,
		height: PILL_HEIGHT,
		tile: { x: x + PILL_PAD, y: options.y - TILE / 2, size: TILE, radius: TILE_RADIUS },
		mark: { x: x + PILL_PAD + TILE / 2, y: options.y },
		textX: x + PILL_PAD + TILE + PILL_GAP,
		countX: x + PILL_PAD + TILE + PILL_GAP + nameWidth + PILL_GAP,
		baseline
	};
	const from = {
		x: options.side === 'left' ? options.edge + LINE_GAP : options.edge - LINE_GAP,
		y: options.y
	};
	const run = flatRun(Math.abs(options.hub.x - from.x));
	const flat = {
		x: options.side === 'left' ? from.x + run : from.x - run,
		y: options.y
	};
	return {
		node: options.node,
		pill,
		text,
		stroke: { from, flat, hub: options.hub }
	};
}

/** The height a wide canvas takes for this many rows: the rows spread over the card, so a
 *  quiet window is a short card and a busy one fills the cap. */
function wideHeight(rows: number): number {
	const band = Math.max(0, rows - 1) * ROW_PITCH + PILL_HEIGHT + 2 * scatterRoom(ROW_PITCH);
	return Math.min(MAX_HEIGHT, Math.max(MIN_HEIGHT, EDGE + band + EDGE));
}

/** The height the rows of a tunnel need, which is the height its canvas takes when
 *  nothing else gives it more. */
export function tunnelHeight(tunnel: RouteTunnel): number {
	return wideHeight(Math.max(tunnel.agents.length, tunnel.connections.length));
}

/** The tunnel on a wide canvas: the agents on the left, Relo in the middle, the
 *  connections on the right. The rows spread over the card's height, so the map reads
 *  the same whether two clients are talking or ten. */
function wideGeometry(tunnel: RouteTunnel, width: number, height: number): TunnelGeometry {
	const rows = Math.max(tunnel.agents.length, tunnel.connections.length);
	const cardHeight = Math.max(height, wideHeight(rows));
	const top = EDGE;
	const bottom = cardHeight - EDGE;
	const centre = (top + bottom) / 2;
	const hubWidth = Math.min(HUB_MAX, Math.max(HUB_MIN, width * 0.13));
	const hub = {
		x: width / 2 - hubWidth / 2,
		y: centre - HUB_HEIGHT / 2,
		width: hubWidth,
		height: HUB_HEIGHT
	};
	const hubLeft = { x: hub.x, y: centre };
	const hubRight = { x: hub.x + hub.width, y: centre };
	// The rails hug the hub: a pill stands on the edge the curve leaves from, so the
	// lines run the space between the rail and the hub.
	const clientEdge = hub.x - CURVE_ROOM;
	const providerEdge = hub.x + hub.width + CURVE_ROOM;
	const agentHeights = railHeights({
		count: tunnel.agents.length,
		top,
		bottom,
		pill: PILL_HEIGHT
	});
	const connectionHeights = railHeights({
		count: tunnel.connections.length,
		top,
		bottom,
		pill: PILL_HEIGHT
	});
	const agents = tunnel.agents.map((node, index) => {
		const count = String(node.count);
		const room =
			clientEdge - EDGE - 2 * PILL_PAD - TILE - 2 * PILL_GAP - count.length * COUNT_LETTER;
		return row({
			node,
			y: agentHeights[index],
			side: 'left',
			edge: clientEdge,
			room,
			hub: hubLeft
		});
	});
	const connections = tunnel.connections.map((node, index) => {
		const count = String(node.count);
		const room =
			width -
			EDGE -
			providerEdge -
			2 * PILL_PAD -
			TILE -
			2 * PILL_GAP -
			count.length * COUNT_LETTER;
		return row({
			node,
			y: connectionHeights[index],
			side: 'right',
			edge: providerEdge,
			room,
			hub: hubRight
		});
	});

	return {
		width,
		height: cardHeight,
		agents,
		connections,
		legs: [
			...agents.map((placement) => ({
				id: 'agent:' + placement.node.id,
				path: legPath(placement.stroke, 'x'),
				count: placement.node.count,
				width: legWidth(placement.node.count)
			})),
			...connections.map((placement) => ({
				id: 'connection:' + placement.node.id,
				path: legFromHub(placement.stroke, 'x'),
				count: placement.node.count,
				width: legWidth(placement.node.count)
			}))
		],
		active: activePath(tunnel, agents, connections),
		hub,
		total: tunnel.total
	};
}

/** The line the newest request took, from its agent through Relo to its connection. */
function activePath(
	tunnel: RouteTunnel,
	agents: TunnelPlacement[],
	connections: TunnelPlacement[]
): { path: string; agent: string; connection: string } {
	const active = tunnel.active;
	const agent = active ? agents.find((placement) => placement.node.id === active.agent) : undefined;
	const connection = active
		? connections.find((placement) => placement.node.id === active.connection)
		: undefined;
	if (!active || !agent || !connection) {
		return { path: '', agent: '', connection: '' };
	}
	return {
		path: routePath(agent.stroke, connection.stroke, 'x', 'x'),
		agent: active.agent,
		connection: active.connection
	};
}

/** The tunnel as it is drawn for the room it has. */
export interface TunnelRoom {
	/** The width of the card's canvas. */
	width: number;
	/** The height the card may fill. */
	height: number;
}

export function tunnelGeometry(tunnel: RouteTunnel, room: TunnelRoom): TunnelGeometry {
	return wideGeometry(tunnel, Math.max(room.width, MIN_TUNNEL_WIDTH), room.height);
}

/** One rail as a list reads it: the node, its share of the busiest node beside it, and
 *  the width its bar takes. */
export interface RailShare {
	node: TunnelNode;
	/** The node's share of the rail's busiest node, from 0 to 1. */
	share: number;
}

/** The shares one rail's list draws, busiest first. */
export function railShares(nodes: readonly TunnelNode[]): RailShare[] {
	const busiest = nodes.reduce((max, node) => Math.max(max, node.count), 0);
	return nodes
		.slice()
		.sort((left, right) => right.count - left.count || left.id.localeCompare(right.id))
		.map((node) => ({ node, share: busiest > 0 ? node.count / busiest : 0 }));
}
