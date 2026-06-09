<script lang="ts">
  import { isError, type MeshcoreWasm, RouteTypes, PayloadTypes } from "./wasm";

  let { mc }: { mc: MeshcoreWasm } = $props();

  type PacketTab = "grptxt" | "txtmsg" | "raw";
  let tab = $state<PacketTab>("grptxt");

  // GRP_TXT fields
  type GrpKeyMode = "name" | "secret";
  let g_keyMode = $state<GrpKeyMode>("name");
  let g_channel = $state("#test");
  let g_secret  = $state("");
  let g_sender  = $state("");
  let g_text    = $state("");

  // TXT_MSG fields
  let t_priv    = $state("");
  let t_myPub   = $state("");
  let t_peerPub = $state("");
  let t_text    = $state("");

  // RAW fields
  let r_route       = $state(1);  // FLOOD
  let r_type        = $state(5);  // GRP_TXT
  let r_version     = $state(0);
  let r_pathHash    = $state(2);
  let r_payloadHex  = $state("");

  // Output
  let result      = $state("");
  let resultError = $state("");
  let copied      = $state(false);

  function encode() {
    result = "";
    resultError = "";
    copied = false;

    if (tab === "grptxt") {
      if (!g_sender || !g_text) { resultError = "Fill in sender and message."; return; }
      let r;
      if (g_keyMode === "secret") {
        if (!g_secret) { resultError = "Enter the channel secret (hex)."; return; }
        r = mc.encodeGroupTextSecret(g_secret, g_sender, g_text);
      } else {
        if (!g_channel) { resultError = "Enter a channel name."; return; }
        r = mc.encodeGroupText(g_channel, g_sender, g_text);
      }
      if (isError(r)) { resultError = r.error; } else { result = r.hex; }

    } else if (tab === "txtmsg") {
      if (!t_priv || !t_peerPub || !t_text) { resultError = "Fill in your private key, peer public key and message."; return; }
      const r = mc.encodeDirectText(t_priv, t_peerPub, t_text);
      if (isError(r)) { resultError = r.error; } else { result = r.hex; }

    } else {
      const r = mc.encodeRaw(r_route, r_type, r_version, r_pathHash, r_payloadHex);
      if (isError(r)) { resultError = r.error; } else { result = r.hex; }
    }
  }

  function generateKeypair() {
    const kp = mc.generateKeypair();
    t_priv = kp.privateKey;
    t_myPub = kp.publicKey;
  }

  async function copy() {
    if (!result) return;
    await navigator.clipboard.writeText(result);
    copied = true;
    setTimeout(() => (copied = false), 1500);
  }
</script>

<div class="panel">
  <!-- Packet type tabs -->
  <div class="type-tabs">
    <button class:active={tab === "grptxt"} onclick={() => (tab = "grptxt")}>
      GRP_TXT
      <span class="sub">channel message</span>
    </button>
    <button class:active={tab === "txtmsg"} onclick={() => (tab = "txtmsg")}>
      TXT_MSG
      <span class="sub">direct message</span>
    </button>
    <button class:active={tab === "raw"} onclick={() => (tab = "raw")}>
      RAW
      <span class="sub">any type</span>
    </button>
  </div>

  <!-- ── GRP_TXT ── -->
  {#if tab === "grptxt"}
    <div class="fields">
      <div class="mode-toggle">
        <button class:active={g_keyMode === "name"}   onclick={() => (g_keyMode = "name")}>Channel name</button>
        <button class:active={g_keyMode === "secret"} onclick={() => (g_keyMode = "secret")}>Channel secret</button>
      </div>
      {#if g_keyMode === "name"}
        <label>
          <span class="lbl">Channel name</span>
          <input bind:value={g_channel} placeholder="#test" />
        </label>
      {:else}
        <label>
          <span class="lbl">Channel secret (hex, 16 bytes)</span>
          <input bind:value={g_secret} placeholder="32 hex chars" class="mono" />
        </label>
      {/if}
      <label>
        <span class="lbl">Sender name</span>
        <input bind:value={g_sender} placeholder="Alice" />
      </label>
      <label>
        <span class="lbl">Message</span>
        <textarea bind:value={g_text} placeholder="Hello mesh!" rows={3}></textarea>
      </label>
    </div>

  <!-- ── TXT_MSG ── -->
  {:else if tab === "txtmsg"}
    <div class="fields">
      <label>
        <span class="lbl">My private key</span>
        <div class="key-row">
          <input bind:value={t_priv} placeholder="64 hex chars" class="mono" />
          <button class="sm" onclick={generateKeypair}>Generate</button>
        </div>
      </label>
      {#if t_myPub}
        <label>
          <span class="lbl">My public key (derived)</span>
          <input value={t_myPub} readonly class="mono dim" />
        </label>
      {/if}
      <label>
        <span class="lbl">Peer public key</span>
        <input bind:value={t_peerPub} placeholder="64 hex chars" class="mono" />
      </label>
      <label>
        <span class="lbl">Message</span>
        <textarea bind:value={t_text} placeholder="Hello!" rows={3}></textarea>
      </label>
    </div>

  <!-- ── RAW ── -->
  {:else}
    <div class="fields">
      <div class="two-col">
        <label>
          <span class="lbl">Route type</span>
          <select bind:value={r_route}>
            {#each RouteTypes as rt}
              <option value={rt.code}>{rt.label}</option>
            {/each}
          </select>
        </label>
        <label>
          <span class="lbl">Payload type</span>
          <select bind:value={r_type}>
            {#each PayloadTypes as pt}
              <option value={pt.code}>{pt.label} (0x{pt.code.toString(16).padStart(2,"0")})</option>
            {/each}
          </select>
        </label>
        <label>
          <span class="lbl">Version (0–3)</span>
          <input type="number" bind:value={r_version} min={0} max={3} />
        </label>
        <label>
          <span class="lbl">Path hash size (bytes)</span>
          <select bind:value={r_pathHash}>
            <option value={1}>1</option>
            <option value={2}>2</option>
            <option value={3}>3</option>
            <option value={4}>4</option>
          </select>
        </label>
      </div>
      <label>
        <span class="lbl">Payload (hex)</span>
        <textarea bind:value={r_payloadHex} placeholder="Raw payload bytes as hex (may be empty)" rows={3} class="mono"></textarea>
      </label>
    </div>
  {/if}

  <button class="primary" onclick={encode}>Encode</button>

  {#if resultError}
    <div class="error">{resultError}</div>
  {/if}

  {#if result}
    <div class="output-hdr">
      Encoded hex
      <button class="copy-btn" onclick={copy}>{copied ? "✓ Copied" : "Copy"}</button>
    </div>
    <div class="hex-out">{result}</div>
    <div class="byte-count">{result.length / 2} bytes</div>
  {/if}
</div>

<style>
  .panel {
    background: #161b22;
    border: 1px solid #30363d;
    border-radius: 8px;
    padding: 20px;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
  .type-tabs {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
  }
  .type-tabs button {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 2px;
    background: #0d1117;
    border: 1px solid #30363d;
    border-radius: 6px;
    color: #8b949e;
    cursor: pointer;
    font-size: 13px;
    font-weight: 600;
    padding: 8px 14px;
    transition: background 0.1s, color 0.1s, border-color 0.1s;
    min-width: 110px;
  }
  .type-tabs button:hover { background: #21262d; color: #e6edf3; }
  .type-tabs button.active {
    background: #21262d;
    border-color: #1f6feb;
    color: #79c0ff;
  }
  .sub {
    font-size: 10px;
    font-weight: 400;
    color: #6e7681;
  }
  .type-tabs button.active .sub { color: #8b949e; }

  .fields { display: flex; flex-direction: column; gap: 10px; }
  .mode-toggle { display: flex; gap: 0; border: 1px solid #30363d; border-radius: 6px; overflow: hidden; width: fit-content; }
  .mode-toggle button { background: #0d1117; border: none; border-radius: 0; color: #8b949e; font-size: 12px; padding: 5px 12px; }
  .mode-toggle button:hover { background: #21262d; color: #e6edf3; }
  .mode-toggle button.active { background: #21262d; color: #79c0ff; font-weight: 600; }
  .mode-toggle button:first-child { border-right: 1px solid #30363d; }
  .two-col {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 10px;
  }
  label { display: flex; flex-direction: column; gap: 4px; }
  .lbl {
    font-size: 11px;
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: #8b949e;
  }
  input, textarea, select {
    background: #0d1117;
    border: 1px solid #30363d;
    border-radius: 6px;
    color: #e6edf3;
    font-size: 13px;
    padding: 8px 10px;
    outline: none;
    resize: vertical;
    width: 100%;
    font-family: inherit;
  }
  input:focus, textarea:focus, select:focus { border-color: #58a6ff; }
  .mono { font-family: "Cascadia Code", "Fira Code", monospace; font-size: 12px; }
  .dim { color: #8b949e; }
  .key-row { display: flex; gap: 6px; }
  .key-row input { flex: 1; }

  button { background: #21262d; border: 1px solid #30363d; border-radius: 6px; color: #e6edf3; cursor: pointer; font-size: 13px; padding: 8px 14px; transition: background 0.1s; font-family: inherit; }
  button:hover { background: #30363d; }
  button.primary { background: #1f6feb; border-color: #1f6feb; color: #fff; font-weight: 600; }
  button.primary:hover { background: #388bfd; }
  button.sm { padding: 8px 10px; font-size: 12px; white-space: nowrap; }

  .error { background: #3d1f1f; border: 1px solid #6e2a2a; border-radius: 6px; color: #f97583; font-size: 12px; padding: 8px 10px; }

  .output-hdr { display: flex; align-items: center; justify-content: space-between; font-size: 11px; font-weight: 500; text-transform: uppercase; letter-spacing: 0.06em; color: #8b949e; }
  .copy-btn { padding: 3px 8px; font-size: 11px; }
  .hex-out { background: #0d1117; border: 1px solid #30363d; border-radius: 6px; color: #79c0ff; font-family: "Cascadia Code", "Fira Code", monospace; font-size: 12px; padding: 10px; word-break: break-all; line-height: 1.7; }
  .byte-count { font-size: 11px; color: #6e7681; text-align: right; margin-top: -8px; }
</style>
