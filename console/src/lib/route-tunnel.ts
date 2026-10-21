// The routing tunnel: the routes one window's requests took through Relo. The agents
// stand on one side, Relo in the middle, and the connections on the other, and every
// request is a line from an agent through Relo to the connection that served it.
//
// The model is pure, so the geometry can be tested without a canvas: the component draws
// what these functions decide and decides nothing itself.

import { CLIENT_OPTIONS, type ClientOption } from './client-setup';
import type { AccessKey, Provider, UsageLogRow, UsageRollupRow } from './types';

/** How many nodes one rail holds on a wide card. */
export const RAIL_LIMIT = 10;

/** How many nodes one rail holds on a phone, where ten would make the card a long
 *  column. */
export const PHONE_RAIL_LIMIT = 5;

/** The id the daemon gives a client the operator named by hand. The console lists that
 *  choice as "other". */
const CUSTOM_CLIENT = 'custom';

const TILE = 26;
const TILE_RADIUS = 7;
const MARK = 16;
const PHONE_TILE = 24;
const PHONE_TILE_RADIUS = 6;
const PHONE_MARK = 14;
const NAME_GAP = 8;
const LINE_GAP = 14;
const COUNT_GAP = 8;
const HUB_MIN = 56;
const HUB_MAX = 84;
const HUB_HEIGHT = 34;
/** The room a curve needs between the hub's edge and a name, so it sweeps rather than
 *  hooks. */
const CURVE_ROOM = 56;
/** The room a phone's curve needs between the spine and a name. */
const PHONE_CURVE_ROOM = 32;
/** The air two rows on a rail always keep, so a scattered row never lands on its
 *  neighbour. */
const MIN_GAP = 10;
const PHONE_PITCH = 32;
/** The space the rows keep from the card's edges. */
const EDGE = 12;
/** The space one row takes on a wide card. */
const ROW_PITCH = 50;
/** The height a wide card takes for one row, and the cap a full one reaches. */
const MIN_HEIGHT = 180;
const MAX_HEIGHT = 420;
/** The width of a letter of the 12px system font, and of the mono count beside it. */
const LETTER = 6.5;
const COUNT_LETTER = 7;

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
}

/** A point on the tunnel's canvas. */
export interface TunnelPoint {
	x: number;
	y: number;
}

/** The line that carries one node into Relo: the flat run beside its name, and the curve
 *  that leaves that run. */
export interface TunnelStroke {
	/** Where the line starts, beside the name. */
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
	/** The size of the mark inside the tile. */
	mark: number;
}

/** Where one node stands: its tile, its name, and the line it rides. */
export interface TunnelPlacement {
	node: TunnelNode;
	/** The tile the node's mark sits in. */
	tile: TunnelTile;
	/** The name as it is drawn, cut to the room the rail has. */
	text: string;
	/** Where the name stands. */
	label: TunnelText;
	stroke: TunnelStroke;
}

/** A piece of text the tunnel draws, and the way it reads. */
export interface TunnelText extends TunnelPoint {
	anchor: 'start' | 'end';
}

/** The tunnel as it is drawn: the rails, the hub, and every line. */
export interface TunnelGeometry {
	width: number;
	height: number;
	vertical: boolean;
	agents: TunnelPlacement[];
	connections: TunnelPlacement[];
	/** The line each rail takes, one per node. */
	legs: { id: string; path: string; count: number; width: number }[];
	/** The line the newest request took, empty when there is none. */
	active: { path: string; agent: string; connection: string };
	/** Relo, the hub every route passes through. */
	hub: { x: number; y: number; width: number; height: number };
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
		active
	};
}

/** Cuts a name to the room its rail has. A letter of the 12px system font is about 6.5px
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

/** How far a line runs flat beside a name before it curves into the hub. The run keeps
 *  room for the curve, so the curve sweeps rather than hooks. */
function flatRun(span: number): number {
	return Math.max(0, Math.min(60, Math.max(12, span * 0.35), span - 12));
}

/** How many letters the count beside a name takes. */
function countLetters(node: TunnelNode): number {
	return String(node.count).length;
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
	/** The band's top edge, where the first row's tile starts. */
	top: number;
	/** The band's bottom edge, where the last row's tile ends. */
	bottom: number;
	/** The height of a row's tile. */
	tile: number;
}

/** The row heights of one rail. The same slot moves the same amount on both rails, so the
 *  two sides stay mirrored, and the movement is bounded so two rows always keep their
 *  gap. */
function railHeights(options: RailOptions): number[] {
	const span = options.bottom - options.top - options.tile;
	const pitch = options.count > 1 ? span / (options.count - 1) : 0;
	const room = scatterRoom(pitch);
	const first = options.count > 1 ? options.top + options.tile / 2 : options.top + span / 2;
	return Array.from({ length: options.count }, (_, index) => {
		const base = first + index * pitch;
		const moved = base + (scatter('slot-' + index) / 0x100000000 - 0.5) * 2 * room;
		return Math.min(
			options.bottom - options.tile / 2,
			Math.max(options.top + options.tile / 2, moved)
		);
	});
}

/** One row of a rail: the tile, the name, the count, and the line that leaves them. */
interface RowOptions {
	node: TunnelNode;
	/** The row's centre line. */
	y: number;
	/** Where the tile sits. */
	tileX: number;
	/** The tile the node's mark sits in. */
	tile: { size: number; radius: number; mark: number };
	/** Where the name stands. */
	nameX: number;
	anchor: 'start' | 'end';
	/** The room the name may take. */
	room: number;
	/** The point on the hub the line lands on. */
	hub: TunnelPoint;
	/** Which way the line runs flat: towards the hub, or away from it. */
	direction: 1 | -1;
	/** Where the flat run ends, when the card does not run it beside the name. */
	flatX?: number;
}

function row(options: RowOptions): TunnelPlacement {
	const count = String(options.node.count);
	const text = cut(options.node.label, options.room);
	const width = text.length * LETTER + COUNT_GAP + count.length * COUNT_LETTER;
	const from = {
		x:
			options.anchor === 'start'
				? options.nameX + width + LINE_GAP
				: options.nameX - width - LINE_GAP,
		y: options.y
	};
	const run = flatRun(Math.abs(options.hub.x - from.x));
	const flat = {
		x: options.flatX ?? from.x + options.direction * run,
		y: options.y
	};
	return {
		node: options.node,
		tile: {
			x: options.tileX,
			y: options.y - options.tile.size / 2,
			size: options.tile.size,
			radius: options.tile.radius,
			mark: options.tile.mark
		},
		text,
		label: { x: options.nameX, y: options.y + 4, anchor: options.anchor },
		stroke: { from, flat, hub: options.hub }
	};
}

/** The height a wide card takes for this many rows: the rows spread over the card, so a
 *  quiet window is a short card and a busy one fills the cap. */
function wideHeight(rows: number): number {
	const band = Math.max(0, rows - 1) * ROW_PITCH + TILE + 2 * scatterRoom(ROW_PITCH);
	return Math.min(MAX_HEIGHT, Math.max(MIN_HEIGHT, EDGE + band + EDGE));
}

/** The tunnel on a wide card: the agents on the left, Relo in the middle, the connections
 *  on the right. The rows spread over the card's height, so the map reads the same
 *  whether two clients are talking or ten. */
function wideGeometry(tunnel: RouteTunnel, width: number, height: number): TunnelGeometry {
	const rows = Math.max(tunnel.agents.length, tunnel.connections.length);
	const cardHeight = Math.max(height, wideHeight(rows));
	const top = EDGE;
	const bottom = cardHeight - EDGE;
	const centre = (top + bottom) / 2;
	const hubWidth = Math.min(HUB_MAX, Math.max(HUB_MIN, width * 0.14));
	const hub = {
		x: width / 2 - hubWidth / 2,
		y: centre - HUB_HEIGHT / 2,
		width: hubWidth,
		height: HUB_HEIGHT
	};
	const hubLeft = { x: hub.x, y: centre };
	const hubRight = { x: hub.x + hub.width, y: centre };
	const tile = { size: TILE, radius: TILE_RADIUS, mark: MARK };
	const nameX = TILE + NAME_GAP;
	const rightNameX = width - nameX;
	const agentHeights = railHeights({
		count: tunnel.agents.length,
		top,
		bottom,
		tile: TILE
	});
	const connectionHeights = railHeights({
		count: tunnel.connections.length,
		top,
		bottom,
		tile: TILE
	});
	const agents = tunnel.agents.map((node, index) => {
		const room = hub.x - CURVE_ROOM - nameX - LINE_GAP - COUNT_GAP - countLetters(node);
		return row({
			node,
			y: agentHeights[index],
			tileX: 0,
			tile,
			nameX,
			anchor: 'start',
			room,
			hub: hubLeft,
			direction: 1
		});
	});
	const connections = tunnel.connections.map((node, index) => {
		const room =
			rightNameX - (hub.x + hub.width + CURVE_ROOM) - LINE_GAP - COUNT_GAP - countLetters(node);
		return row({
			node,
			y: connectionHeights[index],
			tileX: width - TILE,
			tile,
			nameX: rightNameX,
			anchor: 'end',
			room,
			hub: hubRight,
			direction: -1
		});
	});

	return {
		width,
		height: cardHeight,
		vertical: false,
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
		active: activePath(tunnel, agents, connections, 'x', 'x'),
		hub
	};
}

/** The tunnel on a phone: the agents above Relo, the connections below it, and Relo the
 *  spine both rails run into. */
function phoneGeometry(tunnel: RouteTunnel, width: number): TunnelGeometry {
	const tile = { size: PHONE_TILE, radius: PHONE_TILE_RADIUS, mark: PHONE_MARK };
	const nameX = PHONE_TILE + NAME_GAP;
	const hubWidth = Math.min(HUB_MAX, Math.max(HUB_MIN, width * 0.14));
	const hubX = width - hubWidth - EDGE;
	const spine = hubX + hubWidth / 2;
	const flatX = spine - PHONE_CURVE_ROOM;
	const agentTop = EDGE;
	const agentBand = Math.max(0, tunnel.agents.length - 1) * PHONE_PITCH + PHONE_TILE;
	const agentsBottom = agentTop + agentBand;
	const hubTop = agentsBottom + 18;
	const hub = {
		x: hubX,
		y: hubTop,
		width: hubWidth,
		height: HUB_HEIGHT
	};
	const hubTopPoint = { x: spine, y: hubTop };
	const hubBottomPoint = { x: spine, y: hubTop + HUB_HEIGHT };
	const connectionTop = hubTop + HUB_HEIGHT + 18;
	const connectionBand = Math.max(0, tunnel.connections.length - 1) * PHONE_PITCH + PHONE_TILE;
	const height = connectionTop + connectionBand + EDGE;

	const agentHeights = railHeights({
		count: tunnel.agents.length,
		top: agentTop,
		bottom: agentsBottom,
		tile: PHONE_TILE
	});
	const connectionHeights = railHeights({
		count: tunnel.connections.length,
		top: connectionTop,
		bottom: connectionTop + connectionBand,
		tile: PHONE_TILE
	});
	const agents = tunnel.agents.map((node, index) => {
		const room = flatX - nameX - LINE_GAP - COUNT_GAP - countLetters(node);
		return row({
			node,
			y: agentHeights[index],
			tileX: 0,
			tile,
			nameX,
			anchor: 'start',
			room,
			hub: hubTopPoint,
			direction: 1,
			flatX
		});
	});
	const connections = tunnel.connections.map((node, index) => {
		const room = flatX - nameX - LINE_GAP - COUNT_GAP - countLetters(node);
		return row({
			node,
			y: connectionHeights[index],
			tileX: 0,
			tile,
			nameX,
			anchor: 'start',
			room,
			hub: hubBottomPoint,
			direction: 1,
			flatX
		});
	});

	return {
		width,
		height,
		vertical: true,
		agents,
		connections,
		legs: [
			...agents.map((placement) => ({
				id: 'agent:' + placement.node.id,
				path: legPath(placement.stroke, 'y'),
				count: placement.node.count,
				width: legWidth(placement.node.count)
			})),
			...connections.map((placement) => ({
				id: 'connection:' + placement.node.id,
				path: legFromHub(placement.stroke, 'y'),
				count: placement.node.count,
				width: legWidth(placement.node.count)
			}))
		],
		active: activePath(tunnel, agents, connections, 'y', 'y'),
		hub
	};
}

/** The line the newest request took, from its agent through Relo to its connection. */
function activePath(
	tunnel: RouteTunnel,
	agents: TunnelPlacement[],
	connections: TunnelPlacement[],
	arrive: 'x' | 'y',
	depart: 'x' | 'y'
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
		path: routePath(agent.stroke, connection.stroke, arrive, depart),
		agent: active.agent,
		connection: active.connection
	};
}

/** The room the tunnel is drawn in: the width the card gives it and the height it may
 *  fill. */
export interface TunnelRoom {
	/** The width of the card's tunnel box. */
	width: number;
	/** The height of the card's tunnel box. */
	height: number;
	/** Whether the card is too narrow for two rails. */
	vertical: boolean;
}

/** The tunnel as it is drawn for the room it has. A phone draws it upright, where the
 *  rails have room to name their nodes. */
export function tunnelGeometry(tunnel: RouteTunnel, room: TunnelRoom): TunnelGeometry {
	const width = Math.max(room.width, 240);
	if (room.vertical) return phoneGeometry(tunnel, width);
	return wideGeometry(tunnel, width, room.height);
}
