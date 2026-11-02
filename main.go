package main

import (
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"sqwave-mp/internal/net"
	"sqwave-mp/internal/scenes"
)

func main() {
	net.RegisterGobTypes()
	m := &scenes.Manager{}
	m.Set(scenes.NewMainMenu(m))

	ebiten.SetWindowSize(800, 600)
	ebiten.SetWindowTitle("Sqwave MP")
	if err := ebiten.RunGame(m); err != nil {
		log.Fatal(err)
	}
}
