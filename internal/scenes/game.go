package scenes

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"sqwave-mp/internal/net"
)

type GameScene struct {
	m      *Manager
	host   *net.Host
	client *net.Client
}

func NewGameScene(m *Manager, host *net.Host, client *net.Client) *GameScene {
	g := &GameScene{m: m, host: host, client: client}
	if client != nil {
		client.OnDisconnect = func() { g.m.Set(NewMainMenu(g.m)) }
	}
	return g
}

func (g *GameScene) Close() {
	if g.host != nil {
		g.host.Close()
	}
	if g.client != nil {
		g.client.Close()
	}
}

func (g *GameScene) Update() error {
	dx, dy := 0.0, 0.0
	if ebiten.IsKeyPressed(ebiten.KeyW) {
		dy -= 1
	}
	if ebiten.IsKeyPressed(ebiten.KeyS) {
		dy += 1
	}
	if ebiten.IsKeyPressed(ebiten.KeyA) {
		dx -= 1
	}
	if ebiten.IsKeyPressed(ebiten.KeyD) {
		dx += 1
	}
	// Нормализуем диагональ, чтобы скорость была одинаковой
	if l := math.Hypot(dx, dy); l > 0 {
		dx /= l
		dy /= l
	}

	// Отправляем КАЖДЫЙ кадр — это направление, а не дельта,
	// поэтому частота отправки больше не влияет на скорость.
	if g.host != nil {
		g.host.SendInput(dx, dy)
	}
	if g.client != nil {
		g.client.SendInput(dx, dy)
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.m.Set(NewMainMenu(g.m))
	}
	return nil
}

func (g *GameScene) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{30, 30, 40, 255})

	var players []net.PlayerState
	var myID uint32
	var role string
	if g.host != nil {
		players = g.host.Players()
		myID = g.host.MyID()
		role = "HOST"
	} else if g.client != nil {
		players = g.client.Players()
		myID = g.client.MyID()
		role = "CLIENT"
	}

	for _, p := range players {
		c := color.RGBA{100, 150, 255, 255}
		if p.ID == myID {
			c = color.RGBA{100, 255, 150, 255}
		}
		vector.DrawFilledRect(screen, float32(p.X-16), float32(p.Y-16), 32, 32, c, false)
		ebitenutil.DebugPrintAt(screen, p.Name, int(p.X)-20, int(p.Y)-30)
	}

	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%s | You: %s (ID %d) | Players: %d",
		role, selfName(players, myID), myID, len(players)), 10, 10)
	ebitenutil.DebugPrintAt(screen, "WASD to move — ESC to leave", 10, 28)
}

func selfName(ps []net.PlayerState, id uint32) string {
	for _, p := range ps {
		if p.ID == id {
			return p.Name
		}
	}
	return "?"
}
