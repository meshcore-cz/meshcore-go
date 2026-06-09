package cli

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/meshcore-cz/meshpkt"
	"github.com/spf13/cobra"
)

// newPktCommand builds the `mc pkt` subtree by iterating meshpkt.Ops at
// startup — no per-operation code lives here.
func newPktCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pkt",
		Short: "Encode and decode MeshCore radio packets (offline)",
		Long: strings.TrimSpace(`
Encode and decode MeshCore radio packet wire formats without a live radio
connection. All operations are purely local — no device is required.

Output is always JSON. Encoded packets return {"hex":"..."}.`),
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = cmd.Help()
			return fmt.Errorf("missing subcommand")
		},
	}

	cmd.AddCommand(newPktDecodeCommand(app))

	subgroups := map[string]*cobra.Command{}

	for _, op := range meshpkt.Ops {
		op := op
		group, name := opGroupAndName(op.Name)
		if group == "decode" {
			continue // handled by the single decode command
		}

		use := name
		for _, p := range op.Params {
			use += " <" + camelToKebab(p.Name) + ">"
		}

		leaf := &cobra.Command{
			Use:   use,
			Short: opShort(op.Name),
			Args:  cobra.ExactArgs(len(op.Params)),
			RunE:  runWithEnvNoContext(app, nil, makeOpHandler(op)),
		}

		if group == "" {
			cmd.AddCommand(leaf)
			continue
		}
		if subgroups[group] == nil {
			g := &cobra.Command{
				Use:   group,
				Short: capitalize(group) + " packets",
				RunE: func(cmd *cobra.Command, _ []string) error {
					_ = cmd.Help()
					return fmt.Errorf("missing %s subcommand", cmd.Use)
				},
			}
			subgroups[group] = g
			cmd.AddCommand(g)
		}
		subgroups[group].AddCommand(leaf)
	}

	return cmd
}

func newPktDecodeCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "decode <packet-hex>",
		Short: "Decode a packet from hex",
		Long: strings.TrimSpace(`
Decode any MeshCore wire packet. The envelope (route, type, hops, payload) is
always decoded. For ADVERT packets the payload is decoded inline. Encrypted
payloads (GRP_TXT, TXT_MSG) include their raw payload hex.`),
		Args: cobra.MinimumNArgs(1),
		RunE: runWithEnvNoContext(app, nil, cmdPktDecode),
	}
}

func cmdPktDecode(e *env) error {
	raw, err := parseHexArgs(e.rest)
	if err != nil {
		return err
	}
	pkt, err := meshpkt.DecodePacket(raw)
	if err != nil {
		return err
	}

	hops := pkt.Hops()
	hopHex := make([]string, len(hops))
	for i, h := range hops {
		hopHex[i] = hex.EncodeToString(h)
	}

	out := map[string]any{
		"route":        pkt.Route.String(),
		"type":         pkt.Type.String(),
		"version":      int(pkt.Version),
		"pathHashSize": pkt.PathHashSize,
		"hopCount":     pkt.HopCount(),
		"hops":         hopHex,
		"payloadHex":   hex.EncodeToString(pkt.Payload),
	}
	if pkt.Route.IsTransport() {
		out["transportCodes"] = []int{int(pkt.TransportCodes[0]), int(pkt.TransportCodes[1])}
	}
	if pkt.Type == meshpkt.PayloadAdvert {
		if adv, err := meshpkt.DecodeAdvertPayload(pkt.Payload); err == nil {
			payload := map[string]any{
				"publicKey": hex.EncodeToString(adv.PublicKey),
				"timestamp": adv.Timestamp.Unix(),
				"name":      adv.Name,
				"hasGPS":    adv.HasGPS,
			}
			if adv.HasGPS {
				payload["lat"] = adv.Lat
				payload["lon"] = adv.Lon
			}
			out["payload"] = payload
		}
	}

	return pktJSON(e, out)
}

func makeOpHandler(op meshpkt.Op) func(*env) error {
	return func(e *env) error {
		args, err := parseOpCLIArgs(op, e.rest)
		if err != nil {
			return err
		}
		result, err := op.Run(args)
		if err != nil {
			return err
		}
		return pktJSON(e, result)
	}
}

func pktJSON(e *env, v any) error {
	e.out.JSON = true
	return e.out.JSONValue(v)
}

func parseOpCLIArgs(op meshpkt.Op, args []string) ([]any, error) {
	out := make([]any, len(op.Params))
	for i, p := range op.Params {
		switch p.Kind {
		case meshpkt.ParamString:
			out[i] = args[i]
		case meshpkt.ParamHex:
			b, err := hex.DecodeString(strings.ReplaceAll(args[i], " ", ""))
			if err != nil {
				return nil, fmt.Errorf("arg %q: invalid hex: %w", p.Name, err)
			}
			out[i] = b
		case meshpkt.ParamInt:
			n, err := strconv.Atoi(args[i])
			if err != nil {
				return nil, fmt.Errorf("arg %q: %w", p.Name, err)
			}
			out[i] = n
		}
	}
	return out, nil
}

func opGroupAndName(name string) (group, cmd string) {
	kebab := camelToKebab(name)
	prefix, rest, found := strings.Cut(kebab, "-")
	if found && (prefix == "encode" || prefix == "decode") {
		return prefix, rest
	}
	return "", kebab
}

func camelToKebab(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			b.WriteByte('-')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

func opShort(name string) string {
	words := strings.Split(camelToKebab(name), "-")
	if len(words) > 0 {
		words[0] = capitalize(words[0])
	}
	return strings.Join(words, " ")
}

func capitalize(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
