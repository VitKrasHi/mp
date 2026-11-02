package net

import (
	"fmt"
	"net"
	"sync"
	"time"
)

type hostPlayer struct {
	ID     uint32
	Name   string
	X, Y   float64
	Conn   net.Conn
	send   chan *Message
	lastDX float64 // последний input
	lastDY float64
}

type Host struct {
	listener    net.Listener
	broadcaster *Broadcaster
	gameName    string
	maxPlayers  int
	port        int

	mu      sync.RWMutex
	players map[uint32]*hostPlayer
	nextID  uint32
	localID uint32

	stop chan struct{}
}

func NewHost(gameName string, maxPlayers int) *Host {
	return &Host{
		gameName:   gameName,
		maxPlayers: maxPlayers,
		port:       DefaultTCPPort,
		players:    map[uint32]*hostPlayer{},
		nextID:     1,
		stop:       make(chan struct{}),
	}
}

func (h *Host) Start() error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", h.port))
	if err != nil {
		return err
	}
	h.listener = ln

	bc, err := NewBroadcaster(func() ServerInfo {
		h.mu.RLock()
		cur := len(h.players)
		h.mu.RUnlock()
		return ServerInfo{App: AppMagic, Version: 1, Port: h.port,
			GameName: h.gameName, MaxPlayers: h.maxPlayers, CurrentPlayers: cur}
	})
	if err != nil {
		ln.Close()
		return err
	}
	bc.Start()
	h.broadcaster = bc

	go h.tickLoop()
	go h.acceptLoop()
	go h.stateLoop()
	return nil
}

func (h *Host) acceptLoop() {
	for {
		conn, err := h.listener.Accept()
		if err != nil {
			return
		}
		go h.handleConn(conn)
	}
}

func (h *Host) handleConn(conn net.Conn) {
	if tc, ok := conn.(*net.TCPConn); ok {
		tc.SetNoDelay(true)
	}
	msg, err := ReadMessage(conn)
	if err != nil || msg.Type != MsgJoin || msg.Join == nil {
		conn.Close()
		return
	}
	h.mu.Lock()
	if len(h.players) >= h.maxPlayers {
		h.mu.Unlock()
		conn.Close()
		return
	}
	id := h.nextID
	h.nextID++
	p := &hostPlayer{ID: id, Name: msg.Join.Name, X: 400, Y: 300, Conn: conn, send: make(chan *Message, 128)}
	h.players[id] = p
	h.mu.Unlock()

	welcome := &Message{Type: MsgWelcome, Welcome: &WelcomeMsg{
		PlayerID: int(id), PlayerName: p.Name, GameName: h.gameName, MaxPlayers: h.maxPlayers,
	}}
	if err := WriteMessage(conn, welcome); err != nil {
		h.removePlayer(id)
		conn.Close()
		return
	}

	go h.sender(p)
	for {
		msg, err := ReadMessage(conn)
		if err != nil {
			break
		}
		if msg.Type == MsgInput && msg.Input != nil {
			h.applyInput(id, msg.Input)
		}
	}
	h.removePlayer(id)
	conn.Close()
}

func (h *Host) sender(p *hostPlayer) {
	for {
		select {
		case <-h.stop:
			return
		case m := <-p.send:
			if m == nil {
				return
			}
			if err := WriteMessage(p.Conn, m); err != nil {
				return
			}
		}
	}
}

func (h *Host) removePlayer(id uint32) {
	h.mu.Lock()
	delete(h.players, id)
	h.mu.Unlock()
}

func (h *Host) applyInput(id uint32, in *InputMsg) {
	h.mu.Lock()
	defer h.mu.Unlock()
	p, ok := h.players[id]
	if !ok {
		return
	}
	// Просто запоминаем направление — применяем его в tickLoop
	p.lastDX = in.DX
	p.lastDY = in.DY
}

// tickLoop применяет движение 60 раз в секунду с фиксированным dt.
func (h *Host) tickLoop() {
	const speed = 180.0 // px/сек
	const dt = 1.0 / 60.0
	t := time.NewTicker(time.Second / 60)
	defer t.Stop()
	for {
		select {
		case <-h.stop:
			return
		case <-t.C:
			h.mu.Lock()
			for _, p := range h.players {
				p.X += p.lastDX * speed * dt
				p.Y += p.lastDY * speed * dt
				if p.X < 20 {
					p.X = 20
				}
				if p.X > 780 {
					p.X = 780
				}
				if p.Y < 20 {
					p.Y = 20
				}
				if p.Y > 580 {
					p.Y = 580
				}
			}
			h.mu.Unlock()
		}
	}
}

func (h *Host) stateLoop() {
	t := time.NewTicker(30 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-h.stop:
			return
		case <-t.C:
			h.broadcastState()
		}
	}
}

func (h *Host) broadcastState() {
	h.mu.RLock()
	defer h.mu.RUnlock()
	players := make([]PlayerState, 0, len(h.players))
	for _, p := range h.players {
		players = append(players, PlayerState{ID: p.ID, Name: p.Name, X: p.X, Y: p.Y})
	}
	m := &Message{Type: MsgState, State: &StateMsg{Players: players}}
	for _, p := range h.players {
		if p.Conn == nil {
			continue
		}
		select {
		case p.send <- m:
		default:
		}
	}
}

func (h *Host) AddLocalPlayer(name string) uint32 {
	h.mu.Lock()
	defer h.mu.Unlock()
	id := h.nextID
	h.nextID++
	h.players[id] = &hostPlayer{ID: id, Name: name, X: 400, Y: 300}
	h.localID = id
	return id
}

func (h *Host) MyID() uint32 { return h.localID }

func (h *Host) Players() []PlayerState {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]PlayerState, 0, len(h.players))
	for _, p := range h.players {
		out = append(out, PlayerState{ID: p.ID, Name: p.Name, X: p.X, Y: p.Y})
	}
	return out
}

func (h *Host) SendInput(dx, dy float64) {
	h.applyInput(h.localID, &InputMsg{DX: dx, DY: dy})
}

func (h *Host) Close() {
	select {
	case <-h.stop:
		return
	default:
		close(h.stop)
	}
	if h.broadcaster != nil {
		h.broadcaster.Stop()
	}
	if h.listener != nil {
		h.listener.Close()
	}
	h.mu.Lock()
	for _, p := range h.players {
		if p.Conn != nil {
			p.Conn.Close()
		}
	}
	h.mu.Unlock()
}
