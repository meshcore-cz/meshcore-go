package backend

import (
	"fmt"
	"os"
	"time"
)

// Logf writes a timestamped line to stderr for the background backend log.
func Logf(format string, args ...any) {
	ts := time.Now().Format("2006-01-02 15:04:05.000")
	fmt.Fprintf(os.Stderr, "%s %s\n", ts, fmt.Sprintf(format, args...))
}
