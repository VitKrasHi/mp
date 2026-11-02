package net

import (
	"fmt"
	"net"
	"sync"
	"time"
)

type snapshot struct {
	at      time.Time
	players []PlayerState
}

type Client struct {
	conn   net.Conn
	myID   uint32
	name   string
	sendCh chan *Message
	done   chan struct{}

	mu        sync.RWMutex
	snapshots []snapshot // последние ~10 снапшотов

	// Предсказание
	predictedX, predictedY float64
	isFirstInput           bool

	OnDisconnect func()
}

func NewClient() *Client {
	return &Client{sendCh: make(chan *Message, 64), done: make(chan struct{})}
}

func (c *Client) Connect(addr string, port int, name string) error {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", addr, port), 5*time.Second)
	if err != nil {
		return err
	}

	c.conn = conn
	if tc, ok := conn.(*net.TCPConn); ok {
		tc.SetNoDelay(true)
	}

	c.name = name

	if err := WriteMessage(conn, &Message{Type: MsgJoin, Join: &JoinMsg{Name: name}}); err != nil {
		conn.Close()
		return err
	}
	msg, err := ReadMessage(conn)
	if err != nil || msg.Type != MsgWelcome {
		conn.Close()
		return fmt.Errorf("bad welcome")
	}
	c.myID = uint32(msg.Welcome.PlayerID)

	go c.readLoop()
	go c.writeLoop()
	return nil
}

func (c *Client) readLoop() {
	for {
		msg, err := ReadMessage(c.conn)
		if err != nil {
			select {
			case <-c.done:
			default:
				if c.OnDisconnect != nil {
					c.OnDisconnect()
				}
			}
			return
		}
		if msg.Type == MsgState && msg.State != nil {
			c.mu.Lock()

			// 1. Сохраняем снапшот для интерполяции
			c.snapshots = append(c.snapshots, snapshot{
				at:      time.Now(),
				players: msg.State.Players,
			})
			if len(c.snapshots) > 10 {
				c.snapshots = c.snapshots[len(c.snapshots)-10:]
			}

			// 2. КОРРЕКТИРОВКА (Reconciliation) для своего игрока
			for _, p := range msg.State.Players {
				if p.ID == c.myID {
					if !c.isFirstInput {
						c.predictedX = p.X
						c.predictedY = p.Y
						c.isFirstInput = true
					} else {
						// Плавно тянем предсказание к реальной позиции с хоста.
						// 0.2 означает, что за 1 снапшот мы исправляем 20% ошибки.
						c.predictedX += (p.X - c.predictedX) * 0.2
						c.predictedY += (p.Y - c.predictedY) * 0.2
					}
				}
			}
			c.mu.Unlock()
		}
	}
}

func (c *Client) writeLoop() {
	for {
		select {
		case <-c.done:
			return
		case m := <-c.sendCh:
			if err := WriteMessage(c.conn, m); err != nil {
				return
			}
		}
	}
}

func (c *Client) SendInput(dx, dy float64) {
	// Защищаем предсказание мьютексом, т.к. readLoop тоже его меняет
	c.mu.Lock()
	const speed = 180.0
	const dt = 1.0 / 60.0
	c.predictedX += dx * speed * dt
	c.predictedY += dy * speed * dt

	if c.predictedX < 20 {
		c.predictedX = 20
	}
	if c.predictedX > 780 {
		c.predictedX = 780
	}
	if c.predictedY < 20 {
		c.predictedY = 20
	}
	if c.predictedY > 580 {
		c.predictedY = 580
	}
	c.mu.Unlock()

	select {
	case c.sendCh <- &Message{Type: MsgInput, Input: &InputMsg{DX: dx, DY: dy}}:
	default:
	}
}

func (c *Client) MyID() uint32 { return c.myID }

func (c *Client) Players() []PlayerState {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.snapshots) < 3 {
		if len(c.snapshots) > 0 {
			return c.snapshots[0].players
		}
		return nil
	}

	renderTime := time.Now().Add(-150 * time.Millisecond)

	var a, b *snapshot
	for i := range c.snapshots {
		if c.snapshots[i].at.Before(renderTime) || c.snapshots[i].at.Equal(renderTime) {
			a = &c.snapshots[i]
		}
		if c.snapshots[i].at.After(renderTime) {
			b = &c.snapshots[i]
			break
		}
	}
	if a == nil {
		a = &c.snapshots[0]
	}
	if b == nil {
		b = &c.snapshots[len(c.snapshots)-1]
	}

	if a == b || b.at.Equal(a.at) {
		// Даже если не интерполируем, нам нужно подменить своего игрока
		out := make([]PlayerState, len(a.players))
		copy(out, a.players)
		for i := range out {
			if out[i].ID == c.myID {
				out[i].X = c.predictedX
				out[i].Y = c.predictedY
			}
		}
		return out
	}

	span := b.at.Sub(a.at).Seconds()
	elapsed := renderTime.Sub(a.at).Seconds()
	t := elapsed / span
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}

	bByID := map[uint32]PlayerState{}
	for _, p := range b.players {
		bByID[p.ID] = p
	}
	out := make([]PlayerState, 0, len(a.players))
	for _, pa := range a.players {
		pb, ok := bByID[pa.ID]
		if !ok {
			out = append(out, pa)
			continue
		}

		// Интерполируем всех
		pX := pa.X + (pb.X-pa.X)*t
		pY := pa.Y + (pb.Y-pa.Y)*t

		// ПОДМЕНЯЕМ СВОЕГО ИГРОКА НА ПРЕДСКАЗАННОГО
		if pa.ID == c.myID {
			pX = c.predictedX
			pY = c.predictedY
		}

		out = append(out, PlayerState{
			ID:   pa.ID,
			Name: pa.Name,
			X:    pX,
			Y:    pY,
		})
	}
	return out
}

func (c *Client) Close() {
	select {
	case <-c.done:
		return
	default:
		close(c.done)
	}
	if c.conn != nil {
		c.conn.Close()
	}
}
