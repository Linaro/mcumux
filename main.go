package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/google/goterm/term"
	"github.com/tarm/serial"
)

var port = flag.Int("port", 2167, "UDP port for mcumgr")

// Shared serial port
var ser *serial.Port
var pty *term.PTY

func main() {
	flag.Parse()

	err := netHandler()
	if err != nil {
		log.Fatalf("Unable to open network port: {}", err)
	}

	err = serialize()
	if err != nil {
		log.Fatalf("Unable to open serial port: {}", err)
	}
}

// netHandler Opens a UDP "connection" and begins handling requests
// that come from the mcumgr.  If successful, will return nil, with
// a goroutine running in the background to handle input.
func netHandler() error {
	// Use global pty
	var err error
	pty, err = term.OpenPTY()
	if err != nil {
		return err
	}

	name, err := pty.PTSName()
	if err != nil {
		return err
	}
	fmt.Printf("Use %s for pty\n", name)

	go reader()

	return nil
}

func reader() {
	buffer := make([]byte, 128)

	// There shouldn't be any data on the pty not destined for the
	// device, so just write it through.
	for {
		n, err := pty.Master.Read(buffer)
		if err != nil {
			fmt.Printf("pty error: {}", err)
			// Close?
			return
		}

		_, err = ser.Write(buffer[:n])
		if err != nil {
			fmt.Printf("serial write error: {}", err)
			return
		}
	}
}

// serialize Opens the serial port, capturing what is sent, and
// showing it on stdout.
func serialize() error {
	c := serial.Config{
		Name: "/dev/ttyACM0",
		Baud: 115200,
	}

	s, err := serial.OpenPort(&c)
	if err != nil {
		return err
	}
	defer s.Close()

	ser = s

	buffer := make([]byte, 1024)
	plain := make([]byte, 0, 1024)
	packet := make([]byte, 0, 128)

	state := 0
	var next byte = 0
	for {
		n, err := s.Read(buffer)
		if err != nil {
			return err
		}

		plain = plain[:0]
		for i := 0; i < n; i++ {
			switch state {
			case 0:
				// Idle state
				if buffer[i] == 6 {
					state = 1
					next = 9
					packet = append(packet[:0], buffer[i])
				} else if buffer[i] == 4 {
					state = 1
					next = 20
					packet = append(packet[:0], buffer[i])
				} else {
					plain = append(plain, buffer[i])
				}
			case 1:
				// Expecting character from packet.
				if buffer[i] != next {
					// Unexpected data.  Push them
					// onto the plain.
					plain = append(plain, packet[0], buffer[i])
					state = 0
				} else {
					state = 2
					packet = append(packet, buffer[i])
				}
			case 2:
				// Expecting newline. Everything else
				// just goes in the packet.
				packet = append(packet, buffer[i])
				if buffer[i] == '\n' {
					fmt.Printf("mcumgr: %q\n", packet)
					_, err = pty.Master.Write(packet)
					if err != nil {
						fmt.Printf("Error sending: {}", err)
					}
					state = 0
				}
			}
		}

		if len(plain) != 0 {
			_, err = os.Stdout.Write(plain)
			if err != nil {
				fmt.Printf("Error writing to stdout: {}", err)
			}
		}
	}

	return nil
}
