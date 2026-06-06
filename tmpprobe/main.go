package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"time"

	bugserial "go.bug.st/serial"
)

func main() {
	path := "/dev/cu.usbserial-0001"
	port, err := bugserial.Open(path, &bugserial.Mode{BaudRate: 115200})
	if err != nil { fmt.Println("open:", err); os.Exit(1) }
	defer port.Close()
	port.SetReadTimeout(2 * time.Second)

	// 1) Just listen for 2s for any unsolicited bytes.
	fmt.Println("== passive listen 2s ==")
	dump(port, 2*time.Second)

	// 2) Send APP_START with our '>' + LE16 len framing.
	name := "mcr"
	payload := append([]byte{1, 3, 0,0,0,0,0,0}, []byte(name)...)
	frame := append([]byte{'>'}, le16(uint16(len(payload)))...)
	frame = append(frame, payload...)
	fmt.Printf("== send framed APP_START (% x) ==\n", frame)
	port.Write(frame)
	dump(port, 3*time.Second)

	// 3) Send raw payload (no framing).
	fmt.Printf("== send RAW APP_START (% x) ==\n", payload)
	port.Write(payload)
	dump(port, 3*time.Second)
}

func le16(v uint16) []byte { b := make([]byte,2); binary.LittleEndian.PutUint16(b,v); return b }

func dump(port bugserial.Port, d time.Duration) {
	deadline := time.Now().Add(d)
	buf := make([]byte, 512)
	total := 0
	for time.Now().Before(deadline) {
		n, err := port.Read(buf)
		if n > 0 {
			total += n
			fmt.Printf("  recv %d: % x\n", n, buf[:n])
			fmt.Printf("       ascii: %q\n", string(buf[:n]))
		}
		if err != nil { break }
	}
	if total == 0 { fmt.Println("  (nothing)") }
}
