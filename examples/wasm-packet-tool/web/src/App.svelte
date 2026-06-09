<script lang="ts">
  import { onMount } from "svelte";
  import { loadWasm, type MeshcoreWasm } from "./lib/wasm";
  import EncodePanel from "./lib/EncodePanel.svelte";
  import DecodePanel from "./lib/DecodePanel.svelte";

  let mc = $state<MeshcoreWasm | null>(null);
  let loadError = $state("");
  let activeTab = $state<"encode" | "decode">("encode");

  onMount(async () => {
    try {
      mc = await loadWasm();
    } catch (e) {
      loadError = String(e);
    }
  });
</script>

<div class="app">
  <header>
    <div class="title-row">
      <h1>MeshCore Packet Tool</h1>
      {#if mc}
        <span class="badge">WASM ready</span>
      {/if}
    </div>
    <p class="subtitle">Encode and decode MeshCore radio packet wire format in your browser</p>
  </header>

  {#if loadError}
    <div class="fatal">
      <strong>Failed to load WASM module:</strong> {loadError}
      <br />Run <code>make wasm</code> in the example directory first, then refresh.
    </div>
  {:else}
    <nav class="top-tabs">
      <button class:active={activeTab === "encode"} onclick={() => (activeTab = "encode")}>
        ⬆ Encode
      </button>
      <button class:active={activeTab === "decode"} onclick={() => (activeTab = "decode")}>
        ⬇ Decode
      </button>
    </nav>

    <div class="panel-wrap">
      {#if !mc}
        <div class="loading">Loading WASM module…</div>
      {:else if activeTab === "encode"}
        <EncodePanel {mc} />
      {:else}
        <DecodePanel {mc} />
      {/if}
    </div>
  {/if}
</div>

<style>
  :global(*, *::before, *::after) { box-sizing: border-box; }
  :global(body) {
    margin: 0;
    background: #0d1117;
    color: #e6edf3;
    font-family: "Segoe UI", system-ui, sans-serif;
    font-size: 14px;
    min-height: 100vh;
  }
  .app {
    max-width: 860px;
    margin: 0 auto;
    padding: 28px 16px 48px;
  }
  header { margin-bottom: 20px; }
  .title-row {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 4px;
  }
  h1 {
    margin: 0;
    font-size: 20px;
    font-weight: 600;
    color: #f0f6fc;
  }
  .badge {
    font-size: 11px;
    font-weight: 500;
    background: #1a3a2a;
    color: #3fb950;
    border: 1px solid #238636;
    border-radius: 20px;
    padding: 2px 8px;
  }
  .subtitle {
    margin: 0;
    color: #8b949e;
    font-size: 13px;
  }
  .top-tabs {
    display: flex;
    gap: 0;
    border-bottom: 1px solid #30363d;
    margin-bottom: 20px;
  }
  .top-tabs button {
    background: none;
    border: none;
    border-bottom: 2px solid transparent;
    color: #8b949e;
    cursor: pointer;
    font-size: 14px;
    font-weight: 500;
    padding: 10px 20px;
    margin-bottom: -1px;
    transition: color 0.15s, border-color 0.15s;
  }
  .top-tabs button:hover { color: #e6edf3; }
  .top-tabs button.active {
    color: #f0f6fc;
    border-bottom-color: #1f6feb;
  }
  .panel-wrap { width: 100%; }
  .loading {
    color: #8b949e;
    padding: 48px;
    text-align: center;
  }
  .fatal {
    background: #3d1f1f;
    border: 1px solid #6e2a2a;
    border-radius: 6px;
    padding: 16px;
    color: #f97583;
    line-height: 1.6;
  }
  .fatal code {
    background: #1c1c1c;
    padding: 2px 6px;
    border-radius: 4px;
    font-family: monospace;
  }
</style>
