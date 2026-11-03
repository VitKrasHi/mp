package scenes

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"sqwave-mp/internal/net"
)

type JoinMenu struct {
	m        *Manager
	listener *net.Listener
	servers  []net.ServerInfo
	selected int

	mu       sync.Mutex
	scanned  []net.ServerInfo
	scanning bool

	name       string
	editing    bool
	directAddr string
	directMode bool
	errMsg     string
}

// В NewJoinMenu запускаем скан в фоне
func NewJoinMenu(m *Manager) *JoinMenu {
	l, _ := net.NewListener()
	s := &JoinMenu{
		m:        m,
		listener: l,
		name:     "Player",
	}
	// Запускаем фоновое сканирование подсети (на случай если Broadcast заблокирован)
	go s.refresh()
	return s
}

func (s *JoinMenu) Close() {
	if s.listener != nil {
		s.listener.Stop()
	}
}

// refresh запускает сканирование подсети в фоне.
func (s *JoinMenu) refresh() {
	s.mu.Lock()
	s.scanning = true
	s.mu.Unlock()

	var scanned []net.ServerInfo
	for _, ip := range net.ScanSubnet(net.DefaultTCPPort) {
		scanned = append(scanned, net.ServerInfo{
			GameName:       "Direct Host",
			Address:        ip,
			Port:           net.DefaultTCPPort,
			CurrentPlayers: 1,
			MaxPlayers:     4,
			LastSeen:       time.Now(),
		})
	}

	s.mu.Lock()
	s.scanned = scanned
	s.scanning = false
	s.mu.Unlock()
}

func (s *JoinMenu) Update() error {
	// Объединяем multicast + результаты скана
	var all []net.ServerInfo
	seen := make(map[string]bool)

	// 1. Результаты от Broadcast (живой эфир)
	if s.listener != nil {
		for _, srv := range s.listener.Servers() {
			key := fmt.Sprintf("%s:%d", srv.Address, srv.Port)
			if !seen[key] {
				all = append(all, srv)
				seen[key] = true
			}
		}
	}

	// 2. Результаты от Сканирования подсети
	s.mu.Lock()
	for _, srv := range s.scanned {
		key := fmt.Sprintf("%s:%d", srv.Address, srv.Port)
		if !seen[key] {
			all = append(all, srv)
			seen[key] = true
		}
	}
	scanning := s.scanning
	s.mu.Unlock()

	s.servers = all
	if s.selected >= len(s.servers) {
		s.selected = len(s.servers) - 1
		if s.selected < 0 {
			s.selected = 0
		}
	}

	if s.editing {
		for _, r := range ebiten.AppendInputChars(nil) {
			s.name += string(r)
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && len(s.name) > 0 {
			s.name = s.name[:len(s.name)-1]
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
			s.editing = false
		}
		return nil
	}
	if s.directMode {
		for _, r := range ebiten.AppendInputChars(nil) {
			s.directAddr += string(r)
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && len(s.directAddr) > 0 {
			s.directAddr = s.directAddr[:len(s.directAddr)-1]
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
			s.connectDirect()
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
			s.directMode = false
		}
		return nil
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) && s.selected > 0 {
		s.selected--
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) && s.selected < len(s.servers)-1 {
		s.selected++
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyN) {
		s.editing = true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyD) {
		s.directMode = true
		s.directAddr = ""
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		if !scanning {
			go s.refresh()
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		s.doJoin()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		s.m.Set(NewMainMenu(s.m))
	}

	// Кнопки мышью
	mx, my := ebiten.CursorPosition()
	btnY := 490.0
	for i := 0; i < 3; i++ {
		bx := 150.0 + float64(i)*180
		if float64(mx) >= bx && float64(mx) <= bx+150 &&
			float64(my) >= btnY && float64(my) <= btnY+45 {
			if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
				switch i {
				case 0:
					s.doJoin()
				case 1:
					if !scanning {
						go s.refresh()
					}
				case 2:
					s.m.Set(NewMainMenu(s.m))
				}
			}
		}
	}
	return nil
}

func (s *JoinMenu) doJoin() {
	if s.selected >= len(s.servers) {
		s.errMsg = "No server selected"
		return
	}
	srv := s.servers[s.selected]
	client := net.NewClient()
	if err := client.Connect(srv.Address, srv.Port, s.name); err != nil {
		s.errMsg = "Connect: " + err.Error()
		return
	}
	if s.listener != nil {
		s.listener.Stop()
	}
	s.m.Set(NewGameScene(s.m, nil, client))
}

func (s *JoinMenu) connectDirect() {
	parts := strings.Split(s.directAddr, ":")
	if len(parts) != 2 {
		s.errMsg = "Format: IP:PORT"
		return
	}
	port, err := strconv.Atoi(parts[1])
	if err != nil {
		s.errMsg = "Bad port"
		return
	}
	client := net.NewClient()
	if err := client.Connect(parts[0], port, s.name); err != nil {
		s.errMsg = "Connect: " + err.Error()
		return
	}
	if s.listener != nil {
		s.listener.Stop()
	}
	s.m.Set(NewGameScene(s.m, nil, client))
}

func (s *JoinMenu) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{16, 20, 30, 255})
	ebitenutil.DebugPrintAt(screen, "FIND SERVERS", 350, 30)

	nameHint := ""
	if s.editing {
		nameHint = "_"
	}
	ebitenutil.DebugPrintAt(screen, "[N] Name: "+s.name+nameHint, 50, 60)

	directHint := ""
	if s.directMode {
		directHint = "_"
	}
	ebitenutil.DebugPrintAt(screen, "[D] Direct: "+s.directAddr+directHint, 50, 80)

	vector.DrawFilledRect(screen, 50, 110, 700, 350, color.RGBA{25, 30, 42, 255}, false)

	s.mu.Lock()
	scanning := s.scanning
	s.mu.Unlock()

	if scanning && len(s.servers) == 0 {
		ebitenutil.DebugPrintAt(screen, "Scanning subnet...", 300, 270)
	} else if !scanning && len(s.servers) == 0 {
		ebitenutil.DebugPrintAt(screen, "No servers found. Press [R] to rescan.", 240, 270)
	}

	for i, srv := range s.servers {
		y := 120 + i*50
		c := color.RGBA{40, 60, 90, 255}
		if i == s.selected {
			c = color.RGBA{80, 140, 200, 255}
		}
		vector.DrawFilledRect(screen, 60, float32(y), 680, 44, c, false)
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%s | %s:%d | %d/%d",
			srv.GameName, srv.Address, srv.Port,
			srv.CurrentPlayers, srv.MaxPlayers), 70, y+14)
	}

	for i, b := range []string{"Join", "Refresh", "Back"} {
		bx := 150.0 + float64(i)*180
		c := color.RGBA{40, 60, 90, 255}
		if i == 0 && s.selected < len(s.servers) {
			c = color.RGBA{80, 140, 200, 255}
		}
		vector.DrawFilledRect(screen, float32(bx), 490, 150, 45, c, false)
		ebitenutil.DebugPrintAt(screen, b, int(bx)+55, 505)
	}

	if s.errMsg != "" {
		ebitenutil.DebugPrintAt(screen, s.errMsg, 50, 560)
	}
}
