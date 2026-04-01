package daemon

import (
	"net"

	"github.com/viettrungluu/ditty/internal/dlog"
	"github.com/viettrungluu/ditty/internal/protocol"
	"github.com/viettrungluu/ditty/internal/session"
)

// Server listens on a Unix domain socket and handles client connections.
type Server struct {
	ln     net.Listener
	daemon *Daemon
}

// NewServer creates and starts a Unix socket server for the given session.
func NewServer(name string, d *Daemon) (*Server, error) {
	ln, err := session.ListenSocket(name)
	if err != nil {
		return nil, err
	}
	dlog.Printf("server: listening for session %q", name)

	s := &Server{ln: ln, daemon: d}
	go s.acceptLoop()
	return s, nil
}

// Close shuts down the server.
func (s *Server) Close() {
	s.ln.Close()
}

// acceptLoop accepts client connections one at a time.
func (s *Server) acceptLoop() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			// Listener closed.
			return
		}
		s.handleConn(conn)
	}
}

// handleConn handles a single client connection. It blocks until the client
// disconnects or the session ends.
func (s *Server) handleConn(conn net.Conn) {
	c := &clientConn{conn: conn}
	defer func() {
		s.daemon.ClearClient(c)
		conn.Close()
	}()

	if err := s.daemon.SetClient(c); err != nil {
		protocol.WriteMessage(conn, protocol.Message{
			Type:    protocol.MsgError,
			Payload: []byte(err.Error()),
		})
		return
	}

	// Read messages from the client until disconnect or error.
	for {
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			return
		}

		dlog.Printf("server: received message type=%d len=%d",
			msg.Type, len(msg.Payload))

		switch msg.Type {
		case protocol.MsgInput:
			s.daemon.HandleInput(c, msg.Payload)
		case protocol.MsgInterrupt:
			s.daemon.HandleInterrupt()
		case protocol.MsgStop:
			s.daemon.HandleStop(c)
		case protocol.MsgKill:
			s.daemon.HandleKill(c)
		default:
			dlog.Printf("server: unknown message type: %d", msg.Type)
		}
	}
}

// clientConn wraps a net.Conn with helpers for sending protocol messages.
type clientConn struct {
	conn net.Conn
}

// sendOutput sends an Output message to the client.
func (c *clientConn) sendOutput(data []byte) error {
	return protocol.WriteMessage(c.conn, protocol.Message{
		Type:    protocol.MsgOutput,
		Payload: data,
	})
}

// sendBuffered sends a BufferedOutput message to the client.
func (c *clientConn) sendBuffered(data []byte) error {
	return protocol.WriteMessage(c.conn, protocol.Message{
		Type:    protocol.MsgBufferedOutput,
		Payload: data,
	})
}

// sendPromptDetected sends a PromptDetected message to the client.
func (c *clientConn) sendPromptDetected() error {
	return protocol.WriteMessage(c.conn, protocol.Message{
		Type: protocol.MsgPromptDetected,
	})
}

// sendExited sends an Exited message to the client with the given exit code.
func (c *clientConn) sendExited(code byte) error {
	return protocol.WriteMessage(c.conn, protocol.Message{
		Type:    protocol.MsgExited,
		Payload: []byte{code},
	})
}

// sendError sends an Error message to the client.
func (c *clientConn) sendError(msg string) error {
	return protocol.WriteMessage(c.conn, protocol.Message{
		Type:    protocol.MsgError,
		Payload: []byte(msg),
	})
}
