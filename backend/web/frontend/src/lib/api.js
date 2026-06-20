// Thin fetch helpers over the backend JSON API. Every endpoint returns either a
// JSON body or {"error": "..."} with a non-2xx status.

export async function getJSON(path) {
	const res = await fetch(path);
	const data = await res.json().catch(() => ({}));
	if (!res.ok) throw new Error(data.error || res.statusText);
	return data;
}

export async function postJSON(path, body) {
	const res = await fetch(path, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(body || {})
	});
	const data = await res.json().catch(() => ({}));
	if (!res.ok) throw new Error(data.error || res.statusText);
	return data;
}
