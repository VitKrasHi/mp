package net

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	AppMagic      = "SQWAVE_MP"
	DiscoveryPort = 25001
	bcInterval    = 1 * time.Second
	serverTTL     = 5 * time.Second
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

// Broadcaster (Хост): отправляет пакеты на адрес 255.255.255.255
type Broadcaster struct {
	infoFn func() ServerInfo
	conn   *net.UDPConn
	done   chan struct{}
}

func NewBroadcaster(infoFn func() ServerInfo) (*Broadcaster, error) {
	conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4bcast, Port: DiscoveryPort})
	if err != nil {
		return nil, err
	}
	conn.SetWriteBuffer(2048)
	return &Broadcaster{infoFn: infoFn, conn: conn, done: make(chan struct{})}, nil
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
	b.conn.Write(data)
}

func (b *Broadcaster) Stop() {
	select {
	case <-b.done:
	default:
		close(b.done)
	}
	b.conn.Close()
}

// Listener (Клиент): слушает порт 25001 на всех интерфейсах
type Listener struct {
	conn    *net.UDPConn
	mu      sync.RWMutex
	servers map[string]*ServerInfo
	done    chan struct{}
}

func NewListener() (*Listener, error) {
	// Слушаем 0.0.0.0:25001, чтобы принимать пакеты отовсюду (LAN, ZeroTier, Radmin)
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: DiscoveryPort})
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

// Добавь эту функцию в конец discovery.go
func ScanSubnet(port int) []string {
	var ips []string
	var mu sync.Mutex

	add := func(ip string) {
		mu.Lock()
		ips = append(ips, ip)
		mu.Unlock()
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 100) // Ограничиваем количество одновременных подключений

	// 1. Проверяем localhost (для теста на 1 ПК)
	wg.Add(1)
	go func() {
		defer wg.Done()
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		if conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
			conn.Close()
			add("127.0.0.1")
		}
	}()

	// 2. Собираем IP-адреса всех активных интерфейсов (LAN, ZeroTier, Radmin)
	ifaces, _ := net.Interfaces()
	seen := make(map[string]bool)
	var subnets []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		name := strings.ToLower(iface.Name)
		// Пропускаем Docker, виртуалки и прочий мусор
		if strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "br-") ||
			strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "cni") ||
			strings.HasPrefix(name, "flannel") || strings.HasPrefix(name, "virbr") {
			continue
		}

		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ipv4 := ipnet.IP.To4()
			if ipv4 == nil {
				continue
			}

			// Берем первые три октета (например, 192.168.1.)
			sub := fmt.Sprintf("%d.%d.%d.", ipv4[0], ipv4[1], ipv4[2])
			if seen[sub] {
				continue
			}
			seen[sub] = true
			subnets = append(subnets, sub)
		}
	}

	// 3. Параллельно сканируем подсети
	for _, subnet := range subnets {
		for i := 1; i <= 254; i++ {
			wg.Add(1)
			go func(ip string) {
				defer wg.Done()
				sem <- struct{}{}        // Занимаем слот
				defer func() { <-sem }() // Освобождаем слот

				addr := fmt.Sprintf("%s:%d", ip, port)
				conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
				if err == nil {
					conn.Close()
					add(ip)
				}
			}(fmt.Sprintf("%s%d", subnet, i))
		}
	}

	wg.Wait()
	return ips
}
