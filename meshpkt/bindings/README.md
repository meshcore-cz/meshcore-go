# Binding templates

Copy-paste starters for wiring `meshpkt.Ops` into another project. These files are
**not compiled** by the SDK (`*.tmpl` suffix). Paste into your app, rename to
`.go`, and adjust import paths / output paths as needed.

All packet logic stays in `meshpkt` — templates only handle transport glue.

## Templates

| File | Paste as | Purpose |
|------|----------|---------|
| [`wasm.main.go.tmpl`](wasm.main.go.tmpl) | `wasm/main.go` | Browser binding via `syscall/js` → `window.meshcore` |
| [`gen-ts.main.go.tmpl`](gen-ts.main.go.tmpl) | `cmd/gen-ts/main.go` | Generate TypeScript types from `meshpkt.Ops` |

## WASM quick start

1. Add `github.com/meshcore-cz/meshcore-go` to your `go.mod` (use `replace` for local dev).
2. Copy `wasm.main.go.tmpl` → `wasm/main.go`.
3. Build:

   ```sh
   GOOS=js GOARCH=wasm go build -o web/public/meshcore.wasm ./wasm
   cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/public/
   ```

4. Load `wasm_exec.js` before instantiating the `.wasm` module; call `window.meshcore.*`.

Byte arrays cross the JS boundary as lowercase hex strings. Each function returns
`{…fields}` on success or `{error: "…"}` on failure.

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
