package companion

import "fmt"

// commandNames maps a host→device command opcode (the first byte of an outbound
// packet) to its CMD_* name. Used to label outbound frames in logs; outbound
// packets are not otherwise decoded back into typed commands.
var commandNames = map[byte]string{
	cmdAppStart:         "CMD_APP_START",
	cmdSendTxtMsg:       "CMD_SEND_TXT_MSG",
	cmdSendChannelTxt:   "CMD_SEND_CHANNEL_TXT_MSG",
	cmdGetContacts:      "CMD_GET_CONTACTS",
	cmdGetDeviceTime:    "CMD_GET_DEVICE_TIME",
	cmdSetDeviceTime:    "CMD_SET_DEVICE_TIME",
	cmdSendSelfAdvert:   "CMD_SEND_SELF_ADVERT",
	cmdSetAdvertName:    "CMD_SET_ADVERT_NAME",
	cmdAddUpdateContact: "CMD_ADD_UPDATE_CONTACT",
	cmdSyncNextMessage:  "CMD_SYNC_NEXT_MESSAGE",
	cmdSetRadioParams:   "CMD_SET_RADIO_PARAMS",
	cmdSetTxPower:       "CMD_SET_TX_POWER",
	cmdResetPath:        "CMD_RESET_PATH",
	cmdSetAdvertLatLon:  "CMD_SET_ADVERT_LATLON",
	cmdRemoveContact:    "CMD_REMOVE_CONTACT",
	cmdShareContact:     "CMD_SHARE_CONTACT",
	cmdExportContact:    "CMD_EXPORT_CONTACT",
	cmdImportContact:    "CMD_IMPORT_CONTACT",
	cmdReboot:           "CMD_REBOOT",
	cmdGetBattery:       "CMD_GET_BATTERY",
	cmdDeviceQuery:      "CMD_DEVICE_QUERY",
	cmdSendLogin:        "CMD_SEND_LOGIN",
	cmdSendStatusReq:    "CMD_SEND_STATUS_REQ",
	cmdHasConnection:    "CMD_HAS_CONNECTION",
	cmdLogout:           "CMD_LOGOUT",
	cmdGetChannel:       "CMD_GET_CHANNEL",
	cmdSetChannel:       "CMD_SET_CHANNEL",
	cmdSendTracePath:    "CMD_SEND_TRACE_PATH",
	cmdSendControlData:  "CMD_SEND_CONTROL_DATA",
	cmdGetStats:         "CMD_GET_STATS",
	cmdSendMeshPacket:   "CMD_SEND_RAW_PACKET",
}

// CommandName returns the CMD_* name for a host→device command opcode, or a
// generic "CMD(0x..)" label for an unrecognised opcode.
func CommandName(code byte) string {
	if name, ok := commandNames[code]; ok {
		return name
	}
	return fmt.Sprintf("CMD(0x%02x)", code)
}
