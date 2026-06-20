<script>
	let { children } = $props();
</script>

{@render children()}

<style>
	:global(:root) {
		--bg: #0d1117;
		--panel: #161b22;
		--border: #30363d;
		--text: #e6edf3;
		--muted: #8b949e;
		--accent: #2f81f7;
		--ok: #3fb950;
		--warn: #d29922;
		--err: #f85149;
		color-scheme: dark;
	}
	:global(body) {
		margin: 0;
		background: var(--bg);
		color: var(--text);
		font: 14px/1.5 ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, sans-serif;
	}
	:global(*) {
		box-sizing: border-box;
	}
	:global(button) {
		font: inherit;
	}
	:global(code) {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
	}
</style>
