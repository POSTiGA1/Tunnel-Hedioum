package pool

import (
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/yamux"
)

const (
	StateActive   int32 = 0
	StateDraining int32 = 1 // Connection is waiting for active logical streams to finish
)

// YamuxSession wraps the HashiCorp Yamux multiplexer with our Chaos Balancing metadata.
type YamuxSession struct {
	session *yamux.Session
	state   int32 // Atomic state: Active vs Draining

	// Atomic counters for real-time bandwidth calculation (Bytes)
	bytesTransferred uint64

	// Chaos Mesh / DPI Evasion Parameters
	baseLimitMbps  int
	jitterMbps     int
	currentCapMbps int

	lastActivity time.Time
	mu           sync.RWMutex
}

// NewYamuxSession initializes a monitored physical connection with a randomized bandwidth cap.
func NewYamuxSession(ys *yamux.Session, baseLimit, jitter int) *YamuxSession {
	s := &YamuxSession{
		session:       ys,
		state:         StateActive,
		baseLimitMbps: baseLimit,
		jitterMbps:    jitter,
		lastActivity:  time.Now(),
	}
	s.UpdateChaosLimit() // Initialize the first fluctuating cap
	return s
}

// monitoredStream is a Decorator for net.Conn that intercepts IO operations to count bytes atomically.
type monitoredStream struct {
	net.Conn
	parent *YamuxSession
}

func (m *monitoredStream) Read(b []byte) (int, error) {
	n, err := m.Conn.Read(b)
	if n > 0 {
		atomic.AddUint64(&m.parent.bytesTransferred, uint64(n))
	}
	return n, err
}

func (m *monitoredStream) Write(b []byte) (int, error) {
	n, err := m.Conn.Write(b)
	if n > 0 {
		atomic.AddUint64(&m.parent.bytesTransferred, uint64(n))
	}
	return n, err
}

// OpenStream opens a new logical stream, wrapping it in our bandwidth monitor.
func (ys *YamuxSession) OpenStream() (net.Conn, error) {
	stream, err := ys.session.OpenStream()
	if err == nil {
		ys.mu.Lock()
		ys.lastActivity = time.Now()
		ys.mu.Unlock()
		// Wrap the native stream to intercept and count traffic
		return &monitoredStream{Conn: stream, parent: ys}, nil
	}
	return nil, err
}

// GetAndResetBytes atomically fetches the total transferred bytes since the last check, and resets the counter to 0.
// This is ultra-efficient for the background watchdog to calculate real-time Mbps.
func (ys *YamuxSession) GetAndResetBytes() uint64 {
	return atomic.SwapUint64(&ys.bytesTransferred, 0)
}

// UpdateChaosLimit shifts the bandwidth cap randomly to evade DPI pattern matching.
func (ys *YamuxSession) UpdateChaosLimit() {
	ys.mu.Lock()
	defer ys.mu.Unlock()

	if ys.jitterMbps == 0 {
		ys.currentCapMbps = ys.baseLimitMbps
		return
	}

	// Calculate a random variance between -jitter and +jitter
	variance := rand.Intn((ys.jitterMbps*2)+1) - ys.jitterMbps
	ys.currentCapMbps = ys.baseLimitMbps + variance

	// Ensure the cap never drops to zero or below
	if ys.currentCapMbps < 1 {
		ys.currentCapMbps = 1
	}
}

// CurrentCap returns the active fluctuating limit for this specific connection.
func (ys *YamuxSession) CurrentCap() int {
	ys.mu.RLock()
	defer ys.mu.RUnlock()
	return ys.currentCapMbps
}

// --- State Management (Active / Draining) ---

func (ys *YamuxSession) SetDraining() {
	atomic.StoreInt32(&ys.state, StateDraining)
}

// Revive rescues a draining connection back to active duty, saving the overhead of a new TCP handshake.
func (ys *YamuxSession) Revive() {
	atomic.StoreInt32(&ys.state, StateActive)
}

func (ys *YamuxSession) IsDraining() bool {
	return atomic.LoadInt32(&ys.state) == StateDraining
}

func (ys *YamuxSession) IsActive() bool {
	return atomic.LoadInt32(&ys.state) == StateActive
}

// --- Core Wrapper Functions ---

func (ys *YamuxSession) ActiveStreams() int {
	if ys.session == nil || ys.session.IsClosed() {
		return 0
	}
	return ys.session.NumStreams()
}

func (ys *YamuxSession) IsClosed() bool {
	return ys.session.IsClosed()
}

func (ys *YamuxSession) Close() error {
	return ys.session.Close()
}

func (ys *YamuxSession) IdleTime() time.Duration {
	ys.mu.RLock()
	defer ys.mu.RUnlock()
	return time.Since(ys.lastActivity)
}
