# Binding templates

Copy-paste starters for wiring `meshpkt.Ops` into another project. These files are
**not compiled** by the SDK (`*.tmpl` suffix). Paste into your app, rename to
`.go`, and adjust import paths / output paths as needed.

All packet logic stays in `meshpkt` — templates only handle transport glue.
Dispatch helpers live in [`call.go`](../call.go) (`meshpkt.CallJSON`).

## Templates

| File | Paste as | Purpose |
|------|----------|---------|
| [`wasm-lite.main.go.tmpl`](wasm-lite.main.go.tmpl) | `cmd/meshpkt-wasm-lite/main.go` | **Recommended.** TinyGo WASM, ~400 KB |
| [`gen-ts.main.go.tmpl`](gen-ts.main.go.tmpl) | `cmd/gen-ts/main.go` | Generate TypeScript types from `meshpkt.Ops` |
| [`wasm.main.go.tmpl`](wasm.main.go.tmpl) | `wasm/main.go` | Legacy full Go WASM (~3 MB, `syscall/js`) |

## TinyGo WASM quick start (recommended)

**Prerequisites:** TinyGo 0.39+ (Go 1.25), [binaryen](https://github.com/WebAssembly/binaryen) for `-opt=z`.

1. Add `github.com/meshcore-cz/meshcore-go` to your `go.mod` (use `replace` for local dev).
2. Copy `wasm-lite.main.go.tmpl` → `cmd/meshpkt-wasm-lite/main.go`.
3. Build:

   ```sh
   tinygo build -target=wasm -no-debug -opt=z -panic=trap \
     -o web/public/meshpkt.wasm ./cmd/meshpkt-wasm-lite
   cp "$(tinygo env TINYGOROOT)/targets/wasm_exec.js" web/public/
   ```

4. In the browser: load `wasm_exec.js`, `go.run()` the module, call `wasm.exports.call(name, JSON.stringify(args))`.
   Wrap exports using `meshcoreOpNames` from generated `wasm.gen.ts` (see example app `web/src/lib/wasm.ts`).

Byte arrays cross the boundary as lowercase hex strings inside the JSON arg array.
Each call returns a JSON object: success fields from the op, or `{error: "…"}`.

## TypeScript types (optional)

1. Copy `gen-ts.main.go.tmpl` → `cmd/gen-ts/main.go`.
2. Run:

   ```sh
   go run ./cmd/gen-ts -out web/src/lib/wasm.gen.ts
   ```

Re-run when `meshpkt.Ops` changes in a newer SDK version.

## Example app

See [meshcore-packet-tool](https://github.com/meshcore-cz/meshcore-packet-tool) for a
full Svelte + Vite app using these templates.
