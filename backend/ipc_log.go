package backend

import (
	"encoding/json"
	"strconv"
	"time"
)

func formatIPCParams(method string, params json.RawMessage) string {
	if len(params) == 0 {
		return ""
	}
	switch method {
	case "repeater_login":
		var p repeaterLoginParams
		if json.Unmarshal(params, &p) == nil {
			return "repeater=" + p.Repeater + " password=<redacted>"
		}
	case "send_text":
		var p sendTextParams
		if json.Unmarshal(params, &p) == nil {
			return "recipient=" + p.Recipient + " text_len=" + strconv.Itoa(len(p.Text))
		}
	case "send_channel_text":
		var p channelSendParams
		if json.Unmarshal(params, &p) == nil {
			return "channel=" + p.Channel + " text_len=" + strconv.Itoa(len(p.Text))
		}
	case "raw_send":
		var p rawParams
		if json.Unmarshal(params, &p) == nil {
			return "payload_len=" + strconv.Itoa(len(p.Payload))
		}
	case "repeater_exec":
		var p repeaterExecParams
		if json.Unmarshal(params, &p) == nil {
			return "repeater=" + p.Repeater + " command=" + p.Command
		}
	}
	s := string(params)
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}

func logIPCRequest(id uint64, method string, params json.RawMessage) {
	if detail := formatIPCParams(method, params); detail != "" {
		Logf("ipc request id=%d method=%s %s", id, method, detail)
		return
	}
	Logf("ipc request id=%d method=%s", id, method)
}

func logIPCResponse(id uint64, method string, err error, d time.Duration) {
	ms := d.Truncate(time.Millisecond)
	if err != nil {
		Logf("ipc response id=%d method=%s ok=false error=%q duration=%s", id, method, err.Error(), ms)
		return
	}
	Logf("ipc response id=%d method=%s ok=true duration=%s", id, method, ms)
}
