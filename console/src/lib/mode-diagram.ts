// The mode diagram: how one request travels from a coding client to Relo, in each of the
// two ways Claude Code can be pointed at it. In environment mode the client's own settings
// carry Relo's address, so every request goes to Relo. In proxy mode the client keeps
// talking to Anthropic and Relo's intercept sits in the path, sending the models Relo
// serves to Relo and relaying everything else to Anthropic untouched.
//
// The model is pure, so the geometry can be tested without a canvas: the component draws
// what these functions decide and decides nothing itself.

export type DiagramMode = 'env' | 'proxy';
export type DiagramOrientation = 'row' | 'column';
export type NodeId = 'client' | 'intercept' | 'relo' | 'providers' | 'anthropic';

/** The text a node carries, already in the operator's language. */
export interface NodeLabel {
	name: string;
	sub: string;
}
export type DiagramLabels = Record<NodeId, NodeLabel>;

/** bound goes to Relo under Relo's key, native goes to Anthropic with the caller's own
 *  credential, plain is a single line that carries everything, off is a path the mode
 *  does not take. */
export type EdgeKind = 'bound' | 'native' | 'plain' | 'off';

export interface DiagramNode {
	id: NodeId;
	name: string;
	sub: string;
	x: number;
	y: number;
	width: number;
	height: number;
	/** A node the mode does not use stands dim, so the operator sees what is bypassed. */
	dim: boolean;
}

export interface DiagramEdge {
	/** Stable across modes, so a line two modes share morphs instead of being redrawn. */
	id: string;
	kind: EdgeKind;
	path: string;
	/** Whether traffic can run along it in this mode. */
	flows: boolean;
	/** Where the fork's branch names itself, at the middle of the line. */
	chip: { x: number; y: number } | null;
}

export interface Diagram {
	orientation: DiagramOrientation;
	width: number;
	height: number;
	nodes: DiagramNode[];
	edges: DiagramEdge[];
}

/** The room the diagram keeps from the canvas's edges. */
const EDGE = 12;
const NODE_HEIGHT = 46;
/** The least a node is wide, and the width of a letter of its 12px name and its 10px mono
 *  sub line, plus the air inside. */
const NODE_MIN = 84;
const NAME_LETTER = 7;
const SUB_LETTER = 6.2;
const NODE_PAD = 20;
/** The air between two steps of the path: the least a line needs to read as one. */
const STEP_GAP = 34;
/** The air between the two branches of the fork, in a row and in a column layout. */
const BRANCH_GAP_ROW = 40;
const BRANCH_GAP_COLUMN = 22;
const COLUMN_GAP = 24;

/** The step of the path a node stands on, and which side of the fork it takes. Every node
 *  keeps its place across modes, so only the lines move when the mode changes. */
const PLACES: Record<NodeId, { step: number; branch: number }> = {
	client: { step: 0, branch: 0.5 },
	intercept: { step: 1, branch: 0.5 },
	relo: { step: 2, branch: 0 },
	anthropic: { step: 2, branch: 1 },
	providers: { step: 3, branch: 0 }
};
const STEPS = 4;

const NODE_IDS = Object.keys(PLACES) as NodeId[];

/** The width a node needs for the words it carries. */
function widthOf(label: NodeLabel): number {
	const name = label.name.length * NAME_LETTER;
	const sub = label.sub.length * SUB_LETTER;
	return Math.max(NODE_MIN, Math.ceil(Math.max(name, sub) + NODE_PAD));
}

interface Frame {
	orientation: DiagramOrientation;
	width: number;
	height: number;
	/** The nodes' own width, one per node. */
	widths: Record<NodeId, number>;
	/** The left edge (row) or top edge (column) of each step's band. */
	stepAt: number[];
	branchGap: number;
	nodeWidth: number;
}

function frameOf(width: number, labels: DiagramLabels): Frame {
	const widths = Object.fromEntries(NODE_IDS.map((id) => [id, widthOf(labels[id])])) as Record<
		NodeId,
		number
	>;
	const bands = Array.from({ length: STEPS }, (_, step) =>
		Math.max(...NODE_IDS.filter((id) => PLACES[id].step === step).map((id) => widths[id]))
	);
	const needed = 2 * EDGE + bands.reduce((sum, band) => sum + band, 0) + (STEPS - 1) * STEP_GAP;
	if (width >= needed) {
		const gap = (width - 2 * EDGE - bands.reduce((sum, band) => sum + band, 0)) / (STEPS - 1);
		const stepAt: number[] = [];
		let at = EDGE;
		for (const band of bands) {
			stepAt.push(at);
			at += band + gap;
		}
		return {
			orientation: 'row',
			width,
			height: 2 * EDGE + 2 * NODE_HEIGHT + BRANCH_GAP_ROW,
			widths,
			stepAt,
			branchGap: BRANCH_GAP_ROW,
			nodeWidth: 0
		};
	}
	const nodeWidth = Math.max(...NODE_IDS.map((id) => widths[id]));
	const stepAt = Array.from(
		{ length: STEPS },
		(_, step) => EDGE + step * (NODE_HEIGHT + STEP_GAP * 0.75)
	);
	return {
		orientation: 'column',
		width: Math.max(width, 2 * EDGE + 2 * nodeWidth + COLUMN_GAP),
		height: EDGE * 2 + STEPS * NODE_HEIGHT + (STEPS - 1) * STEP_GAP * 0.75,
		widths,
		stepAt,
		branchGap: BRANCH_GAP_COLUMN,
		nodeWidth
	};
}

function nodeAt(id: NodeId, frame: Frame, label: NodeLabel, dim: boolean): DiagramNode {
	const place = PLACES[id];
	const width = frame.widths[id];
	if (frame.orientation === 'row') {
		const band = Math.max(
			...NODE_IDS.filter((other) => PLACES[other].step === place.step).map(
				(other) => frame.widths[other]
			)
		);
		return {
			id,
			name: label.name,
			sub: label.sub,
			x: frame.stepAt[place.step] + (band - width) / 2,
			y: EDGE + place.branch * (NODE_HEIGHT + frame.branchGap),
			width,
			height: NODE_HEIGHT,
			dim
		};
	}
	const across = 2 * frame.nodeWidth + COLUMN_GAP;
	const left = (frame.width - across) / 2;
	return {
		id,
		name: label.name,
		sub: label.sub,
		x: left + place.branch * (frame.nodeWidth + COLUMN_GAP) + (frame.nodeWidth - width) / 2,
		y: frame.stepAt[place.step],
		width,
		height: NODE_HEIGHT,
		dim
	};
}

type Point = { x: number; y: number };

/** A line leaves the side of a node that faces the way the path runs and enters the
 *  facing side of the next. */
function edgeBetween(
	orientation: DiagramOrientation,
	from: DiagramNode,
	to: DiagramNode
): { path: string; middle: Point } {
	let start: Point;
	let end: Point;
	let path: string;
	if (orientation === 'row') {
		start = { x: from.x + from.width, y: from.y + from.height / 2 };
		end = { x: to.x, y: to.y + to.height / 2 };
		const bend = (start.x + end.x) / 2;
		path = `M ${r(start.x)} ${r(start.y)} C ${r(bend)} ${r(start.y)} ${r(bend)} ${r(end.y)} ${r(end.x)} ${r(end.y)}`;
	} else {
		start = { x: from.x + from.width / 2, y: from.y + from.height };
		end = { x: to.x + to.width / 2, y: to.y };
		const bend = (start.y + end.y) / 2;
		path = `M ${r(start.x)} ${r(start.y)} C ${r(start.x)} ${r(bend)} ${r(end.x)} ${r(bend)} ${r(end.x)} ${r(end.y)}`;
	}
	return { path, middle: { x: (start.x + end.x) / 2, y: (start.y + end.y) / 2 } };
}

function r(value: number): number {
	return Math.round(value * 10) / 10;
}

/** The diagram for one mode on a canvas `width` wide. A canvas too narrow for the whole
 *  path in one row stacks it in a column instead. */
export function modeDiagram(input: {
	mode: DiagramMode;
	width: number;
	labels: DiagramLabels;
}): Diagram {
	const { mode, labels } = input;
	const frame = frameOf(input.width, labels);
	const all = Object.fromEntries(
		NODE_IDS.map((id) => [id, nodeAt(id, frame, labels[id], mode === 'env' && id === 'anthropic')])
	) as Record<NodeId, DiagramNode>;
	const edge = (
		id: string,
		kind: EdgeKind,
		from: NodeId,
		to: NodeId,
		options: { flows: boolean; chip?: boolean }
	): DiagramEdge => {
		const line = edgeBetween(frame.orientation, all[from], all[to]);
		return {
			id,
			kind,
			path: line.path,
			flows: options.flows,
			chip: options.chip ? line.middle : null
		};
	};
	const nodes: DiagramNode[] = [all.client, all.relo, all.providers, all.anthropic];
	const edges: DiagramEdge[] = [];
	if (mode === 'env') {
		edges.push(
			edge('in', 'plain', 'client', 'relo', { flows: true }),
			edge('out', 'plain', 'relo', 'providers', { flows: true }),
			edge('off', 'off', 'client', 'anthropic', { flows: false })
		);
	} else {
		nodes.splice(1, 0, all.intercept);
		edges.push(
			edge('in', 'plain', 'client', 'intercept', { flows: true }),
			edge('bound', 'bound', 'intercept', 'relo', { flows: true, chip: true }),
			edge('out', 'plain', 'relo', 'providers', { flows: true }),
			edge('native', 'native', 'intercept', 'anthropic', { flows: true, chip: true })
		);
	}
	return {
		orientation: frame.orientation,
		width: frame.width,
		height: frame.height,
		nodes,
		edges
	};
}
