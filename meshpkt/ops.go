package meshpkt

import (
	"encoding/hex"
	"fmt"
	"time"
)

// ParamKind describes how an argument crosses an external binding boundary
// (e.g. JavaScript/WASM, HTTP, CLI).
type ParamKind int

const (
	ParamString ParamKind = iota // plain string
	ParamHex                     // hex-encoded bytes → binding decodes to []byte before Run
	ParamInt                     // integer
)

// Param describes a single input argument of an Op.
type Param struct {
	Name string
	Kind ParamKind
}

// ResultKind describes the TypeScript/JSON type of a result field.
type ResultKind int

const (
	ResultString      ResultKind = iota // → string
	ResultNumber                        // → number (int, float, unix timestamp)
	ResultBool                          // → boolean
	ResultStringArray                   // → string[]
	ResultNumberPair                    // → [number, number]
)

// ResultField describes a single field in the map[string]any returned by Op.Run.
type ResultField struct {
	Name     string
	Kind     ResultKind
	Optional bool
}

// Op describes a single callable operation. Binding layers (WASM, HTTP, CLI…)
// iterate Ops to build their dispatch tables automatically, and code generators
// use Params and Result to emit typed interfaces.
//
// Run receives pre-parsed arguments matching Params in order:
//   - ParamString → string
//   - ParamHex    → []byte (decoded by the binding layer)
//   - ParamInt    → int
//
// Result fields in the returned map use hex-encoded strings for byte slices so
// they cross any text boundary cleanly.
type Op struct {
	Name           string
	Params         []Param
	Result         []ResultField
	ResultTypeName string // friendly type name for code gen; defaults to pascal(Name)+"Result"
	Run            func(args []any) (map[string]any, error)
}

// Ops is the registry of all meshpkt operations exposed to binding layers.
var Ops = []Op{

	// ── encoders ──────────────────────────────────────────────────────────────

	{
		Name: "encodeGroupText",
		Params: []Param{
			{Name: "channelName", Kind: ParamString},
			{Name: "sender", Kind: ParamString},
			{Name: "text", Kind: ParamString},
		},
		Result:         []ResultField{{Name: "hex", Kind: ResultString}},
		ResultTypeName: "HexResult",
		Run: func(args []any) (map[string]any, error) {
			pkt, err := GroupTextPacketFromName(args[0].(string), args[1].(string), args[2].(string), time.Now())
			if err != nil {
				return nil, err
			}
			return map[string]any{"hex": hex.EncodeToString(pkt)}, nil
		},
	},

	{
		Name: "encodeGroupTextSecret",
		Params: []Param{
			{Name: "secret", Kind: ParamHex},
			{Name: "sender", Kind: ParamString},
			{Name: "text", Kind: ParamString},
		},
		Result: []ResultField{{Name: "hex", Kind: ResultString}},
		Run: func(args []any) (map[string]any, error) {
			secret := args[0].([]byte)
			if len(secret) < ChannelSecretLen {
				return nil, fmt.Errorf("secret must be at least %d bytes", ChannelSecretLen)
			}
			pkt, err := GroupTextPacket(secret[:ChannelSecretLen], args[1].(string), args[2].(string), time.Now())
			if err != nil {
				return nil, err
			}
			return map[string]any{"hex": hex.EncodeToString(pkt)}, nil
		},
	},

	{
		Name: "encodeDirectText",
		Params: []Param{
			{Name: "privKey", Kind: ParamString},
			{Name: "peerPubKey", Kind: ParamString},
			{Name: "text", Kind: ParamString},
		},
		Result: []ResultField{{Name: "hex", Kind: ResultString}},
		Run: func(args []any) (map[string]any, error) {
			pkt, err := DirectTextPacketFromKeys(args[0].(string), args[1].(string), args[2].(string), time.Now())
			if err != nil {
				return nil, err
			}
			return map[string]any{"hex": hex.EncodeToString(pkt)}, nil
		},
	},

	{
		Name: "encodeRaw",
		Params: []Param{
			{Name: "route", Kind: ParamInt},
			{Name: "payloadType", Kind: ParamInt},
			{Name: "version", Kind: ParamInt},
			{Name: "pathHashSize", Kind: ParamInt},
			{Name: "payload", Kind: ParamHex},
		},
		Result: []ResultField{{Name: "hex", Kind: ResultString}},
		Run: func(args []any) (map[string]any, error) {
			pkt := Packet{
				Route:        RouteType(args[0].(int)),
				Type:         PayloadType(args[1].(int)),
				Version:      byte(args[2].(int)),
				PathHashSize: args[3].(int),
				Payload:      args[4].([]byte),
			}
			raw, err := EncodePacket(pkt)
			if err != nil {
				return nil, err
			}
			return map[string]any{"hex": hex.EncodeToString(raw)}, nil
		},
	},

	// ── decoders ──────────────────────────────────────────────────────────────

	{
		Name: "decodeEnvelope",
		Params: []Param{
			{Name: "packet", Kind: ParamHex},
		},
		Result: []ResultField{
			{Name: "route", Kind: ResultString},
			{Name: "routeCode", Kind: ResultNumber},
			{Name: "type", Kind: ResultString},
			{Name: "typeCode", Kind: ResultNumber},
			{Name: "version", Kind: ResultNumber},
			{Name: "pathHashSize", Kind: ResultNumber},
			{Name: "hopCount", Kind: ResultNumber},
			{Name: "hops", Kind: ResultStringArray},
			{Name: "payloadHex", Kind: ResultString},
			{Name: "isTransport", Kind: ResultBool},
			{Name: "transportCodes", Kind: ResultNumberPair, Optional: true},
		},
		ResultTypeName: "Envelope",
		Run: func(args []any) (map[string]any, error) {
			pkt, err := DecodePacket(args[0].([]byte))
			if err != nil {
				return nil, err
			}
			hops := pkt.Hops()
			hopHex := make([]any, len(hops))
			for i, h := range hops {
				hopHex[i] = hex.EncodeToString(h)
			}
			result := map[string]any{
				"route":        pkt.Route.String(),
				"routeCode":    int(pkt.Route),
				"type":         pkt.Type.String(),
				"typeCode":     int(pkt.Type),
				"version":      int(pkt.Version),
				"pathHashSize": pkt.PathHashSize,
				"hopCount":     pkt.HopCount(),
				"hops":         hopHex,
				"payloadHex":   hex.EncodeToString(pkt.Payload),
				"isTransport":  pkt.Route.IsTransport(),
			}
			if pkt.Route.IsTransport() {
				result["transportCodes"] = []any{int(pkt.TransportCodes[0]), int(pkt.TransportCodes[1])}
			}
			return result, nil
		},
	},

	{
		Name: "decodeGroupText",
		Params: []Param{
			{Name: "payload", Kind: ParamHex},
			{Name: "channelName", Kind: ParamString},
		},
		Result: []ResultField{
			{Name: "sender", Kind: ResultString},
			{Name: "text", Kind: ResultString},
			{Name: "timestamp", Kind: ResultNumber},
			{Name: "txtType", Kind: ResultNumber},
			{Name: "attempt", Kind: ResultNumber},
			{Name: "channelHash", Kind: ResultString},
		},
		ResultTypeName: "GroupTextPayload",
		Run: func(args []any) (map[string]any, error) {
			secret := DeriveChannelSecret(args[1].(string))
			gt, err := DecodeGroupTextPayload(secret, args[0].([]byte))
			if err != nil {
				return nil, err
			}
			return opGroupTextResult(gt, secret), nil
		},
	},

	{
		Name: "decodeGroupTextSecret",
		Params: []Param{
			{Name: "payload", Kind: ParamHex},
			{Name: "secret", Kind: ParamHex},
		},
		Result: []ResultField{
			{Name: "sender", Kind: ResultString},
			{Name: "text", Kind: ResultString},
			{Name: "timestamp", Kind: ResultNumber},
			{Name: "txtType", Kind: ResultNumber},
			{Name: "attempt", Kind: ResultNumber},
			{Name: "channelHash", Kind: ResultString},
		},
		Run: func(args []any) (map[string]any, error) {
			secret := args[1].([]byte)
			if len(secret) < ChannelSecretLen {
				return nil, fmt.Errorf("secret must be at least %d bytes", ChannelSecretLen)
			}
			secret = secret[:ChannelSecretLen]
			gt, err := DecodeGroupTextPayload(secret, args[0].([]byte))
			if err != nil {
				return nil, err
			}
			return opGroupTextResult(gt, secret), nil
		},
	},

	{
		Name: "decodeDirectText",
		Params: []Param{
			{Name: "payload", Kind: ParamHex},
			{Name: "privKey", Kind: ParamString},
			{Name: "peerPubKey", Kind: ParamString},
		},
		Result: []ResultField{
			{Name: "destHash", Kind: ResultString},
			{Name: "srcHash", Kind: ResultString},
			{Name: "text", Kind: ResultString},
			{Name: "timestamp", Kind: ResultNumber},
			{Name: "txtType", Kind: ResultNumber},
			{Name: "attempt", Kind: ResultNumber},
		},
		ResultTypeName: "DirectTextPayload",
		Run: func(args []any) (map[string]any, error) {
			dt, err := DecodeDirectTextPayloadFromKeys(args[0].([]byte), args[1].(string), args[2].(string))
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"destHash":  fmt.Sprintf("%02x", dt.DestHash),
				"srcHash":   fmt.Sprintf("%02x", dt.SrcHash),
				"text":      dt.Text,
				"timestamp": dt.Timestamp.Unix(),
				"txtType":   int(dt.TxtType),
				"attempt":   int(dt.Attempt),
			}, nil
		},
	},

	{
		Name: "decodeAdvert",
		Params: []Param{
			{Name: "payload", Kind: ParamHex},
		},
		Result: []ResultField{
			{Name: "publicKey", Kind: ResultString},
			{Name: "timestamp", Kind: ResultNumber},
			{Name: "name", Kind: ResultString},
			{Name: "hasGPS", Kind: ResultBool},
			{Name: "lat", Kind: ResultNumber, Optional: true},
			{Name: "lon", Kind: ResultNumber, Optional: true},
		},
		ResultTypeName: "AdvertPayload",
		Run: func(args []any) (map[string]any, error) {
			adv, err := DecodeAdvertPayload(args[0].([]byte))
			if err != nil {
				return nil, err
			}
			result := map[string]any{
				"publicKey": hex.EncodeToString(adv.PublicKey),
				"timestamp": adv.Timestamp.Unix(),
				"name":      adv.Name,
				"hasGPS":    adv.HasGPS,
			}
			if adv.HasGPS {
				result["lat"] = adv.Lat
				result["lon"] = adv.Lon
			}
			return result, nil
		},
	},

	// ── utilities ─────────────────────────────────────────────────────────────

	{
		Name:   "generateKeypair",
		Params: []Param{},
		Result: []ResultField{
			{Name: "publicKey", Kind: ResultString},
			{Name: "privateKey", Kind: ResultString},
		},
		ResultTypeName: "KeypairResult",
		Run: func(args []any) (map[string]any, error) {
			kp, err := Generate()
			if err != nil {
				return nil, err
			}
			return map[string]any{"publicKey": kp.PublicKey, "privateKey": kp.PrivateKey}, nil
		},
	},

	{
		Name: "deriveChannelSecret",
		Params: []Param{
			{Name: "channelName", Kind: ParamString},
		},
		Result: []ResultField{{Name: "hex", Kind: ResultString}},
		Run: func(args []any) (map[string]any, error) {
			return map[string]any{"hex": hex.EncodeToString(DeriveChannelSecret(args[0].(string)))}, nil
		},
	},

	{
		Name: "sharedSecret",
		Params: []Param{
			{Name: "privKey", Kind: ParamString},
			{Name: "peerPubKey", Kind: ParamString},
		},
		Result: []ResultField{{Name: "hex", Kind: ResultString}},
		Run: func(args []any) (map[string]any, error) {
			shared, err := SharedSecret(args[0].(string), args[1].(string))
			if err != nil {
				return nil, err
			}
			return map[string]any{"hex": hex.EncodeToString(shared)}, nil
		},
	},
}

// opGroupTextResult builds the result map for a decoded GRP_TXT message.
func opGroupTextResult(gt GroupText, secret []byte) map[string]any {
	return map[string]any{
		"sender":      gt.Sender,
		"text":        gt.Text,
		"timestamp":   gt.Timestamp.Unix(),
		"txtType":     int(gt.TxtType),
		"attempt":     int(gt.Attempt),
		"channelHash": fmt.Sprintf("%02x", ChannelHash(secret)),
	}
}
