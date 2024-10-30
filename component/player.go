package component

import (
	"github.com/yohamta/donburi"
)

type PlayerData struct {
	Name   string
	Health float64
	Id     int
	// Profile *ebiten.Image
}

var Player = donburi.NewComponentType[PlayerData]()
