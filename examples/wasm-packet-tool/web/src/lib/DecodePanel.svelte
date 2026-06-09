<script lang="ts">
  import {
    isError,
    fmtTimestamp,
    type MeshcoreWasm,
    type Envelope,
    type GroupTextPayload,
    type DirectTextPayload,
    type AdvertPayload,
  } from "./wasm";

  let { mc }: { mc: MeshcoreWasm } = $props();

  // ── Step 1: envelope ─────────────────────────────────────────────────────────
  let hexInput     = $state("");
  let envelope     = $state<Envelope | null>(null);
  let envelopeErr  = $state("");

  function parseEnvelope() {
    envelope = null;
    envelopeErr = "";
    clearPayload();
    const h = hexInput.trim().replace(/\s/g, "");
    if (!h) { envelopeErr = "Paste a hex packet first."; return; }
    const r = mc.decodeEnvelope(h);
    if (isError(r)) { envelopeErr = r.error; return; }
    envelope = r;
    // Auto-decode unencrypted types immediately
    if (envelope.type === "ADVERT") decodeAdvert();
  }

  // ── Step 2: payload decryption ────────────────────────────────────────────────
  type GrpKeyMode = "name" | "secret";
  let p_grpKeyMode = $state<GrpKeyMode>("name");
  let p_channel = $state("");
  let p_secret  = $state("");
  let p_priv    = $state("");
  let p_peerPub = $state("");

  type PayloadResult =
    | { kind: "grptxt"; data: GroupTextPayload }
    | { kind: "txtmsg"; data: DirectTextPayload }
    | { kind: "advert"; data: AdvertPayload };

  let payloadResult = $state<PayloadResult | null>(null);
  let payloadErr    = $state("");

  function clearPayload() {
    payloadResult = null;
    payloadErr = "";
  }

  function decodeAdvert() {
    if (!envelope) return;
    const r = mc.decodeAdvert(envelope.payloadHex);
    if (isError(r)) { payloadErr = r.error; }
    else { payloadResult = { kind: "advert", data: r }; }
  }

  function decodeGroupText() {
    if (!envelope) return;
    payloadErr = "";
    payloadResult = null;
    let r;
    if (p_grpKeyMode === "secret") {
      if (!p_secret) { payloadErr = "Enter the channel secret (hex)."; return; }
      r = mc.decodeGroupTextSecret(envelope.payloadHex, p_secret);
    } else {
      if (!p_channel) { payloadErr = "Enter a channel name."; return; }
      r = mc.decodeGroupText(envelope.payloadHex, p_channel);
    }
    if (isError(r)) { payloadErr = r.error; }
    else { payloadResult = { kind: "grptxt", data: r }; }
  }

  function decodeDirectText() {
    if (!envelope) return;
    if (!p_priv || !p_peerPub) { payloadErr = "Enter both private key and peer public key."; return; }
    payloadErr = "";
    payloadResult = null;
    const r = mc.decodeDirectText(envelope.payloadHex, p_priv, p_peerPub);
    if (isError(r)) { payloadErr = r.error; }
    else { payloadResult = { kind: "txtmsg", data: r }; }
  }

  function generateKeypair() {
    const kp = mc.generateKeypair();
    p_priv = kp.privateKey;
  }

  // Re-clear payload decode when envelope changes
  $effect(() => { if (envelope) clearPayload(); });
</script>

<div class="panel">

  <!-- ── Step 1 ─────────────────────────────────────────────────────────────── -->
  <section>
    <h3>Step 1 — parse envelope</h3>
    <label>
      <span class="lbl">Hex packet</span>
      <textarea bind:value={hexInput} placeholder="Paste hex-encoded MeshCore packet…" rows={4} class="mono"></textarea>
    </label>
    <button class="primary" onclick={parseEnvelope}>Parse</button>
    {#if envelopeErr}
      <div class="error">{envelopeErr}</div>
    {/if}
  </section>

  <!-- ── Envelope result ──────────────────────────────────────────────────────── -->
  {#if envelope}
    <div class="card">
      <div class="card-hdr">
        <span class="type-badge">{envelope.type}</span>
        <span class="route-badge">{envelope.route}</span>
        {#if envelope.version > 0}<span class="ver-badge">v{envelope.version}</span>{/if}
      </div>
      <table>
        <tbody>
          <tr><td>Route</td><td>{envelope.route}</td></tr>
          <tr><td>Payload type</td><td class="mono">{envelope.type} <span class="dim">(0x{envelope.typeCode.toString(16).padStart(2,"0")})</span></td></tr>
          <tr><td>Version</td><td>{envelope.version}</td></tr>
          <tr><td>Path hash size</td><td>{envelope.pathHashSize} byte{envelope.pathHashSize !== 1 ? "s" : ""} per hop</td></tr>
          <tr><td>Hops</td><td>{envelope.hopCount === 0 ? "0 (fresh)" : envelope.hops.join(" → ")}</td></tr>
          {#if envelope.isTransport && envelope.transportCodes}
            <tr><td>Transport codes</td><td class="mono">0x{envelope.transportCodes[0].toString(16).padStart(4,"0")} 0x{envelope.transportCodes[1].toString(16).padStart(4,"0")}</td></tr>
          {/if}
          <tr><td>Payload ({envelope.payloadHex.length/2}B)</td><td class="mono hex-wrap">{envelope.payloadHex}</td></tr>
        </tbody>
      </table>
    </div>

    <!-- ── Step 2 ──────────────────────────────────────────────────────────── -->
    {#if envelope.type === "GRP_TXT"}
      <section class="step2">
        <h3>Step 2 — decrypt GRP_TXT payload</h3>
        <div class="mode-toggle">
          <button class:active={p_grpKeyMode === "name"}   onclick={() => (p_grpKeyMode = "name")}>Channel name</button>
          <button class:active={p_grpKeyMode === "secret"} onclick={() => (p_grpKeyMode = "secret")}>Channel secret</button>
        </div>
        {#if p_grpKeyMode === "name"}
          <label>
            <span class="lbl">Channel name</span>
            <input bind:value={p_channel} placeholder="#test" />
          </label>
        {:else}
          <label>
            <span class="lbl">Channel secret (hex, 16 bytes)</span>
            <input bind:value={p_secret} placeholder="32 hex chars" class="mono" />
          </label>
        {/if}
        <button onclick={decodeGroupText}>Decrypt</button>
        {#if payloadErr}<div class="error">{payloadErr}</div>{/if}
        {#if payloadResult?.kind === "grptxt"}
          {@const d = payloadResult.data}
          <div class="payload-card">
            <div class="msg-text">"{d.text}"</div>
            <table>
              <tbody>
                <tr><td>Sender</td><td>{d.sender || "(none)"}</td></tr>
                <tr><td>Timestamp</td><td>{fmtTimestamp(d.timestamp)}</td></tr>
                <tr><td>Channel hash</td><td class="mono">{d.channelHash}</td></tr>
                {#if d.txtType !== 0}<tr><td>Txt type</td><td>{d.txtType}</td></tr>{/if}
                {#if d.attempt !== 0}<tr><td>Attempt</td><td>{d.attempt}</td></tr>{/if}
              </tbody>
            </table>
          </div>
        {/if}
      </section>

    {:else if envelope.type === "TXT_MSG"}
      <section class="step2">
        <h3>Step 2 — decrypt TXT_MSG payload</h3>
        <label>
          <span class="lbl">My private key (hex)</span>
          <div class="key-row">
            <input bind:value={p_priv} placeholder="64 hex chars" class="mono" />
            <button class="sm" onclick={generateKeypair}>Generate</button>
          </div>
        </label>
        <label>
          <span class="lbl">Peer public key (hex)</span>
          <input bind:value={p_peerPub} placeholder="64 hex chars" class="mono" />
        </label>
        <button onclick={decodeDirectText}>Decrypt</button>
        {#if payloadErr}<div class="error">{payloadErr}</div>{/if}
        {#if payloadResult?.kind === "txtmsg"}
          {@const d = payloadResult.data}
          <div class="payload-card">
            <div class="msg-text">"{d.text}"</div>
            <table>
              <tbody>
                <tr><td>Dest hash</td><td class="mono">{d.destHash}</td></tr>
                <tr><td>Src hash</td><td class="mono">{d.srcHash}</td></tr>
                <tr><td>Timestamp</td><td>{fmtTimestamp(d.timestamp)}</td></tr>
                {#if d.txtType !== 0}<tr><td>Txt type</td><td>{d.txtType}</td></tr>{/if}
                {#if d.attempt !== 0}<tr><td>Attempt</td><td>{d.attempt}</td></tr>{/if}
              </tbody>
            </table>
          </div>
        {/if}
      </section>

    {:else if envelope.type === "ADVERT"}
      {#if payloadErr}<div class="error">{payloadErr}</div>{/if}
      {#if payloadResult?.kind === "advert"}
        {@const d = payloadResult.data}
        <div class="payload-card">
          {#if d.name}<div class="node-name">📡 {d.name}</div>{/if}
          <table>
            <tbody>
              <tr><td>Public key</td><td class="mono hex-wrap">{d.publicKey}</td></tr>
              <tr><td>Timestamp</td><td>{fmtTimestamp(d.timestamp)}</td></tr>
              {#if d.hasGPS}
                <tr><td>GPS</td><td>{d.lat?.toFixed(6)}°, {d.lon?.toFixed(6)}°</td></tr>
              {/if}
            </tbody>
          </table>
        </div>
      {/if}

    {:else if envelope.type === "ACK"}
      <div class="info-note">
        ACK payload: 4-byte CRC — <code class="mono">{envelope.payloadHex}</code>
      </div>

    {:else}
      <div class="info-note">
        Raw payload ({envelope.payloadHex.length/2} bytes) — no structured decoder for <strong>{envelope.type}</strong>.
      </div>
    {/if}
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
    gap: 20px;
  }
  section { display: flex; flex-direction: column; gap: 10px; }
  .step2 { border-top: 1px solid #21262d; padding-top: 20px; }
  h3 { margin: 0; font-size: 13px; font-weight: 600; color: #8b949e; text-transform: uppercase; letter-spacing: 0.06em; }
  label { display: flex; flex-direction: column; gap: 4px; }
  .lbl { font-size: 11px; font-weight: 500; text-transform: uppercase; letter-spacing: 0.06em; color: #8b949e; }
  input, textarea {
    background: #0d1117; border: 1px solid #30363d; border-radius: 6px;
    color: #e6edf3; font-size: 13px; padding: 8px 10px; outline: none;
    resize: vertical; width: 100%; font-family: inherit;
  }
  input:focus, textarea:focus { border-color: #58a6ff; }
  .mono { font-family: "Cascadia Code", "Fira Code", monospace; font-size: 12px; }
  .key-row { display: flex; gap: 6px; }
  .key-row input { flex: 1; }

  button { background: #21262d; border: 1px solid #30363d; border-radius: 6px; color: #e6edf3; cursor: pointer; font-size: 13px; padding: 8px 14px; transition: background 0.1s; font-family: inherit; }
  button:hover { background: #30363d; }
  button.primary { background: #1f6feb; border-color: #1f6feb; color: #fff; font-weight: 600; }
  button.primary:hover { background: #388bfd; }
  button.sm { padding: 8px 10px; font-size: 12px; white-space: nowrap; }

  .mode-toggle { display: flex; gap: 0; border: 1px solid #30363d; border-radius: 6px; overflow: hidden; width: fit-content; }
  .mode-toggle button { background: #0d1117; border: none; border-radius: 0; color: #8b949e; font-size: 12px; padding: 5px 12px; }
  .mode-toggle button:hover { background: #21262d; color: #e6edf3; }
  .mode-toggle button.active { background: #21262d; color: #79c0ff; font-weight: 600; }
  .mode-toggle button:first-child { border-right: 1px solid #30363d; }
  .error { background: #3d1f1f; border: 1px solid #6e2a2a; border-radius: 6px; color: #f97583; font-size: 12px; padding: 8px 10px; }
  .info-note { background: #1c2128; border: 1px solid #30363d; border-radius: 6px; color: #8b949e; font-size: 12px; padding: 10px 12px; }
  .info-note code { color: #79c0ff; }

  /* Envelope card */
  .card { background: #0d1117; border: 1px solid #30363d; border-radius: 6px; overflow: hidden; }
  .card-hdr { display: flex; gap: 6px; padding: 10px 12px; border-bottom: 1px solid #21262d; flex-wrap: wrap; }
  .type-badge { background: #1f3a5c; color: #79c0ff; border: 1px solid #1f6feb; border-radius: 4px; font-size: 12px; font-weight: 600; padding: 2px 8px; font-family: monospace; }
  .route-badge { background: #1c2128; color: #8b949e; border: 1px solid #30363d; border-radius: 4px; font-size: 11px; padding: 2px 8px; }
  .ver-badge { background: #1c2128; color: #6e7681; border: 1px solid #30363d; border-radius: 4px; font-size: 11px; padding: 2px 8px; }

  /* Shared table style */
  table { width: 100%; border-collapse: collapse; font-size: 13px; }
  td { padding: 6px 12px; border-bottom: 1px solid #21262d; vertical-align: top; }
  td:last-child { border-bottom: none; }
  tr:last-child td { border-bottom: none; }
  td:first-child { color: #8b949e; width: 40%; white-space: nowrap; font-size: 12px; }
  td:last-child { color: #e6edf3; }
  .dim { color: #6e7681; }
  .hex-wrap { word-break: break-all; line-height: 1.6; }

  /* Payload card */
  .payload-card { background: #0d1117; border: 1px solid #238636; border-radius: 6px; overflow: hidden; }
  .msg-text { padding: 10px 12px; color: #7ee787; font-size: 14px; border-bottom: 1px solid #21262d; word-break: break-word; }
  .node-name { padding: 10px 12px; color: #f0f6fc; font-size: 15px; font-weight: 600; border-bottom: 1px solid #21262d; }
</style>
