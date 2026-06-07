package queue

import "errors"

// ErrResponseOverflow is returned when the response buffer for an in-flight
// request is full. Protocol frames must not be silently dropped.
var ErrResponseOverflow = errors.New("protocol response buffer overflow")
