/** Nokia-style snake over the activity grid. Cells are indexed col * SNAKE_ROWS + row. The snake
 *  heads for the nearest uneaten active day by breadth-first search around its own body, and
 *  prefers the roomiest move when a meal would trap it. Greedy, not optimal: a full tour would need
 *  a Hamiltonian-path planner. */
export const SNAKE_ROWS = 7;
const MAX_LENGTH = 24;

/** How a dumb snake chases: it wanders, lunges at the nearest food without minding its body,
 *  runs for the farthest food, or sweeps column by column. */
export const DUMB_STRATEGIES = ['wander', 'lunge', 'farthest', 'sweep'] as const;
export type DumbStrategy = (typeof DUMB_STRATEGIES)[number];

export interface SnakeGame {
	cols: number;
	/** Playable cells: a day not yet reached by today is a wall. */
	open: boolean[];
	food: Set<number>;
	eaten: Set<number>;
	/** Head first. */
	body: number[];
	grow: number;
	done: boolean;
	/** A dumb snake wanders and may crash into its own body. */
	smart: boolean;
	strategy: DumbStrategy;
	crashed: boolean;
	rng: () => number;
}

export function newSnake(
	open: boolean[],
	food: Set<number>,
	cols: number,
	smart = true,
	rng: () => number = Math.random,
	strategy: DumbStrategy = DUMB_STRATEGIES[Math.floor(rng() * DUMB_STRATEGIES.length)]
): SnakeGame {
	const start = open.findIndex(Boolean);
	return {
		cols,
		open,
		food,
		eaten: new Set(),
		body: start < 0 ? [] : [start],
		grow: 2,
		done: start < 0 || food.size === 0,
		smart,
		strategy,
		crashed: false,
		rng
	};
}

function neighbors(game: SnakeGame, cell: number): number[] {
	const col = Math.floor(cell / SNAKE_ROWS);
	const row = cell % SNAKE_ROWS;
	const out: number[] = [];
	if (row > 0) out.push(cell - 1);
	if (row < SNAKE_ROWS - 1) out.push(cell + 1);
	if (col > 0) out.push(cell - SNAKE_ROWS);
	if (col < game.cols - 1) out.push(cell + SNAKE_ROWS);
	return out.filter((n) => game.open[n]);
}

/** Cells the head may not enter this step; the tail frees up unless the snake is growing. */
function blockedCells(game: SnakeGame): Set<number> {
	const blocked = new Set(game.body);
	if (game.grow === 0) blocked.delete(game.body[game.body.length - 1]);
	return blocked;
}

/** The cells reachable from one cell, capped so a roomy board costs little. */
function room(game: SnakeGame, from: number, blocked: Set<number>, cap: number): number {
	const seen = new Set([from]);
	const queue = [from];
	for (let i = 0; i < queue.length && seen.size < cap; i += 1) {
		for (const n of neighbors(game, queue[i])) {
			if (!seen.has(n) && !blocked.has(n)) {
				seen.add(n);
				queue.push(n);
			}
		}
	}
	return seen.size;
}

/** The first step of the shortest route to the nearest uneaten day, or -1. */
function towardFood(game: SnakeGame, blocked: Set<number>): number {
	const head = game.body[0];
	const first = new Map<number, number>();
	const queue: number[] = [];
	for (const n of neighbors(game, head)) {
		if (!blocked.has(n)) {
			first.set(n, n);
			queue.push(n);
		}
	}
	for (let i = 0; i < queue.length; i += 1) {
		const cell = queue[i];
		if (game.food.has(cell) && !game.eaten.has(cell)) return first.get(cell) as number;
		for (const n of neighbors(game, cell)) {
			if (!first.has(n) && !blocked.has(n)) {
				first.set(n, first.get(cell) as number);
				queue.push(n);
			}
		}
	}
	return -1;
}

function chooseMove(game: SnakeGame): number {
	const blocked = blockedCells(game);
	const need = game.body.length + 1;
	const greedy = towardFood(game, blocked);
	if (greedy >= 0 && room(game, greedy, new Set([...blocked, game.body[0]]), need) >= need) {
		return greedy;
	}
	let best = -1;
	let bestRoom = -1;
	for (const n of neighbors(game, game.body[0])) {
		if (blocked.has(n)) continue;
		const space = room(game, n, new Set([...blocked, game.body[0]]), game.open.length);
		if (space > bestRoom) {
			best = n;
			bestRoom = space;
		}
	}
	return best;
}

/** The first step of the shortest route to the farthest uneaten day, ignoring the body. */
function towardFarthest(game: SnakeGame): number {
	const head = game.body[0];
	const first = new Map<number, number>();
	const queue: number[] = [];
	for (const n of neighbors(game, head)) {
		first.set(n, n);
		queue.push(n);
	}
	let target = -1;
	for (let i = 0; i < queue.length; i += 1) {
		const cell = queue[i];
		if (game.food.has(cell) && !game.eaten.has(cell)) target = cell;
		for (const n of neighbors(game, cell)) {
			if (!first.has(n) && n !== head) {
				first.set(n, first.get(cell) as number);
				queue.push(n);
			}
		}
	}
	return target < 0 ? -1 : (first.get(target) as number);
}

/** The next cell of a column-by-column sweep, or -1 past its end. */
function sweepMove(game: SnakeGame): number {
	const head = game.body[0];
	const row = head % SNAKE_ROWS;
	const down = Math.floor(head / SNAKE_ROWS) % 2 === 0;
	const edge = down ? row === SNAKE_ROWS - 1 : row === 0;
	const next = edge ? head + SNAKE_ROWS : down ? head + 1 : head - 1;
	return game.open[next] ? next : -1;
}

/** A dumb move by the game's strategy. None of them checks the body, so it can bite itself. */
function dumbMove(game: SnakeGame): number {
	if (game.strategy === 'sweep') return sweepMove(game);
	const options = neighbors(game, game.body[0]).filter((n) => n !== game.body[1]);
	if (options.length === 0) return -1;
	let aim = -1;
	if (game.strategy === 'lunge') aim = towardFood(game, new Set());
	else if (game.strategy === 'farthest') aim = towardFarthest(game);
	else if (game.rng() < 0.5) aim = towardFood(game, new Set());
	if (aim >= 0 && options.includes(aim)) return aim;
	return options[Math.floor(game.rng() * options.length)];
}

/** Advances the snake one cell; marks the game done when it has eaten all or is boxed in. */
export function stepSnake(game: SnakeGame): void {
	if (game.done) return;
	const next = game.smart ? chooseMove(game) : dumbMove(game);
	if (next < 0) {
		game.done = true;
		return;
	}
	if (blockedCells(game).has(next)) {
		game.crashed = true;
		game.done = true;
		return;
	}
	game.body.unshift(next);
	if (game.food.has(next) && !game.eaten.has(next)) {
		game.eaten.add(next);
		if (game.body.length + game.grow < MAX_LENGTH) game.grow += 1;
	}
	if (game.grow > 0) game.grow -= 1;
	else game.body.pop();
	if (game.eaten.size >= game.food.size) game.done = true;
}
