// Package companion implements the standard MeshCore companion protocol used
// by the default client. It targets the companion-v3 framing shared by
// upstream MeshCore and compatible forks such as ZephCore.
//
// The wire layouts here follow the publicly documented companion protocol.
// Some response field offsets are marked provisional and may be refined as
// they are validated against more firmware builds; decoding is deliberately
// tolerant so that unknown or extended packets degrade to a RawMessage rather
// than failing.
package companion

// Command codes sent host -> device. The code is the first byte of the packet.
const (
	cmdAppStart         byte = 1
	cmdSendTxtMsg       byte = 2
	cmdSendChannelTxt   byte = 3
	cmdGetContacts      byte = 4
	cmdGetDeviceTime    byte = 5
	cmdSetDeviceTime    byte = 6
	cmdSendSelfAdvert   byte = 7
	cmdSetAdvertName    byte = 8
	cmdAddUpdateContact byte = 9
	cmdSyncNextMessage  byte = 10
	cmdSetRadioParams   byte = 11
	cmdSetTxPower       byte = 12
	cmdResetPath        byte = 13
	cmdSetAdvertLatLon  byte = 14
	cmdRemoveContact    byte = 15
	cmdShareContact     byte = 16
	cmdExportContact    byte = 17
	cmdImportContact    byte = 18
	cmdReboot           byte = 19
	cmdGetBattery       byte = 20
	cmdDeviceQuery      byte = 22
	cmdSendLogin        byte = 26
	cmdSendStatusReq    byte = 27
	cmdHasConnection    byte = 28
	cmdLogout           byte = 29
	cmdGetChannel       byte = 31
	cmdSetChannel       byte = 32
	cmdSendTracePath    byte = 36
	// cmdSendControlData wraps binary control-plane requests such as node
	// discovery. Value 55 follows the meshcore_py reference (SEND_CONTROL_DATA);
	// a hardware capture showed a 0x2e-prefixed framing, so this is provisional
	// and may need adjustment once verified on-device.
	cmdSendControlData byte = 55
	cmdGetStats        byte = 56
	cmdSendMeshPacket  byte = 65
)

// Local stats sub-types for CMD_GET_STATS / RESP_CODE_STATS.
const (
	StatsTypeCore    byte = 0
	StatsTypeRadio   byte = 1
	StatsTypePackets byte = 2
)

// Control-plane sub-types carried inside SEND_CONTROL_DATA / PUSH_CODE_CONTROL_DATA.
const (
	controlNodeDiscoverReq  byte = 0x80 // request; low bit set = prefix-only keys
	controlNodeDiscoverResp byte = 0x90 // response; low nibble carries node_type
)

// Response codes sent device -> host in reply to a command. The code is the
// first byte of the packet.
const (
	respOK               byte = 0
	respErr              byte = 1
	respContactsStart    byte = 2
	respContact          byte = 3
	respEndOfContacts    byte = 4
	respSelfInfo         byte = 5
	respSent             byte = 6
	respContactMsgRecv   byte = 7
	respChannelMsgRecv   byte = 8
	respCurrTime         byte = 9
	respNoMoreMessages   byte = 10
	respExportContact    byte = 11
	respBatteryVoltage   byte = 12
	respDeviceInfo       byte = 13
	respPrivateKey       byte = 14
	respDisabled         byte = 15
	respContactMsgRecvV3 byte = 16
	respChannelMsgRecvV3 byte = 17
	respChannelInfo      byte = 18
	respStats            byte = 24
)

// Push (asynchronous notification) codes. These have the high bit set, which
// is how the decoder distinguishes them from solicited responses.
const (
	pushAdvert        byte = 0x80
	pushPathUpdated   byte = 0x81
	pushSendConfirmed byte = 0x82
	pushMsgWaiting    byte = 0x83
	pushRawData       byte = 0x84
	pushLoginSuccess  byte = 0x85
	pushLoginFail     byte = 0x86
	pushStatusResp    byte = 0x87
	pushLogRxData     byte = 0x88
	pushTraceData     byte = 0x89
	pushNewAdvert     byte = 0x8A
	pushControlData   byte = 0x8E
)

// Device error codes carried in RESP_CODE_ERR frames (second byte).
const (
	ErrUnsupportedCmd byte = 1
	ErrNotFound       byte = 2
	ErrTableFull      byte = 3
	ErrBadState       byte = 4
	ErrFileIO         byte = 5
	ErrIllegalArg     byte = 6
)

// isPush reports whether a leading code byte denotes a push notification.
func isPush(code byte) bool { return code&0x80 != 0 }
