// Package ringbuf implements a bounded byte ring buffer.
//
// When the buffer is full, new writes overwrite the oldest data. This is used
// by the ditty daemon to buffer REPL output while no client is connected.
package ringbuf

// DefaultCapacity is the default ring buffer size (1 MB).
const DefaultCapacity = 1 << 20

// RingBuf is a fixed-capacity circular byte buffer. Writes that exceed the
// capacity silently overwrite the oldest data.
type RingBuf struct {
	buf  []byte
	cap  int
	head int // next write position
	len  int // number of valid bytes
}

// New creates a ring buffer with the given capacity.
func New(capacity int) *RingBuf {
	if capacity <= 0 {
		panic("ringbuf: capacity must be positive")
	}
	return &RingBuf{
		buf: make([]byte, capacity),
		cap: capacity,
	}
}

// Write appends data to the buffer. If the data exceeds the remaining
// capacity, the oldest bytes are overwritten. If len(data) >= capacity,
// only the last capacity bytes are kept.
func (r *RingBuf) Write(data []byte) {
	n := len(data)
	if n == 0 {
		return
	}

	// If the incoming data is larger than the buffer, only keep the tail.
	if n >= r.cap {
		copy(r.buf, data[n-r.cap:])
		r.head = 0
		r.len = r.cap
		return
	}

	// Write in up to two segments (wrap around).
	first := r.cap - r.head
	if first >= n {
		copy(r.buf[r.head:], data)
	} else {
		copy(r.buf[r.head:], data[:first])
		copy(r.buf, data[first:])
	}

	r.head = (r.head + n) % r.cap
	r.len += n
	if r.len > r.cap {
		r.len = r.cap
	}
}

// ReadAll returns all buffered data in order (oldest first) and clears the
// buffer. The returned slice is a newly allocated copy.
func (r *RingBuf) ReadAll() []byte {
	if r.len == 0 {
		return nil
	}

	out := make([]byte, r.len)
	if r.len < r.cap {
		// Buffer hasn't wrapped yet. Data starts at head - len.
		start := r.head - r.len
		if start < 0 {
			start += r.cap
		}
		if start+r.len <= r.cap {
			copy(out, r.buf[start:start+r.len])
		} else {
			first := r.cap - start
			copy(out, r.buf[start:])
			copy(out[first:], r.buf[:r.len-first])
		}
	} else {
		// Buffer is full. head points to the oldest byte.
		first := r.cap - r.head
		copy(out, r.buf[r.head:])
		copy(out[first:], r.buf[:r.head])
	}

	r.head = 0
	r.len = 0
	return out
}

// Len returns the number of bytes currently in the buffer.
func (r *RingBuf) Len() int {
	return r.len
}
