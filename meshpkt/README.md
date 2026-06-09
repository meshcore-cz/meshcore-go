# meshpkt

Pure Go codec for the MeshCore radio packet wire format. WASM-safe — depends only on the Go standard library.

Key source: [MeshCore packet format](https://github.com/meshcore-dev/MeshCore/blob/main/docs/packet_format.md).

## Wire format

```
[header:1][transport_codes:0|4][path_len:1][path:N][payload]

header    = 0bVVPPPPRR
              RR   — route type (2 bits)
              PPPP — payload type (4 bits)
              VV   — version (2 bits)

path_len  = 0bSSHHHHHH
              HHHHHH — hop count (6 bits)
              SS     — (hashSize − 1) (2 bits) → hashSize = (byte >> 6) + 1

transport_codes — two uint16 LE values (4 bytes total), present only for
                  route types TRANSPORT_FLOOD (0x00) and TRANSPORT_DIRECT (0x03)
```

## Packet types

| Constant | Code | Description |
|---|---|---|
| `PayloadReq` | `0x00` | Request |
| `PayloadResponse` | `0x01` | Response |
| `PayloadTxtMsg` | `0x02` | Direct text message (TXT_MSG) |
| `PayloadAck` | `0x03` | Acknowledgement |
| `PayloadAdvert` | `0x04` | Node advertisement |
| `PayloadGrpTxt` | `0x05` | Channel text message (GRP_TXT) |
| `PayloadGrpData` | `0x06` | Channel data |
| `PayloadAnonReq` | `0x07` | Anonymous request |
| `PayloadPath` | `0x08` | Path |
| `PayloadTrace` | `0x09` | Trace |
| `PayloadMultipart` | `0x0A` | Multipart |
| `PayloadControl` | `0x0B` | Control |
| `PayloadRawCustom` | `0x0F` | Raw / custom |

## Payload formats

**GRP_TXT** (channel message)
```
[channel_hash:1][mac:2][AES-128-ECB ciphertext]
plaintext = [timestamp:4 LE][flags:1]["Sender: text"]
```

**TXT_MSG** (direct message)
```
[dest_hash:1][src_hash:1][mac:2][AES-128-ECB ciphertext]
plaintext = [timestamp:4 LE][flags:1]["text"]
```

**ADVERT** (node advertisement, unencrypted)
```
[pubkey:32][timestamp:4 LE][signature:64][appdata]
appdata = [flags:1][lat?:4 float32 LE][lon?:4 float32 LE][name string]
```

## Crypto

```
key32    = secret16 ‖ zero16
mac      = HMAC-SHA256(key32, ciphertext)[:2]
cipher   = AES-128-ECB, zero-padded to block boundary

channel secret  = SHA256(channelName)[:16]
channel hash    = SHA256(secret[:16])[0]    — 1-byte routing hint
direct secret   = X25519(myPriv, peerPub)[:16]
```

## Files

| File | Contents |
|---|---|
| `packet.go` | Envelope encode/decode, `RouteType`, `PayloadType`, `Packet` |
| `channel.go` | `DeriveChannelSecret`, `ChannelHash` |
| `crypto.go` | AES-128-ECB, HMAC-SHA256, `sealMAC`/`openMAC` (unexported) |
| `meshpkt.go` | GRP_TXT encode/decode, `Option` |
| `txtmsg.go` | TXT_MSG encode/decode |
| `advert.go` | ADVERT decode |
| `keys.go` | X25519 keypair generation, ECDH (`KeyPair`, `Generate`, `SharedSecret`, …) |
| `ops.go` | `Op` registry — declarative definitions consumed by binding layers |
| `call.go` | `CallJSON` / `Call` — JSON dispatch for TinyGo and HTTP bindings |
| `bindings/` | Copy-paste templates (TinyGo WASM, TypeScript codegen) — see [`bindings/README.md`](bindings/README.md) |

## Usage

```go
import "github.com/meshcore-cz/meshcore-go/meshpkt"

// Channel message — encode
secret := meshpkt.DeriveChannelSecret("#general")
pkt, err := meshpkt.GroupTextPacket(secret, "Alice", "hello", time.Now())

// Channel message — convenience (derive secret internally)
pkt, err := meshpkt.GroupTextPacketFromName("#general", "Alice", "hello", time.Now())

// Channel message — decode
envelope, err := meshpkt.DecodePacket(raw)
msg, err := meshpkt.DecodeGroupTextPayload(secret, envelope.Payload)
fmt.Println(msg.Sender, msg.Text)

// Direct message — encode (from hex key strings)
pkt, err := meshpkt.DirectTextPacketFromKeys(myPrivHex, peerPubHex, "hello", time.Now())

// Direct message — decode
msg, err := meshpkt.DecodeDirectTextPayloadFromKeys(envelope.Payload, myPrivHex, peerPubHex)

// ADVERT — decode
adv, err := meshpkt.DecodeAdvertPayload(envelope.Payload)

// Keypair
kp, err := meshpkt.Generate()
shared, err := meshpkt.SharedSecret(myPrivHex, peerPubHex)
```

## Op registry

`meshpkt.Ops` is a slice of `Op` descriptors that binding layers (WASM, HTTP, CLI) can iterate to wire up their dispatch tables without duplicating argument parsing logic:

```go
type Op struct {
    Name   string
    Params []Param   // each has Name and Kind (ParamString | ParamHex | ParamInt)
    Run    func(args []any) (map[string]any, error)
}
```

Copy [`bindings/wasm-lite.main.go.tmpl`](bindings/wasm-lite.main.go.tmpl) for TinyGo (~400 KB WASM) or [`bindings/wasm.main.go.tmpl`](bindings/wasm.main.go.tmpl) for full Go `syscall/js`. Optional TypeScript types: [`bindings/gen-ts.main.go.tmpl`](bindings/gen-ts.main.go.tmpl). JSON dispatch: `meshpkt.CallJSON(name, argsJSON)`.

## Notes

- Direct-message keys use native X25519. Real MeshCore devices use Ed25519 identities converted to Montgomery form for ECDH. Cross-check against firmware `Identity::calcSharedSecret` before relying on TXT_MSG interop with on-air hardware.
- Channel (GRP_TXT) crypto matches firmware and round-trips correctly.
