package backend

import "sync/atomic"

func (s *DeviceSession) trackIPCClient(delta int) {
	atomic.AddInt32(&s.ipcClients, int32(delta))
}

func (s *DeviceSession) ipcClientCount() int {
	return int(atomic.LoadInt32(&s.ipcClients))
}

func (s *DeviceSession) trackRadioWaiter(delta int) {
	atomic.AddInt32(&s.radioQueuePending, int32(delta))
}

func (s *DeviceSession) radioQueueCount() int {
	return int(atomic.LoadInt32(&s.radioQueuePending))
}

func (s *DeviceSession) trackReconnect() {
	atomic.AddInt32(&s.reconnects, 1)
}

func (s *DeviceSession) reconnectCount() int {
	return int(atomic.LoadInt32(&s.reconnects))
}

func (s *DeviceSession) trackRequestOK() {
	atomic.AddInt64(&s.requestsOK, 1)
}

func (s *DeviceSession) trackRequestFailed() {
	atomic.AddInt64(&s.requestsFailed, 1)
}

func (s *DeviceSession) requestCounts() (ok, failed int64) {
	return atomic.LoadInt64(&s.requestsOK), atomic.LoadInt64(&s.requestsFailed)
}
