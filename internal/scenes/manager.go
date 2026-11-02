package scenes

import "github.com/hajimehoshi/ebiten/v2"

type Scene interface {
	Update() error
	Draw(screen *ebiten.Image)
}

type Closer interface{ Close() }

type Manager struct{ current Scene }

func (m *Manager) Set(s Scene) {
	if c, ok := m.current.(Closer); ok {
		c.Close()
	}
	m.current = s
}
func (m *Manager) Update() error {
	if m.current != nil {
		return m.current.Update()
	}
	return nil
}
func (m *Manager) Draw(screen *ebiten.Image) {
	if m.current != nil {
		m.current.Draw(screen)
	}
}
func (m *Manager) Layout(w, h int) (int, int) { return 800, 600 }
