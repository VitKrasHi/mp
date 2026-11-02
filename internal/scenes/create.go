package scenes

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"sqwave-mp/internal/net"
)

type CreateMenu struct {
	m        *Manager
	gameName string
	max      int
	selected int // 0=name,1=max,2=create,3=back
	errMsg   string
}

func NewCreateMenu(m *Manager) *CreateMenu {
	return &CreateMenu{m: m, gameName: "My Game", max: 4}
}

func (s *CreateMenu) Update() error {
	if s.selected == 0 {
		for _, r := range ebiten.AppendInputChars(nil) {
			if len(s.gameName) < 24 {
				s.gameName += string(r)
			}
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && len(s.gameName) > 0 {
			s.gameName = s.gameName[:len(s.gameName)-1]
		}
	}
	if s.selected == 1 {
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) && s.max > 1 {
			s.max--
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowRight) && s.max < 8 {
			s.max++
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		s.selected = (s.selected + 1) % 4
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		switch s.selected {
		case 2:
			s.doCreate()
		case 3:
			s.m.Set(NewMainMenu(s.m))
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		s.m.Set(NewMainMenu(s.m))
	}
	return nil
}

func (s *CreateMenu) doCreate() {
	if s.gameName == "" {
		s.gameName = "Unnamed"
	}
	host := net.NewHost(s.gameName, s.max)
	if err := host.Start(); err != nil {
		s.errMsg = "Error: " + err.Error()
		return
	}
	host.AddLocalPlayer("Host")
	s.m.Set(NewGameScene(s.m, host, nil))
}

func (s *CreateMenu) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{16, 20, 30, 255})
	ebitenutil.DebugPrintAt(screen, "CREATE GAME", 350, 60)
	nameCur := ""
	if s.selected == 0 {
		nameCur = "_"
	}
	ebitenutil.DebugPrintAt(screen, "Game name: "+s.gameName+nameCur, 200, 150)
	maxCur := ""
	if s.selected == 1 {
		maxCur = "  < >"
	}
	ebitenutil.DebugPrintAt(screen, "Max players: "+itoa(s.max)+maxCur, 200, 200)
	if s.selected == 2 {
		ebitenutil.DebugPrintAt(screen, "> [Create] <", 350, 280)
	} else {
		ebitenutil.DebugPrintAt(screen, "[Create]", 350, 280)
	}
	if s.selected == 3 {
		ebitenutil.DebugPrintAt(screen, "> [Back] <", 350, 330)
	} else {
		ebitenutil.DebugPrintAt(screen, "[Back]", 350, 330)
	}
	ebitenutil.DebugPrintAt(screen, "Tab to switch, Enter to confirm", 240, 400)
	if s.errMsg != "" {
		ebitenutil.DebugPrintAt(screen, s.errMsg, 200, 450)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	d := []byte{}
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}
