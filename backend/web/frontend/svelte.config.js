import adapter from '@sveltejs/adapter-static';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	kit: {
		// Single-page app: emit a static index.html shell and let the client
		// router handle the rest. The Go server embeds build/ and serves it.
		adapter: adapter({ fallback: 'index.html' })
	}
};

export default config;
