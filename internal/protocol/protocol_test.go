package protocol

import (
	"bytes"
	"io"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
	}{
		{
			name: "input with payload",
			msg:  Message{Type: MsgInput, Payload: []byte("print('hello')\n")},
		},
		{
			name: "stop no payload",
			msg:  Message{Type: MsgStop, Payload: nil},
		},
		{
			name: "kill no payload",
			msg:  Message{Type: MsgKill, Payload: nil},
		},
		{
			name: "interrupt no payload",
			msg:  Message{Type: MsgInterrupt, Payload: nil},
		},
		{
			name: "output with payload",
			msg:  Message{Type: MsgOutput, Payload: []byte("hello\n")},
		},
		{
			name: "prompt detected",
			msg:  Message{Type: MsgPromptDetected, Payload: nil},
		},
		{
			name: "exited with code",
			msg:  Message{Type: MsgExited, Payload: []byte{0}},
		},
		{
			name: "error with message",
			msg:  Message{Type: MsgError, Payload: []byte("something went wrong")},
		},
		{
			name: "empty payload",
			msg:  Message{Type: MsgOutput, Payload: []byte{}},
		},
		{
			name: "buffered output",
			msg:  Message{Type: MsgBufferedOutput, Payload: []byte("buffered data")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteMessage(&buf, tt.msg); err != nil {
				t.Fatalf("WriteMessage() error: %v", err)
			}

			got, err := ReadMessage(&buf)
			if err != nil {
				t.Fatalf("ReadMessage() error: %v", err)
			}

			if got.Type != tt.msg.Type {
				t.Errorf("type: got %d, want %d", got.Type, tt.msg.Type)
			}
			if !bytes.Equal(got.Payload, tt.msg.Payload) {
				t.Errorf("payload: got %q, want %q", got.Payload, tt.msg.Payload)
			}
		})
	}
}

func TestMultipleMessages(t *testing.T) {
	var buf bytes.Buffer

	msgs := []Message{
		{Type: MsgInput, Payload: []byte("x = 1\n")},
		{Type: MsgOutput, Payload: []byte(">>> ")},
		{Type: MsgPromptDetected},
	}

	for _, msg := range msgs {
		if err := WriteMessage(&buf, msg); err != nil {
			t.Fatalf("WriteMessage() error: %v", err)
		}
	}

	for i, want := range msgs {
		got, err := ReadMessage(&buf)
		if err != nil {
			t.Fatalf("ReadMessage() msg %d error: %v", i, err)
		}
		if got.Type != want.Type {
			t.Errorf("msg %d type: got %d, want %d", i, got.Type, want.Type)
		}
		if !bytes.Equal(got.Payload, want.Payload) {
			t.Errorf("msg %d payload: got %q, want %q", i, got.Payload, want.Payload)
		}
	}
}

func TestReadMessageEOF(t *testing.T) {
	var buf bytes.Buffer
	_, err := ReadMessage(&buf)
	if err == nil {
		t.Error("expected error reading from empty buffer")
	}
}

func TestReadMessageTruncatedHeader(t *testing.T) {
	// Only 3 bytes instead of 5.
	buf := bytes.NewReader([]byte{byte(MsgOutput), 0, 0})
	_, err := ReadMessage(buf)
	if err == nil {
		t.Error("expected error reading truncated header")
	}
}

func TestReadMessageTruncatedPayload(t *testing.T) {
	var buf bytes.Buffer
	// Write a header claiming 10 bytes of payload, but only provide 3.
	WriteMessage(&buf, Message{Type: MsgOutput, Payload: []byte("hello world")})
	// Truncate the buffer to cut off the payload.
	data := buf.Bytes()
	truncated := bytes.NewReader(data[:8]) // header (5) + 3 bytes
	_, err := ReadMessage(truncated)
	if err == nil {
		t.Error("expected error reading truncated payload")
	}
}

func TestPayloadTooLarge(t *testing.T) {
	// Craft a header with an enormous payload length.
	header := []byte{byte(MsgOutput), 0xFF, 0xFF, 0xFF, 0xFF}
	r := bytes.NewReader(header)
	_, err := ReadMessage(r)
	if err == nil {
		t.Error("expected error for oversized payload")
	}
}

func TestWriteMessageError(t *testing.T) {
	// Write to a writer that always fails.
	err := WriteMessage(failWriter{}, Message{Type: MsgOutput, Payload: []byte("x")})
	if err == nil {
		t.Error("expected error writing to failing writer")
	}
}

// failWriter is an io.Writer that always returns an error.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}
