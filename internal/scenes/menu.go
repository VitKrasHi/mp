package scenes

import (
	"image/color"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type MainMenu struct {
	m        *Manager
	selected int
}

var mainButtons = []string{"Create Game", "Join Game", "Exit"}

func NewMainMenu(m *Manager) *MainMenu { return &MainMenu{m: m} }

func (s *MainMenu) Update() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
		s.selected = (s.selected - 1 + len(mainButtons)) % len(mainButtons)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
		s.selected = (s.selected + 1) % len(mainButtons)
	}
	mx, my := ebiten.CursorPosition()
	for i := range mainButtons {
		x, y, w, h := buttonRect(i, 200)
		if float64(mx) >= x && float64(mx) <= x+w && float64(my) >= y && float64(my) <= y+h {
			s.selected = i
			if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
				s.activate(i)
			}
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		s.activate(s.selected)
	}
	return nil
}

func (s *MainMenu) activate(i int) {
	switch i {
	case 0:
		s.m.Set(NewCreateMenu(s.m))
	case 1:
		s.m.Set(NewJoinMenu(s.m))
	case 2:
		os.Exit(0)
	}
}

func (s *MainMenu) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{16, 20, 30, 255})
	ebitenutil.DebugPrintAt(screen, "SQWAVE  —  MULTIPLAYER", 300, 100)
	ebitenutil.DebugPrintAt(screen, "Up/Down to select, Enter to confirm", 260, 130)
	for i, b := range mainButtons {
		x, y, w, h := buttonRect(i, 200)
		c := color.RGBA{40, 60, 90, 255}
		if i == s.selected {
			c = color.RGBA{80, 140, 200, 255}
		}
		vector.DrawFilledRect(screen, float32(x), float32(y), float32(w), float32(h), c, false)
		ebitenutil.DebugPrintAt(screen, b, int(x)+50, int(y)+15)
	}
}

func buttonRect(i, baseY int) (x, y, w, h float64) {
	return 300, float64(baseY + i*60), 200, 45
}
