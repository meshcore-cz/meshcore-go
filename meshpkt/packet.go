// Package meshpkt encodes and decodes MeshCore radio packet wire formats.
//
// It is deliberately import-pure (stdlib crypto/* and encoding/* only) so
// the package compiles under GOOS=js GOARCH=wasm without modification.
//
// The full wire format (from docs/packet_format.md):
//
//	[header:1][transport_codes:0 or 4][path_len:1][path][payload]
//
// header byte layout (0bVVPPPPRR):
//
//	bits 1-0  route type  (RouteType)
//	bits 5-2  payload type (PayloadType)
//	bits 7-6  payload version (0–3)
//
// path_len byte layout (0bSSHHHHHH):
//
//	bits 5-0  hop count (0–63)
//	bits 7-6  path hash size − 1  → size = (byte >> 6) + 1
//
// transport_codes (2 × uint16 LE, 4 bytes total) are present only when route
// type is RouteTransportFlood or RouteTransportDirect.
//
// Payload formats:
//   - GRP_TXT:  [channel_hash:1][mac:2][AES-128-ECB ciphertext]
//   - TXT_MSG:  [dest_hash:1][src_hash:1][mac:2][AES-128-ECB ciphertext]
//   - ADVERT:   [pubkey:32][ts:4 LE][sig:64][appdata...]  (unencrypted)
//
// Plaintext inside encrypted payloads:
//
//	[timestamp:4 LE][flags:1][text]
//	flags = (txt_type << 2) | attempt
//
// Encryption: AES-128-ECB (zero-padded) + HMAC-SHA256(key32, ciphertext)[:2]
// where key32 = secret16 ‖ zero16.
package meshpkt

import (
	"encoding/binary"
	"fmt"
)

// RouteType is the 2-bit routing mode encoded in header bits 1-0.
type RouteType byte

const (
	RouteTransportFlood  RouteType = 0x00 // flood via transport relay
	RouteFlood           RouteType = 0x01 // direct flood
	RouteDirect          RouteType = 0x02 // direct (unicast)
	RouteTransportDirect RouteType = 0x03 // unicast via transport relay
)

// IsTransport reports whether transport codes are present in the wire packet.
func (r RouteType) IsTransport() bool {
	return r == RouteTransportFlood || r == RouteTransportDirect
}

// String returns a human-readable name for the route type.
func (r RouteType) String() string {
	switch r {
	case RouteTransportFlood:
		return "TRANSPORT_FLOOD"
	case RouteFlood:
		return "FLOOD"
	case RouteDirect:
		return "DIRECT"
	case RouteTransportDirect:
		return "TRANSPORT_DIRECT"
	default:
		return fmt.Sprintf("route(0x%02x)", byte(r))
	}
}

// PayloadType is the 4-bit payload type encoded in header bits 5-2.
type PayloadType byte

const (
	PayloadReq       PayloadType = 0x00 // request with dest/src hashes + MAC
	PayloadResponse  PayloadType = 0x01 // response to REQ or ANON_REQ
	PayloadTxtMsg    PayloadType = 0x02 // direct text message (encrypted)
	PayloadAck       PayloadType = 0x03 // acknowledgement (CRC only)
	PayloadAdvert    PayloadType = 0x04 // node advertisement (unencrypted)
	PayloadGrpTxt    PayloadType = 0x05 // group/channel text message (encrypted)
	PayloadGrpData   PayloadType = 0x06 // group datagram (unencrypted)
	PayloadAnonReq   PayloadType = 0x07 // anonymous request
	PayloadPath      PayloadType = 0x08 // returned path
	PayloadTrace     PayloadType = 0x09 // path trace (collects SNR per hop)
	PayloadMultipart PayloadType = 0x0A // packet in a sequence
	PayloadControl   PayloadType = 0x0B // unencrypted control data
	PayloadRawCustom PayloadType = 0x0F // custom/raw packet
)

// AllRouteTypes lists every defined RouteType in wire-format order.
// Useful for code generators and UI dropdowns.
var AllRouteTypes = []RouteType{
	RouteTransportFlood,
	RouteFlood,
	RouteDirect,
	RouteTransportDirect,
}

// AllPayloadTypes lists every defined PayloadType in wire-format order.
// Useful for code generators and UI dropdowns.
var AllPayloadTypes = []PayloadType{
	PayloadReq, PayloadResponse, PayloadTxtMsg, PayloadAck,
	PayloadAdvert, PayloadGrpTxt, PayloadGrpData, PayloadAnonReq,
	PayloadPath, PayloadTrace, PayloadMultipart, PayloadControl,
	PayloadRawCustom,
}

// String returns the payload type name (e.g. "GRP_TXT").
func (t PayloadType) String() string {
	switch t {
	case PayloadReq:
		return "REQ"
	case PayloadResponse:
		return "RESPONSE"
	case PayloadTxtMsg:
		return "TXT_MSG"
	case PayloadAck:
		return "ACK"
	case PayloadAdvert:
		return "ADVERT"
	case PayloadGrpTxt:
		return "GRP_TXT"
	case PayloadGrpData:
		return "GRP_DATA"
	case PayloadAnonReq:
		return "ANON_REQ"
	case PayloadPath:
		return "PATH"
	case PayloadTrace:
		return "TRACE"
	case PayloadMultipart:
		return "MULTIPART"
	case PayloadControl:
		return "CONTROL"
	case PayloadRawCustom:
		return "RAW_CUSTOM"
	default:
		return fmt.Sprintf("unknown(0x%02x)", byte(t))
	}
}

// Packet represents a decoded MeshCore radio packet.
type Packet struct {
	Route          RouteType
	Type           PayloadType
	Version        byte      // payload version, 0–3
	TransportCodes [2]uint16 // only meaningful when Route.IsTransport()
	PathHashSize   int       // bytes per hop hash, 1–4
	Path           []byte    // raw hop bytes; length = HopCount() × PathHashSize
	Payload        []byte    // raw payload bytes
}

// HopCount returns the number of hops encoded in the path.
func (p Packet) HopCount() int {
	if p.PathHashSize <= 0 {
		return 0
	}
	return len(p.Path) / p.PathHashSize
}

// Hops returns the path as a slice of per-hop hash bytes.
func (p Packet) Hops() [][]byte {
	size := p.PathHashSize
	if size <= 0 || len(p.Path) == 0 {
		return nil
	}
	if len(p.Path)%size != 0 {
		return nil
	}
	hops := make([][]byte, 0, len(p.Path)/size)
	for i := 0; i < len(p.Path); i += size {
		hop := make([]byte, size)
		copy(hop, p.Path[i:i+size])
		hops = append(hops, hop)
	}
	return hops
}

// EncodePacket serialises a Packet to its wire representation.
// PathHashSize defaults to 2 when zero.
func EncodePacket(p Packet) ([]byte, error) {
	if p.PathHashSize == 0 {
		p.PathHashSize = 2
	}
	if p.PathHashSize < 1 || p.PathHashSize > 4 {
		return nil, fmt.Errorf("meshpkt: unsupported path hash size %d (use 1–4)", p.PathHashSize)
	}
	hopCount := p.HopCount()
	if hopCount > 63 {
		return nil, fmt.Errorf("meshpkt: hop count %d exceeds maximum 63", hopCount)
	}
	if p.Version > 3 {
		return nil, fmt.Errorf("meshpkt: version %d out of range (0–3)", p.Version)
	}

	header := (p.Version << 6) | (byte(p.Type) << 2) | byte(p.Route)
	pathLen := byte((p.PathHashSize-1)<<6) | byte(hopCount)

	size := 2 + len(p.Path) + len(p.Payload) // header + path_len + path + payload
	if p.Route.IsTransport() {
		size += 4
	}
	buf := make([]byte, 0, size)
	buf = append(buf, header)
	if p.Route.IsTransport() {
		buf = binary.LittleEndian.AppendUint16(buf, p.TransportCodes[0])
		buf = binary.LittleEndian.AppendUint16(buf, p.TransportCodes[1])
	}
	buf = append(buf, pathLen)
	buf = append(buf, p.Path...)
	buf = append(buf, p.Payload...)
	return buf, nil
}

// DecodePacket parses a wire-format MeshCore packet into a Packet.
func DecodePacket(raw []byte) (Packet, error) {
	if len(raw) < 2 {
		return Packet{}, fmt.Errorf("meshpkt: packet too short (%d bytes, need at least 2)", len(raw))
	}

	var p Packet
	i := 0

	header := raw[i]
	i++
	p.Route = RouteType(header & 0x03)
	p.Type = PayloadType((header >> 2) & 0x0f)
	p.Version = header >> 6

	if p.Route.IsTransport() {
		if i+4 > len(raw) {
			return Packet{}, fmt.Errorf("meshpkt: truncated: transport_codes need 4 bytes at offset %d, have %d", i, len(raw)-i)
		}
		p.TransportCodes[0] = binary.LittleEndian.Uint16(raw[i:])
		p.TransportCodes[1] = binary.LittleEndian.Uint16(raw[i+2:])
		i += 4
	}

	if i >= len(raw) {
		return Packet{}, fmt.Errorf("meshpkt: missing path_len byte")
	}
	pathLen := raw[i]
	i++
	p.PathHashSize = int(pathLen>>6) + 1
	hopCount := int(pathLen & 0x3f)
	pathBytes := hopCount * p.PathHashSize
	if i+pathBytes > len(raw) {
		return Packet{}, fmt.Errorf("meshpkt: truncated: path needs %d bytes at offset %d, have %d", pathBytes, i, len(raw)-i)
	}
	p.Path = make([]byte, pathBytes)
	copy(p.Path, raw[i:i+pathBytes])
	i += pathBytes

	p.Payload = make([]byte, len(raw)-i)
	copy(p.Payload, raw[i:])
	return p, nil
}
