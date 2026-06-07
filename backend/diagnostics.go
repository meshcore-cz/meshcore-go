package backend

import "sync/atomic"

func (s *Server) trackIPCClient(delta int) {
	atomic.AddInt32(&s.ipcClients, int32(delta))
}

func (s *Server) ipcClientCount() int {
	return int(atomic.LoadInt32(&s.ipcClients))
}

func (s *Server) trackRadioWaiter(delta int) {
	atomic.AddInt32(&s.radioQueuePending, int32(delta))
}

func (s *Server) radioQueueCount() int {
	return int(atomic.LoadInt32(&s.radioQueuePending))
}

func (s *Server) trackReconnect() {
	atomic.AddInt32(&s.reconnects, 1)
}

func (s *Server) reconnectCount() int {
	return int(atomic.LoadInt32(&s.reconnects))
}

func (s *Server) trackRequestOK() {
	atomic.AddInt64(&s.requestsOK, 1)
}

func (s *Server) trackRequestFailed() {
	atomic.AddInt64(&s.requestsFailed, 1)
}

func (s *Server) requestCounts() (ok, failed int64) {
	return atomic.LoadInt64(&s.requestsOK), atomic.LoadInt64(&s.requestsFailed)
}
