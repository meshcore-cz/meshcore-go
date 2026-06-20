// connectWS opens the dashboard WebSocket and invokes onMessage with each parsed
// envelope ({type, data}). It auto-reconnects with a fixed backoff until the
// returned disposer is called.
export function connectWS(onMessage) {
	let ws;
	let closed = false;
	let timer;

	function open() {
		const proto = location.protocol === 'https:' ? 'wss' : 'ws';
		ws = new WebSocket(`${proto}://${location.host}/ws`);
		ws.onmessage = (e) => {
			try {
				onMessage(JSON.parse(e.data));
			} catch {
				/* ignore malformed frames */
			}
		};
		ws.onclose = () => {
			if (!closed) timer = setTimeout(open, 2000);
		};
	}
	open();

	return () => {
		closed = true;
		clearTimeout(timer);
		if (ws) ws.close();
	};
}
