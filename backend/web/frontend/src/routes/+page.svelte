<script>
	import { onMount } from 'svelte';
	import { getJSON, postJSON } from '$lib/api.js';
	import { connectWS } from '$lib/ws.js';
	import LogTable from '$lib/LogTable.svelte';

	let tab = $state('dashboard');
	let status = $state(null);
	let statusErr = $state('');
	let busy = $state('');

	// Live event log (messages, adverts, acks) from the WebSocket.
	let events = $state([]);

	// Companion log: companion frames host↔radio, newest first.
	let packets = $state([]);
	let compPaused = $state(false);
	let compFilter = $state('all'); // all | in | out
	let pktSeq = 0; // monotonic id so frames in the same millisecond stay unique

	// RF log: decoded over-the-air RF packets (companion 0x88), newest first.
	let rfRows = $state([]);
	let rfPaused = $state(false);
	let rfSeq = 0;

	// Messages tab.
	let messages = $state([]);
	let recipient = $state('');
	let directText = $state('');
	let channelName = $state('');
	let channelText = $state('');
	let waitAck = $state(false);
	let sendResult = $state('');

	// Raw tab.
	let rawHex = $state('');
	let priority = $state(0);
	let useMeshPacket = $state(false);
	let rawResult = $state('');

	const daemon = $derived(status?.daemon ?? null);
	const device = $derived(status?.device ?? null);
	const devices = $derived(daemon?.Devices ?? []);

	function note(ev) {
		events = [{ at: new Date(), ...ev }, ...events].slice(0, 100);
	}

	onMount(() => {
		refreshStatus();
		loadMessages();
		const dispose = connectWS((env) => {
			if (env.type === 'status') {
				status = env.data;
				statusErr = '';
			} else if (env.type === 'event') {
				note(env.data);
				if (env.data.type === 'message') loadMessages();
			} else if (env.type === 'packet') {
				if (!compPaused)
					packets = [{ _id: ++pktSeq, at: new Date(), ...env.data }, ...packets].slice(0, 500);
			} else if (env.type === 'rf') {
				if (!rfPaused)
					rfRows = [{ _id: ++rfSeq, at: new Date(), ...env.data }, ...rfRows].slice(0, 500);
			}
		});
		return dispose;
	});

	async function refreshStatus() {
		try {
			status = await getJSON('/api/status');
			statusErr = '';
		} catch (e) {
			statusErr = String(e.message || e);
		}
	}

	async function loadMessages() {
		try {
			const r = await getJSON('/api/messages?limit=50');
			messages = r.messages ?? [];
		} catch {
			/* keep prior list */
		}
	}

	async function deviceAction(id, action) {
		busy = `${id}:${action}`;
		try {
			await postJSON(`/api/devices/${encodeURIComponent(id)}/${action}`, {});
			await refreshStatus();
		} catch (e) {
			statusErr = String(e.message || e);
		} finally {
			busy = '';
		}
	}

	async function advertise(flood) {
		busy = 'advert';
		try {
			await postJSON('/api/advert', { flood });
		} catch (e) {
			statusErr = String(e.message || e);
		} finally {
			busy = '';
		}
	}

	async function sendDirect() {
		sendResult = '';
		busy = 'send';
		try {
			const r = await postJSON('/api/send', { recipient, text: directText, wait: waitAck });
			sendResult = r.ack ? 'delivered (ack)' : r.ack_error ? `sent, no ack: ${r.ack_error}` : 'sent';
			directText = '';
			await loadMessages();
		} catch (e) {
			sendResult = `error: ${e.message || e}`;
		} finally {
			busy = '';
		}
	}

	async function sendChannel() {
		sendResult = '';
		busy = 'send-channel';
		try {
			await postJSON('/api/send-channel', { channel: channelName, text: channelText });
			sendResult = 'channel message sent';
			channelText = '';
			await loadMessages();
		} catch (e) {
			sendResult = `error: ${e.message || e}`;
		} finally {
			busy = '';
		}
	}

	async function sendRaw() {
		rawResult = '';
		busy = 'raw';
		try {
			if (useMeshPacket) {
				await postJSON('/api/mesh-packet', { priority: Number(priority), packet: rawHex });
				rawResult = 'mesh packet sent';
			} else {
				const r = await postJSON('/api/raw', { payload: rawHex });
				rawResult = JSON.stringify(r.result, null, 2);
			}
		} catch (e) {
			rawResult = `error: ${e.message || e}`;
		} finally {
			busy = '';
		}
	}

	function fmtTime(t) {
		if (!t) return '';
		const d = new Date(t);
		return isNaN(d) ? '' : d.toLocaleTimeString();
	}

	// Go marshals []byte as base64; render it as a hex string for the Tx log.
	function b64ToHex(b64) {
		if (!b64) return '';
		try {
			const bin = atob(b64);
			let out = '';
			for (let i = 0; i < bin.length; i++) out += bin.charCodeAt(i).toString(16).padStart(2, '0');
			return out;
		} catch {
			return '';
		}
	}

	function hexByte(n) {
		return '0x' + Number(n || 0).toString(16).padStart(2, '0');
	}

	const compView = $derived(
		compFilter === 'all' ? packets : packets.filter((p) => p.direction === compFilter)
	);

	function fmtSnr(v) {
		return typeof v === 'number' ? v.toFixed(2) : (v ?? '');
	}
</script>

<header>
	<h1>mc dashboard</h1>
	<nav>
		<button class:active={tab === 'dashboard'} onclick={() => (tab = 'dashboard')}>Dashboard</button>
		<button class:active={tab === 'messages'} onclick={() => (tab = 'messages')}>Messages</button>
		<button class:active={tab === 'companion'} onclick={() => (tab = 'companion')}>Companion Log</button>
		<button class:active={tab === 'rf'} onclick={() => (tab = 'rf')}>RF Log</button>
		<button class:active={tab === 'raw'} onclick={() => (tab = 'raw')}>Raw packets</button>
	</nav>
	<span class="conn" class:up={!!daemon?.Running}>
		{daemon?.Running ? `daemon up (pid ${daemon.PID})` : 'daemon offline'}
	</span>
</header>

{#if statusErr}
	<div class="banner err">{statusErr}</div>
{/if}

<main>
	{#if tab === 'dashboard'}
		<section class="grid">
			<div class="card">
				<h2>Daemon</h2>
				{#if daemon}
					<dl>
						<dt>PID</dt><dd>{daemon.PID}</dd>
						<dt>Uptime</dt><dd>{daemon.UptimeSec}s</dd>
						<dt>Version</dt><dd>{daemon.Version || '—'}</dd>
						<dt>Default</dt><dd>{daemon.DefaultID || '—'}</dd>
					</dl>
				{:else}
					<p class="muted">No daemon status.</p>
				{/if}
			</div>

			<div class="card">
				<h2>Device</h2>
				{#if device?.Device?.Name || device?.Device?.PublicKey}
					<dl>
						<dt>Name</dt><dd>{device.Device.Name || '—'}</dd>
						<dt>Key</dt><dd><code class="key">{device.Device.PublicKey}</code></dd>
						<dt>Firmware</dt><dd>{device.Device.Firmware} {device.Device.FirmwareVersion}</dd>
						<dt>Freq</dt><dd>{(device.Device.RadioFreqKHz / 1000).toFixed(3)} MHz</dd>
						<dt>SF / BW</dt><dd>SF{device.Device.RadioSF} / {device.Device.RadioBWKHz} kHz</dd>
						<dt>TX power</dt><dd>{device.Device.TxPowerDBm} dBm</dd>
						<dt>State</dt>
						<dd><span class="pill" class:ok={device.Healthy}>{device.State}</span></dd>
					</dl>
				{:else}
					<p class="muted">No connected device session.</p>
				{/if}
			</div>

			<div class="card">
				<h2>Replica</h2>
				{#if device}
					<dl>
						<dt>Contacts</dt><dd>{device.Contacts?.Count ?? 0}</dd>
						<dt>Channels</dt><dd>{device.Channels?.Count ?? 0}</dd>
						<dt>Clients</dt><dd>{device.Clients ?? 0}</dd>
						<dt>Reconnects</dt><dd>{device.Reconnects ?? 0}</dd>
					</dl>
				{:else}
					<p class="muted">—</p>
				{/if}
				<div class="row">
					<button disabled={busy === 'advert'} onclick={() => advertise(false)}>Advert (zero-hop)</button>
					<button disabled={busy === 'advert'} onclick={() => advertise(true)}>Advert (flood)</button>
				</div>
			</div>

			<div class="card wide">
				<h2>Sessions</h2>
				<table>
					<thead>
						<tr><th>ID</th><th>Session</th><th>Transport</th><th>URI</th><th></th></tr>
					</thead>
					<tbody>
						{#each devices as d (d.id)}
							<tr>
								<td>{d.id}{#if d.default}<span class="tag">default</span>{/if}</td>
								<td><span class="pill" class:ok={d.connected}>{d.session}</span></td>
								<td>{d.transport || '—'}</td>
								<td class="uri"><code>{d.uri || '—'}</code></td>
								<td class="actions">
									<button disabled={busy === `${d.id}:start`} onclick={() => deviceAction(d.id, 'start')}>start</button>
									<button disabled={busy === `${d.id}:stop`} onclick={() => deviceAction(d.id, 'stop')}>stop</button>
									<button disabled={busy === `${d.id}:restart`} onclick={() => deviceAction(d.id, 'restart')}>restart</button>
								</td>
							</tr>
						{:else}
							<tr><td colspan="5" class="muted">No devices registered.</td></tr>
						{/each}
					</tbody>
				</table>
			</div>

			<div class="card wide">
				<h2>Live events</h2>
				{#if events.length === 0}
					<p class="muted">Waiting for radio events…</p>
				{:else}
					<ul class="log">
						{#each events as e}
							<li><span class="t">{fmtTime(e.at)}</span> <span class="kind">{e.type}</span>
								{#if e.from}<b>{e.from}</b>{/if}
								{e.text || e.name || e.code || ''}</li>
						{/each}
					</ul>
				{/if}
			</div>
		</section>
	{:else if tab === 'messages'}
		<section class="grid">
			<div class="card">
				<h2>Send direct</h2>
				<input placeholder="recipient (name or key prefix)" bind:value={recipient} />
				<textarea placeholder="message" bind:value={directText}></textarea>
				<label class="check"><input type="checkbox" bind:checked={waitAck} /> wait for ack</label>
				<button disabled={busy === 'send' || !recipient || !directText} onclick={sendDirect}>Send</button>
			</div>
			<div class="card">
				<h2>Send to channel</h2>
				<input placeholder="channel (name or #hashtag)" bind:value={channelName} />
				<textarea placeholder="message" bind:value={channelText}></textarea>
				<button disabled={busy === 'send-channel' || !channelName || !channelText} onclick={sendChannel}>Send</button>
			</div>
			{#if sendResult}
				<div class="card wide"><p>{sendResult}</p></div>
			{/if}
			<div class="card wide">
				<div class="row between">
					<h2>Recent messages</h2>
					<button onclick={loadMessages}>Refresh</button>
				</div>
				<table>
					<thead><tr><th>Time</th><th>Dir</th><th>Peer / Channel</th><th>Text</th><th>Status</th></tr></thead>
					<tbody>
						{#each messages as m (m.id ?? m.ID ?? `${m.Timestamp}-${m.Text}`)}
							<tr>
								<td>{fmtTime(m.Timestamp || m.timestamp)}</td>
								<td>{m.Direction || m.direction}</td>
								<td>{m.PeerName || m.peer_name || m.Peer || m.peer || m.Channel || m.channel || ''}</td>
								<td class="text">{m.Text || m.text}</td>
								<td>{m.Status || m.status || ''}</td>
							</tr>
						{:else}
							<tr><td colspan="5" class="muted">No messages stored.</td></tr>
						{/each}
					</tbody>
				</table>
			</div>
		</section>
	{:else if tab === 'companion'}
		{#snippet compControls()}
			<div class="seg">
				<button class:active={compFilter === 'all'} onclick={() => (compFilter = 'all')}>all</button>
				<button class:active={compFilter === 'out'} onclick={() => (compFilter = 'out')}>out</button>
				<button class:active={compFilter === 'in'} onclick={() => (compFilter = 'in')}>in</button>
			</div>
		{/snippet}
		{#snippet compRow(p)}
			<td>{fmtTime(p.at || p.timestamp)}</td>
			<td><span class="dir" class:out={p.direction === 'out'}>{p.direction === 'out' ? '▲ out' : '▼ in'}</span></td>
			<td><code>{hexByte(p.type)}</code></td>
			<td
				>{p.decoded_type || ''}{#if p.async}<span class="tag">async</span>{/if}{#if p.decode_error}<span
						class="err-tag">{p.decode_error}</span
					>{/if}</td
			>
			<td class="bytes"><code>{b64ToHex(p.bytes)}</code></td>
		{/snippet}
		<section class="grid">
			<LogTable
				title="Companion Log"
				rows={compView}
				columns={[
					{ label: 'Time', width: '6.5rem' },
					{ label: 'Dir', width: '4.5rem' },
					{ label: 'Type', width: '4rem' },
					{ label: 'Decoded', width: '14rem' },
					{ label: 'Bytes' }
				]}
				row={compRow}
				controls={compControls}
				bind:paused={compPaused}
				onclear={() => (packets = [])}
				description="Companion frames between host and radio (CMD out / response & push in). Live; newest first, last 500 retained."
				empty="Waiting for frames…"
			/>
		</section>
	{:else if tab === 'rf'}
		{#snippet rfRow(p)}
			<td>{fmtTime(p.at || p.timestamp)}</td>
			<td
				><span class="dir" class:out={p.direction === 'tx'}
					>{p.direction === 'tx' ? '▲ tx' : '▼ rx'}</span
				></td
			>
			<td
				>{p.type || '—'}{#if p.decode_error}<span class="err-tag">undecoded</span>{/if}{#if p.direction === 'tx'}<span
						class="tag">prio {p.priority ?? 0}</span
					>{/if}</td
			>
			<td>{p.route || '—'}</td>
			<td>{p.hop_count ?? ''}</td>
			<td>{p.direction === 'tx' ? '—' : fmtSnr(p.snr) + ' dB'}</td>
			<td>{p.direction === 'tx' ? '—' : p.rssi + ' dBm'}</td>
			<td>{p.length}</td>
			<td class="bytes"><code>{p.bytes}</code></td>
		{/snippet}
		<section class="grid">
			<LogTable
				title="RF Log"
				rows={rfRows}
				columns={[
					{ label: 'Time', width: '6.5rem' },
					{ label: 'Dir', width: '4.5rem' },
					{ label: 'Type', width: '8rem' },
					{ label: 'Route', width: '7rem' },
					{ label: 'Hops', width: '3.5rem' },
					{ label: 'SNR', width: '5rem' },
					{ label: 'RSSI', width: '5rem' },
					{ label: 'Len', width: '3.5rem' },
					{ label: 'Bytes' }
				]}
				row={rfRow}
				bind:paused={rfPaused}
				onclear={() => (rfRows = [])}
				description="Over-the-air packets: received (rx, companion 0x88, with SNR/RSSI) and transmitted by us (tx, via SendMeshPacket). Decoded with meshpkt; newest first, last 500 retained."
				empty="Waiting for RF packets…"
			/>
		</section>
	{:else if tab === 'raw'}
		<section class="grid">
			<div class="card wide">
				<h2>Send raw packet</h2>
				<p class="muted">
					Hex-encoded payload. Off: companion <code>RawSend</code> (CMD_SEND_RAW).
					On: <code>SendMeshPacket</code> (CMD_SEND_RAW_PACKET) — a full wire-format packet
					with a send priority.
				</p>
				<textarea class="mono" placeholder="deadbeef…" bind:value={rawHex}></textarea>
				<div class="row">
					<label class="check"><input type="checkbox" bind:checked={useMeshPacket} /> mesh packet (with priority)</label>
					{#if useMeshPacket}
						<label class="check">priority <input class="num" type="number" min="0" max="255" bind:value={priority} /></label>
					{/if}
					<button disabled={busy === 'raw' || !rawHex} onclick={sendRaw}>Send</button>
				</div>
				{#if rawResult}
					<pre class="result">{rawResult}</pre>
				{/if}
			</div>
		</section>
	{/if}
</main>

<style>
	header {
		display: flex;
		align-items: center;
		gap: 1rem;
		padding: 0.75rem 1.25rem;
		border-bottom: 1px solid var(--border);
		background: var(--panel);
	}
	h1 {
		font-size: 1.05rem;
		margin: 0;
	}
	nav {
		display: flex;
		gap: 0.25rem;
	}
	nav button {
		background: transparent;
		border: 1px solid transparent;
		color: var(--muted);
		padding: 0.35rem 0.75rem;
		border-radius: 6px;
		cursor: pointer;
	}
	nav button.active {
		color: var(--text);
		border-color: var(--border);
		background: var(--bg);
	}
	.conn {
		margin-left: auto;
		color: var(--err);
		font-size: 0.85rem;
	}
	.conn.up {
		color: var(--ok);
	}
	main {
		padding: 1.25rem;
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
		gap: 1rem;
	}
	.card {
		background: var(--panel);
		border: 1px solid var(--border);
		border-radius: 10px;
		padding: 1rem;
	}
	.card.wide {
		grid-column: 1 / -1;
	}
	.card h2 {
		font-size: 0.95rem;
		margin: 0 0 0.75rem;
	}
	dl {
		display: grid;
		grid-template-columns: auto 1fr;
		gap: 0.25rem 0.75rem;
		margin: 0;
	}
	dt {
		color: var(--muted);
	}
	dd {
		margin: 0;
		text-align: right;
		word-break: break-all;
	}
	.key {
		font-size: 0.75rem;
	}
	.muted {
		color: var(--muted);
	}
	.banner.err {
		margin: 0.75rem 1.25rem 0;
		padding: 0.5rem 0.75rem;
		border: 1px solid var(--err);
		border-radius: 8px;
		color: var(--err);
		background: rgba(248, 81, 73, 0.08);
	}
	.pill {
		display: inline-block;
		padding: 0.05rem 0.5rem;
		border-radius: 999px;
		border: 1px solid var(--border);
		color: var(--muted);
		font-size: 0.8rem;
	}
	.pill.ok {
		color: var(--ok);
		border-color: var(--ok);
	}
	.tag {
		margin-left: 0.4rem;
		font-size: 0.7rem;
		color: var(--accent);
	}
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.85rem;
	}
	th,
	td {
		text-align: left;
		padding: 0.4rem 0.5rem;
		border-bottom: 1px solid var(--border);
		vertical-align: top;
	}
	th {
		color: var(--muted);
		font-weight: 600;
	}
	td.uri code,
	td.text {
		word-break: break-all;
	}
	.actions {
		white-space: nowrap;
	}
	input,
	textarea {
		width: 100%;
		background: var(--bg);
		border: 1px solid var(--border);
		color: var(--text);
		border-radius: 6px;
		padding: 0.5rem;
		margin-bottom: 0.6rem;
	}
	textarea {
		min-height: 5rem;
		resize: vertical;
	}
	textarea.mono {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
	}
	input.num {
		width: 5rem;
	}
	button {
		background: var(--accent);
		border: 1px solid var(--accent);
		color: #fff;
		padding: 0.4rem 0.8rem;
		border-radius: 6px;
		cursor: pointer;
	}
	button:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.actions button,
	.row button {
		background: var(--bg);
		border: 1px solid var(--border);
		color: var(--text);
	}
	.row {
		display: flex;
		gap: 0.5rem;
		align-items: center;
		flex-wrap: wrap;
		margin-top: 0.5rem;
	}
	.row.between {
		justify-content: space-between;
	}
	.check {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		color: var(--muted);
		margin-bottom: 0.6rem;
	}
	.check input {
		width: auto;
		margin: 0;
	}
	.log {
		list-style: none;
		margin: 0;
		padding: 0;
		max-height: 16rem;
		overflow: auto;
		font-size: 0.82rem;
	}
	.log li {
		padding: 0.2rem 0;
		border-bottom: 1px solid var(--border);
	}
	.log .t {
		color: var(--muted);
	}
	.log .kind {
		color: var(--accent);
		margin: 0 0.4rem;
	}
	.result {
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 0.75rem;
		overflow: auto;
		font-size: 0.82rem;
	}
	.seg {
		display: inline-flex;
		border: 1px solid var(--border);
		border-radius: 6px;
		overflow: hidden;
	}
	.seg button {
		background: var(--bg);
		border: none;
		border-radius: 0;
		color: var(--muted);
		padding: 0.3rem 0.6rem;
	}
	.seg button.active {
		background: var(--panel);
		color: var(--text);
	}
	.bytes code {
		word-break: break-all;
		color: var(--muted);
	}
	.dir {
		color: var(--accent);
	}
	.dir.out {
		color: var(--warn);
	}
	.err-tag {
		color: var(--err);
		font-size: 0.72rem;
		margin-left: 0.3rem;
	}
</style>
