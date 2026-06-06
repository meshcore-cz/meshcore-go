package ble

import (
	"strings"
	"sync"

	tinyble "tinygo.org/x/bluetooth"
)

var adapterEnableState = struct {
	sync.Mutex
	enabled map[*tinyble.Adapter]bool
}{enabled: map[*tinyble.Adapter]bool{}}

func ensureAdapterEnabled(adapter *tinyble.Adapter) error {
	adapterEnableState.Lock()
	defer adapterEnableState.Unlock()
	if adapterEnableState.enabled[adapter] {
		return nil
	}
	err := adapter.Enable()
	if err == nil || strings.Contains(err.Error(), "already calling Enable function") {
		adapterEnableState.enabled[adapter] = true
		return nil
	}
	return err
}
