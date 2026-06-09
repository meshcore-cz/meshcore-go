package meshpkt

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// OpByName returns the Op registered under name.
func OpByName(name string) (Op, bool) {
	for _, op := range Ops {
		if op.Name == name {
			return op, true
		}
	}
	return Op{}, false
}

// ParseOpArgs converts positional JSON args to the []any shape expected by Op.Run.
// ParamHex values must be hex strings at the binding boundary.
func ParseOpArgs(op Op, raw []json.RawMessage) ([]any, error) {
	if len(raw) != len(op.Params) {
		return nil, fmt.Errorf("%s: need %d arg(s), got %d", op.Name, len(op.Params), len(raw))
	}
	args := make([]any, len(op.Params))
	for i, p := range op.Params {
		switch p.Kind {
		case ParamString:
			var s string
			if err := json.Unmarshal(raw[i], &s); err != nil {
				return nil, fmt.Errorf("arg %q: %w", p.Name, err)
			}
			args[i] = s
		case ParamHex:
			var s string
			if err := json.Unmarshal(raw[i], &s); err != nil {
				return nil, fmt.Errorf("arg %q: %w", p.Name, err)
			}
			b, err := hex.DecodeString(s)
			if err != nil {
				return nil, fmt.Errorf("arg %q: invalid hex: %w", p.Name, err)
			}
			args[i] = b
		case ParamInt:
			var n int
			if err := json.Unmarshal(raw[i], &n); err != nil {
				return nil, fmt.Errorf("arg %q: %w", p.Name, err)
			}
			args[i] = n
		default:
			return nil, fmt.Errorf("arg %q: unsupported param kind", p.Name)
		}
	}
	return args, nil
}

// Call dispatches a registered op by name with pre-parsed arguments.
func Call(name string, args []any) (map[string]any, error) {
	op, ok := OpByName(name)
	if !ok {
		return nil, fmt.Errorf("unknown op %q", name)
	}
	if len(args) != len(op.Params) {
		return nil, fmt.Errorf("%s: need %d arg(s), got %d", name, len(op.Params), len(args))
	}
	return op.Run(args)
}

// CallJSON dispatches an op using a JSON array of positional arguments.
// Returns a JSON object: success fields from the op, or {"error":"…"} on failure.
// Suitable for TinyGo //export call(opName, argsJSON) string bindings.
func CallJSON(name, argsJSON string) string {
	op, ok := OpByName(name)
	if !ok {
		return jsonResult(map[string]any{"error": fmt.Sprintf("unknown op %q", name)})
	}

	var raw []json.RawMessage
	if argsJSON != "" && argsJSON != "[]" {
		if err := json.Unmarshal([]byte(argsJSON), &raw); err != nil {
			return jsonResult(map[string]any{"error": fmt.Sprintf("invalid args JSON: %v", err)})
		}
	}

	args, err := ParseOpArgs(op, raw)
	if err != nil {
		return jsonResult(map[string]any{"error": err.Error()})
	}

	result, err := op.Run(args)
	if err != nil {
		return jsonResult(map[string]any{"error": err.Error()})
	}
	return jsonResult(result)
}

func jsonResult(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		b, _ = json.Marshal(map[string]string{"error": err.Error()})
	}
	return string(b)
}
