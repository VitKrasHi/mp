package net

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

const (
	AppMagic       = "SQWAVE_MP"
	multicastGroup = "239.255.77.77"
	multicastPort  = 25001
	bcInterval     = 1 * time.Second
	serverTTL      = 5 * time.Second
)

type ServerInfo struct {
	App            string    `json:"app"`
	Version        int       `json:"v"`
	Port           int       `json:"port"`
	GameName       string    `json:"name"`
	MaxPlayers     int       `json:"max"`
	CurrentPlayers int       `json:"cur"`
	Address        string    `json:"-"`
	LastSeen       time.Time `json:"-"`
}

type Broadcaster struct {
	infoFn func() ServerInfo
	conn   *net.UDPConn
	group  *net.UDPAddr
	done   chan struct{}
}

func NewBroadcaster(infoFn func() ServerInfo) (*Broadcaster, error) {
	group, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", multicastGroup, multicastPort))
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, err
	}
	return &Broadcaster{infoFn: infoFn, conn: conn, group: group, done: make(chan struct{})}, nil
}

func (b *Broadcaster) Start() {
	go func() {
		b.send()
		t := time.NewTicker(bcInterval)
		defer t.Stop()
		for {
			select {
			case <-b.done:
				return
			case <-t.C:
				b.send()
			}
		}
	}()
}

func (b *Broadcaster) send() {
	data, _ := json.Marshal(b.infoFn())
	b.conn.WriteToUDP(data, b.group)
}

func (b *Broadcaster) Stop() {
	select {
	case <-b.done:
	default:
		close(b.done)
	}
	b.conn.Close()
}

type Listener struct {
	conn    *net.UDPConn
	mu      sync.RWMutex
	servers map[string]*ServerInfo
	done    chan struct{}
}

func NewListener() (*Listener, error) {
	group, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", multicastGroup, multicastPort))
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenMulticastUDP("udp4", nil, group)
	if err != nil {
		return nil, err
	}
	conn.SetReadBuffer(64 * 1024)
	l := &Listener{conn: conn, servers: map[string]*ServerInfo{}, done: make(chan struct{})}
	go l.readLoop()
	go l.pruneLoop()
	return l, nil
}

func (l *Listener) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, raddr, err := l.conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-l.done:
				return
			default:
				time.Sleep(10 * time.Millisecond)
				continue
			}
		}
		var info ServerInfo
		if json.Unmarshal(buf[:n], &info) != nil {
			continue
		}
		if info.App != AppMagic || info.Version != 1 {
			continue
		}
		info.Address = raddr.IP.String()
		info.LastSeen = time.Now()
		key := fmt.Sprintf("%s:%d", info.Address, info.Port)
		l.mu.Lock()
		l.servers[key] = &info
		l.mu.Unlock()
	}
}

func (l *Listener) pruneLoop() {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-l.done:
			return
		case <-t.C:
			l.mu.Lock()
			for k, v := range l.servers {
				if time.Since(v.LastSeen) > serverTTL {
					delete(l.servers, k)
				}
			}
			l.mu.Unlock()
		}
	}
}

func (l *Listener) Servers() []ServerInfo {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]ServerInfo, 0, len(l.servers))
	for _, s := range l.servers {
		out = append(out, *s)
	}
	return out
}

func (l *Listener) Stop() {
	select {
	case <-l.done:
		return
	default:
		close(l.done)
	}
	l.conn.Close()
}
