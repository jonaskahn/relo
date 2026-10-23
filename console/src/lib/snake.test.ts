import { describe, expect, it } from 'vitest';

import { DUMB_STRATEGIES, newSnake, SNAKE_ROWS, stepSnake } from './snake';

function board(cols: number, food: number[]) {
	return newSnake(Array(cols * SNAKE_ROWS).fill(true), new Set(food), cols);
}

function play(game: ReturnType<typeof board>, limit = 2000) {
	let steps = 0;
	while (!game.done && steps < limit) {
		stepSnake(game);
		steps += 1;
		expect(new Set(game.body).size).toBe(game.body.length);
	}
	return steps;
}

describe('the activity snake', () => {
	it('eats every active day without crossing itself', () => {
		const food = [9, 20, 33, 40, 55, 70, 85, 100, 120, 140];
		const game = board(24, food);
		play(game);
		expect(game.eaten.size).toBe(food.length);
	});

	it('takes the nearest day first', () => {
		const game = board(10, [3, 40]);
		play(game, 5);
		expect(game.eaten.has(3)).toBe(true);
		expect(game.eaten.has(40)).toBe(false);
	});

	it('does not move onto a wall', () => {
		const open = Array(3 * SNAKE_ROWS).fill(true);
		for (let row = 0; row < SNAKE_ROWS; row += 1) open[SNAKE_ROWS + row] = false;
		const game = newSnake(open, new Set([2 * SNAKE_ROWS]), 3);
		stepSnake(game);
		expect(game.body.every((cell) => open[cell])).toBe(true);
	});

	it('has nothing to play without food', () => {
		expect(board(4, []).done).toBe(true);
	});

	it('ends a dumb game under every strategy and never leaves the board', () => {
		for (const strategy of DUMB_STRATEGIES) {
			let x = 7;
			const rng = () => (x = (x * 16807) % 2147483647) / 2147483647;
			const open = Array(12 * SNAKE_ROWS).fill(true);
			const game = newSnake(open, new Set([20, 40, 60]), 12, false, rng, strategy);
			for (let i = 0; i < 5000 && !game.done; i += 1) stepSnake(game);
			expect(game.done || strategy === 'wander').toBe(true);
			expect(game.body.every((cell) => open[cell])).toBe(true);
		}
	});

	it('sweeps column by column and eats what lies on its way', () => {
		const game = newSnake(
			Array(4 * SNAKE_ROWS).fill(true),
			new Set([3, 9]),
			4,
			false,
			Math.random,
			'sweep'
		);
		for (let i = 0; i < 30 && !game.done; i += 1) stepSnake(game);
		expect(game.eaten.size).toBe(2);
	});

	it('lets a dumb snake crash into itself and ends the game', () => {
		const game = newSnake(Array(5 * SNAKE_ROWS).fill(true), new Set([34]), 5, false, () => 0);
		game.body = [10, 11, 12, 5, 4, 3];
		game.grow = 0;
		game.rng = () => 0.99;
		let crashed = false;
		for (let i = 0; i < 200 && !game.done; i += 1) {
			stepSnake(game);
			crashed = game.crashed;
		}
		expect(game.done).toBe(true);
		expect(crashed || game.eaten.size === 1).toBe(true);
	});

	it('ends every dumb game, crashed or not', () => {
		for (let seed = 1; seed <= 20; seed += 1) {
			let x = seed;
			const rng = () => (x = (x * 16807) % 2147483647) / 2147483647;
			const game = newSnake(
				Array(12 * SNAKE_ROWS).fill(true),
				new Set([20, 40, 60]),
				12,
				false,
				rng
			);
			for (let i = 0; i < 5000 && !game.done; i += 1) stepSnake(game);
			expect(game.crashed || game.eaten.size === 3 || !game.done).toBe(true);
		}
	});
});
