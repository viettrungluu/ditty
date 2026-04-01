// Package protocol defines the wire protocol between ditty clients and
// per-session daemons.
//
// Each message is framed as:
//
//	[1 byte type][4 bytes big-endian payload length][payload]
//
// Client-to-daemon message types: Input, Stop, Kill, Interrupt.
// Daemon-to-client message types: Output, PromptDetected, Exited, Error.
package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
)

// MsgType identifies the kind of protocol message.
type MsgType byte

const (
	// Client → Daemon messages.

	// MsgInput sends a line of input to the REPL.
	MsgInput MsgType = 1
	// MsgStop requests a graceful shutdown (EOF + wait).
	MsgStop MsgType = 2
	// MsgKill requests a forced shutdown (SIGTERM/SIGKILL).
	MsgKill MsgType = 3
	// MsgInterrupt sends an interrupt (Ctrl-C / \x03) to the REPL.
	MsgInterrupt MsgType = 4

	// Daemon → Client messages.

	// MsgOutput carries a chunk of REPL output.
	MsgOutput MsgType = 10
	// MsgPromptDetected signals that a prompt has been detected.
	MsgPromptDetected MsgType = 11
	// MsgExited signals that the REPL process has exited. The payload is a
	// single byte containing the exit code (0-255).
	MsgExited MsgType = 12
	// MsgError carries an error message string.
	MsgError MsgType = 13
	// MsgBufferedOutput carries buffered output that accumulated while no
	// client was connected.
	MsgBufferedOutput MsgType = 14
)

// maxPayloadSize limits individual message payloads to 16 MB.
const maxPayloadSize = 16 << 20

// Message is a protocol message.
type Message struct {
	Type    MsgType
	Payload []byte
}

// WriteMessage writes a framed message to w.
func WriteMessage(w io.Writer, msg Message) error {
	header := make([]byte, 5)
	header[0] = byte(msg.Type)
	binary.BigEndian.PutUint32(header[1:5], uint32(len(msg.Payload)))

	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if len(msg.Payload) > 0 {
		if _, err := w.Write(msg.Payload); err != nil {
			return fmt.Errorf("write payload: %w", err)
		}
	}
	return nil
}

// ReadMessage reads a framed message from r.
func ReadMessage(r io.Reader) (Message, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(r, header); err != nil {
		return Message{}, fmt.Errorf("read header: %w", err)
	}

	msgType := MsgType(header[0])
	payloadLen := binary.BigEndian.Uint32(header[1:5])

	if payloadLen > maxPayloadSize {
		return Message{}, fmt.Errorf("payload too large: %d bytes (max %d)",
			payloadLen, maxPayloadSize)
	}

	var payload []byte
	if payloadLen > 0 {
		payload = make([]byte, payloadLen)
		if _, err := io.ReadFull(r, payload); err != nil {
			return Message{}, fmt.Errorf("read payload: %w", err)
		}
	}

	return Message{Type: msgType, Payload: payload}, nil
}
