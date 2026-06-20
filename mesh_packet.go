package meshcore

import (
	"context"
	"fmt"
	"time"

	"github.com/meshcore-cz/meshcore-go/protocol"
	"github.com/meshcore-cz/meshcore-go/protocol/companion"
)

// SendMeshPacket transmits a pre-built wire-format MeshCore packet over the
// radio via CMD_SEND_RAW_PACKET (firmware PR #2543). The caller constructs the
// complete packet — header byte, path_len byte, path bytes, and encrypted
// payload — and the firmware's tryParsePacket validates the encoding before
// dispatching.
//
// Priority is a send-priority hint (0 = default). The device replies with OK
// on success, Err(ErrIllegalArg) if the packet bytes fail to parse, or
// Err(ErrTableFull) if the radio's packet pool is exhausted.
//
// Use the meshpkt sub-package to build common packet types.
func (c *Client) SendMeshPacket(ctx context.Context, priority byte, pkt []byte) error {
	msg, err := c.request(ctx, companion.SendMeshPacket{Priority: priority, Packet: pkt})
	if err != nil {
		return err
	}
	switch m := msg.(type) {
	case companion.OK:
		// Record the transmission in the unified RF log (Direction tx).
		c.rf.Emit(RFPacket{
			Timestamp: time.Now(),
			Direction: RFTx,
			Priority:  priority,
			Bytes:     append([]byte(nil), pkt...),
		})
		return nil
	case companion.Err:
		return fmt.Errorf("meshcore: device rejected mesh packet (error code %d)", m.Code)
	default:
		return protocol.ErrUnexpectedResponse
	}
}
