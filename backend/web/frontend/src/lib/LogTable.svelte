<script>
	// Reusable log view: a card with a header (title, live count, optional extra
	// controls, Pause/Clear) and a fixed-layout table. Each consumer supplies the
	// column headers and a `row` snippet that renders the <td>s for one item.
	// Ingestion stays with the parent; this component only displays. New log views
	// (backend logs, mesh packets, …) can reuse it by passing columns + a row.
	let {
		title,
		rows = [],
		columns = [],
		row,
		controls,
		description = '',
		paused = $bindable(false),
		onclear,
		empty = 'Waiting for entries…'
	} = $props();
</script>

<div class="card wide">
	<div class="hdr">
		<h2>{title} <span class="muted">({rows.length})</span></h2>
		<div class="ctrls">
			{#if controls}{@render controls()}{/if}
			<button onclick={() => (paused = !paused)}>{paused ? 'Resume' : 'Pause'}</button>
			<button onclick={() => onclear?.()}>Clear</button>
		</div>
	</div>
	{#if description}<p class="muted desc">{description}</p>{/if}
	<table class="log-table">
		<thead>
			<tr>
				{#each columns as c}<th style={c.width ? `width:${c.width}` : ''}>{c.label}</th>{/each}
			</tr>
		</thead>
		<tbody>
			{#each rows as item (item._id)}
				<tr>{@render row(item)}</tr>
			{:else}
				<tr><td class="muted" colspan={columns.length}>{paused ? 'Paused.' : empty}</td></tr>
			{/each}
		</tbody>
	</table>
</div>

<style>
	.card {
		background: var(--panel);
		border: 1px solid var(--border);
		border-radius: 10px;
		padding: 1rem;
		grid-column: 1 / -1;
	}
	.hdr {
		display: flex;
		gap: 0.5rem;
		align-items: center;
		justify-content: space-between;
		flex-wrap: wrap;
	}
	h2 {
		font-size: 0.95rem;
		margin: 0;
	}
	.ctrls {
		display: flex;
		gap: 0.5rem;
		align-items: center;
		flex-wrap: wrap;
	}
	.muted {
		color: var(--muted);
	}
	.desc {
		margin: 0.5rem 0 0.75rem;
		font-size: 0.85rem;
	}
	button {
		background: var(--bg);
		border: 1px solid var(--border);
		color: var(--text);
		padding: 0.4rem 0.8rem;
		border-radius: 6px;
		cursor: pointer;
	}
	.log-table {
		width: 100%;
		border-collapse: collapse;
		table-layout: fixed;
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 0.78rem;
	}
	/* Cells come from the parent-supplied row snippet, so target them globally. */
	.log-table :global(th),
	.log-table :global(td) {
		text-align: left;
		padding: 0.4rem 0.5rem;
		border-bottom: 1px solid var(--border);
		vertical-align: top;
		word-break: break-all;
	}
	.log-table :global(th) {
		color: var(--muted);
		font-weight: 600;
	}
</style>
