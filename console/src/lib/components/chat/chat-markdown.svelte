<script lang="ts">
	import type { Block, Inline, TableAlign } from '$lib/chat-markdown';
	import ChatCodeBlock from './chat-code-block.svelte';

	// ChatMarkdown renders a parsed reply. Every node becomes an element here,
	// which is what keeps a reply from carrying markup of its own into the page.
	interface Props {
		blocks: Block[];
	}

	let { blocks }: Props = $props();

	// A heading inside a reply sits under the answer's own heading, so the
	// levels start at h4 and step down to h6 as the marks deepen.
	const HEADING_CLASS = [
		'text-lg font-semibold tracking-tight',
		'text-lg font-semibold tracking-tight',
		'text-base font-semibold tracking-tight',
		'text-base font-semibold tracking-tight',
		'text-sm font-semibold',
		'text-sm font-semibold'
	];

	function headingClass(level: number): string {
		return HEADING_CLASS[Math.min(Math.max(level, 1), 6) - 1];
	}

	function alignClass(align: TableAlign): string {
		if (align === 'center') return 'text-center';
		if (align === 'right') return 'text-right';
		return 'text-left';
	}
</script>

{#snippet inlineNodes(nodes: Inline[])}
	<!-- The parser numbers nothing, so a node is its own identity: the tree it returned is the
	     same tree for the life of the render, and a new reply replaces it whole. -->
	{#each nodes as node (node)}
		{#if node.kind === 'text'}{node.text}
		{:else if node.kind === 'break'}<br />
		{:else if node.kind === 'code'}
			<code class="rounded-sm bg-sunken px-1.5 py-0.5 font-mono text-[0.85em]">{node.text}</code>
		{:else if node.kind === 'strong'}
			<strong class="font-semibold">{@render inlineNodes(node.children)}</strong>
		{:else if node.kind === 'em'}
			<em>{@render inlineNodes(node.children)}</em>
		{:else if node.kind === 'del'}
			<del class="text-muted-foreground">{@render inlineNodes(node.children)}</del>
		{:else if node.kind === 'link'}
			<a
				href={node.href}
				target="_blank"
				rel="external noopener noreferrer"
				class="text-accent-deep underline underline-offset-2 hover:text-accent-strong"
				>{@render inlineNodes(node.children)}</a
			>
		{/if}
	{/each}
{/snippet}

{#snippet blockNodes(list: Block[])}
	<!-- No id on a block either: see the note on the inline tree above. -->
	{#each list as node (node)}
		{#if node.kind === 'paragraph'}
			<p>{@render inlineNodes(node.children)}</p>
		{:else if node.kind === 'heading'}
			{#if node.level <= 2}
				<h4 class={headingClass(node.level)}>{@render inlineNodes(node.children)}</h4>
			{:else if node.level <= 4}
				<h5 class={headingClass(node.level)}>{@render inlineNodes(node.children)}</h5>
			{:else}
				<h6 class={headingClass(node.level)}>{@render inlineNodes(node.children)}</h6>
			{/if}
		{:else if node.kind === 'code'}
			<ChatCodeBlock code={node.text} language={node.language} />
		{:else if node.kind === 'list'}
			{#if node.ordered}
				<ol start={node.start} class="list-decimal space-y-1.5 pl-6 marker:text-muted-foreground">
					{#each node.items as item (item)}
						<li class="space-y-2">{@render blockNodes(item)}</li>
					{/each}
				</ol>
			{:else}
				<ul class="list-disc space-y-1.5 pl-6 marker:text-muted-foreground">
					{#each node.items as item (item)}
						<li class="space-y-2">{@render blockNodes(item)}</li>
					{/each}
				</ul>
			{/if}
		{:else if node.kind === 'quote'}
			<blockquote class="space-y-3 rounded-panel bg-sunken px-4 py-3 text-muted-foreground">
				{@render blockNodes(node.children)}
			</blockquote>
		{:else if node.kind === 'divider'}
			<hr class="h-px border-0 bg-line" />
		{:else if node.kind === 'table'}
			<div class="overflow-x-auto rounded-panel border border-line">
				<table class="w-full border-collapse text-sm">
					<thead class="bg-sunken">
						<tr>
							{#each node.header as cell, cellIndex (cell)}
								<th
									scope="col"
									class="px-3 py-2 font-medium text-ink {alignClass(node.align[cellIndex] ?? null)}"
									>{@render inlineNodes(cell)}</th
								>
							{/each}
						</tr>
					</thead>
					<tbody>
						{#each node.rows as row (row)}
							<tr class="even:bg-sunken">
								{#each row as cell, cellIndex (cell)}
									<td class="px-3 py-2 align-top {alignClass(node.align[cellIndex] ?? null)}">
										{@render inlineNodes(cell)}
									</td>
								{/each}
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	{/each}
{/snippet}

<div class="space-y-4 text-[15px]">
	{@render blockNodes(blocks)}
</div>
