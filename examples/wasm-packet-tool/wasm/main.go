//go:build js && wasm

// Command wasm-packet-tool wires the meshpkt.Ops registry to the browser via a
// global `meshcore` JS object. Each Op becomes a JS function that accepts its
// declared parameters as positional arguments and returns {…result} | {error}.
//
// All byte arrays cross the boundary as lowercase hex strings (ParamHex args are
// decoded from hex before being passed to Run; []byte results are hex-encoded by
// the Op itself).
//
// Build: GOOS=js GOARCH=wasm go build -o web/public/meshcore.wasm ./wasm
// Runtime: copy $(go env GOROOT)/lib/wasm/wasm_exec.js to web/public/ and load
// it before instantiating the .wasm file.
package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"syscall/js"

	"github.com/meshcore-cz/meshcore-go/meshpkt"
)

func main() {
	mc := js.Global().Get("Object").New()

	for _, op := range meshpkt.Ops {
		op := op // capture for closure
		mc.Set(op.Name, js.FuncOf(func(_ js.Value, jsArgs []js.Value) any {
			// Validate argument count.
			if len(jsArgs) < len(op.Params) {
				return toJS(map[string]any{
					"error": fmt.Sprintf("%s: need %d arg(s), got %d", op.Name, len(op.Params), len(jsArgs)),
				})
			}

			// Parse arguments according to their declared kinds.
			args := make([]any, len(op.Params))
			for i, p := range op.Params {
				switch p.Kind {
				case meshpkt.ParamString:
					args[i] = jsArgs[i].String()
				case meshpkt.ParamHex:
					b, err := hex.DecodeString(jsArgs[i].String())
					if err != nil {
						return toJS(map[string]any{
							"error": fmt.Sprintf("arg %q: invalid hex: %v", p.Name, err),
						})
					}
					args[i] = b
				case meshpkt.ParamInt:
					args[i] = jsArgs[i].Int()
				}
			}

			// Dispatch.
			result, err := op.Run(args)
			if err != nil {
				return toJS(map[string]any{"error": err.Error()})
			}
			return toJS(result)
		}))
	}

	js.Global().Set("meshcore", mc)
	select {}
}

func toJS(v any) js.Value {
	b, _ := json.Marshal(v)
	return js.Global().Get("JSON").Call("parse", string(b))
}
