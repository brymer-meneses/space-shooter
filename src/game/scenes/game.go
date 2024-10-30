package scenes

import (
	"context"
	"log"
	"space-shooter/assets"
	"space-shooter/config"
	"space-shooter/game/component"
	"space-shooter/rpc"
	"space-shooter/server/messages"

	"github.com/coder/websocket"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/vmihailenco/msgpack/v5"
	"github.com/yohamta/donburi"
	"github.com/yohamta/donburi/ecs"
	"github.com/yohamta/donburi/filter"
)

type GameScene struct {
	assetManager *assets.AssetManager
	ecs          *ecs.ECS
	connection   *websocket.Conn
	playerId     messages.PlayerId
}

func NewGameScene(config *config.AppConfig, assetManager *assets.AssetManager, playerId messages.PlayerId) *GameScene {
	ctx := context.Background()
	connection, _, err := websocket.Dial(ctx, config.ServerWebsocketURL, nil)
	if err != nil {
		log.Fatalf("Failed to connect to the game server at %s\n", config.ServerWebsocketURL)
	}

	var message rpc.BaseMessage
	if err := rpc.ReceiveMessage(ctx, connection, &message); err != nil {
		log.Fatal(err)
	}

	var establishConnection messages.EstablishConnection
	if err := msgpack.Unmarshal(message.Payload, &establishConnection); err != nil {
		log.Fatal(err)
	}

	scene := &GameScene{
		assetManager: assetManager,
		playerId:     establishConnection.PlayerId,
		connection:   connection,
	}

	scene.ecs =
		ecs.NewECS(donburi.NewWorld()).
			AddRenderer(0, scene.drawEnvironment).
			AddSystem(scene.movePlayer)

	scene.createPlayer(establishConnection.PlayerId, &establishConnection.Position)

	for _, enemyData := range establishConnection.EnemyData {
		scene.createPlayer(messages.PlayerId(enemyData.PlayerId), &enemyData.Position)
	}

	go scene.receiveServerUpdates()

	return scene
}

func (self *GameScene) Draw(screen *ebiten.Image) {
	screen.Clear()
	self.assetManager.Background.Render(screen)

	self.ecs.DrawLayer(0, screen)
	self.ecs.Draw(screen)
}

func (self *GameScene) Update() {
	self.ecs.Update()
}

func (self *GameScene) createPlayer(playerId messages.PlayerId, position *component.PositionData) {
	world := self.ecs.World
	entity := world.Create(component.Player, component.Position, component.Sprite)
	player := world.Entry(entity)

	donburi.SetValue(
		player,
		component.Player,
		component.PlayerData{
			Name: "Player One",
			Id:   int(playerId),
		},
	)

	donburi.SetValue(
		player,
		component.Position,
		*position,
	)

	donburi.SetValue(
		player,
		component.Sprite,
		component.SpriteData{Image: self.assetManager.Ships[playerId].Image},
	)

}

// Draws the game environment.
func (self *GameScene) drawEnvironment(ecs *ecs.ECS, screen *ebiten.Image) {
	query := donburi.NewQuery(filter.Contains(component.Player, component.Position, component.Sprite))

	for player := range query.Iter(self.ecs.World) {
		sprite := component.Sprite.Get(player)
		position := component.Position.Get(player)

		op := &ebiten.DrawImageOptions{}

		x_0 := float64(sprite.Image.Bounds().Dx()) / 2
		y_0 := float64(sprite.Image.Bounds().Dy()) / 2

		op.GeoM.Translate(-x_0, -y_0)

		op.GeoM.Rotate(position.Angle)
		op.GeoM.Scale(4, 4)
		op.GeoM.Translate(position.X, position.Y)

		screen.DrawImage(sprite.Image, op)
	}
}

// Handles the movement of the player and sends it to the server.
func (self *GameScene) movePlayer(ecs *ecs.ECS) {

	updatePosition := func(positionData *component.PositionData) {
		message := rpc.NewBaseMessage(
			messages.UpdatePosition{
				PlayerId: self.playerId,
				Position: *positionData,
			})

		marshaled, _ := msgpack.Marshal(message)
		self.connection.Write(context.Background(), websocket.MessageBinary, marshaled)
	}

	query := donburi.NewQuery(filter.Contains(component.Player, component.Position, component.Sprite))

	for player := range query.Iter(ecs.World) {
		if int(self.playerId) != component.Player.GetValue(player).Id {
			continue
		}

		positionData := component.Position.Get(player)
		if ebiten.IsKeyPressed(ebiten.KeyW) {
			positionData.Forward()
			updatePosition(positionData)
		}
		if ebiten.IsKeyPressed(ebiten.KeyA) {
			positionData.RotateClockwise()
			updatePosition(positionData)
		}
		if ebiten.IsKeyPressed(ebiten.KeyD) {
			positionData.RotateCounterClockwise()
			updatePosition(positionData)
		}

	}
}

// Receives information from the server and updates the game state accordingly.
func (self *GameScene) receiveServerUpdates() {
	for {
		var message rpc.BaseMessage
		if err := rpc.ReceiveMessage(context.Background(), self.connection, &message); err != nil {
			continue
		}

		switch message.MessageType {
		case "UpdatePosition":
			{
				var updatePosition messages.UpdatePosition
				if err := msgpack.Unmarshal(message.Payload, &updatePosition); err != nil {
					continue
				}

				entry := findCorrespondingPlayer(self.ecs, updatePosition.PlayerId)
				if entry != nil {
					component.Position.SetValue(entry, updatePosition.Position)
				}
			}
		case "PlayerConnected":
			{
				var playerConnected messages.PlayerConnected
				if err := msgpack.Unmarshal(message.Payload, &playerConnected); err != nil {
					continue
				}

				self.createPlayer(playerConnected.PlayerId, &playerConnected.Position)
			}
		}
	}

}

func findCorrespondingPlayer(ecs *ecs.ECS, playerId messages.PlayerId) *donburi.Entry {
	query := donburi.NewQuery(filter.Contains(component.Player))
	for player := range query.Iter(ecs.World) {
		if int(playerId) == component.Player.GetValue(player).Id {
			return player
		}
	}
	return nil
}
