package backend

import (
	"fmt"
	"os"
	"time"
)

// Logf writes a timestamped line to stderr for the background backend log.
func Logf(format string, args ...any) {
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	fmt.Fprintf(os.Stderr, "%s %s\n", ts, fmt.Sprintf(format, args...))
}
