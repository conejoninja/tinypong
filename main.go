package main

import (
	"machine"
	"time"

	"image/color"

	"math/rand"

	"github.com/conejoninja/tinyfont"
	"tinygo.org/x/drivers/ds3231"
	"tinygo.org/x/drivers/hub75"
)

const (
	IDLE   Status = 0
	PLAY   Status = 1
	WINNER Status = 2
)

type Status uint8

type Game struct {
	display hub75.Device
	colors  []color.RGBA
	rtc     ds3231.Device
	dt      time.Time
	timeStr []byte
	players [2]Player
	ball    Ball
	status  Status
}

type Player struct {
	pin     machine.ADC
	y       int16
	lastY   int16
	targetY float32
	score   uint8
}

type Ball struct {
	x, y, vx, vy float32
}

func main() {
	var game Game
	var err error
	// SPI for the hub75
	machine.SPI0.Configure(machine.SPIConfig{
		SCK:       machine.SPI0_SCK_PIN,
		MOSI:      machine.SPI0_MOSI_PIN,
		MISO:      machine.SPI0_MISO_PIN,
		Frequency: 8000000,
		Mode:      0})
	// I2C for the RTC DS3231
	machine.I2C0.Configure(machine.I2CConfig{})

	game.display = hub75.New(machine.SPI0, machine.D6, machine.D5, machine.D8, machine.D10, machine.D9, machine.D7)
	game.display.Configure(hub75.Config{
		Width:      64,
		Height:     32,
		RowPattern: 16,
		ColorDepth: 2,
		FastUpdate: true,
	})
	game.display.SetBrightness(100)
	game.display.ClearDisplay()

	game.rtc = ds3231.New(machine.I2C0)
	game.rtc.Configure()

	machine.InitADC()
	game.players[0].pin = machine.ADC{machine.A1}
	game.players[0].pin.Configure()

	game.players[1].pin = machine.ADC{machine.A2}
	game.players[1].pin.Configure()

	// Check if the RTC is working properly
	valid := game.rtc.IsTimeValid()
	if !valid {
		date := time.Date(2019, 12, 13, 20, 16, 22, 0, time.UTC)
		game.rtc.SetTime(date)
	}

	running := game.rtc.IsRunning()
	if !running {
		err := game.rtc.SetRunning(true)
		if err != nil {
			println("Error configuring RTC")
		}
	}

	game.colors = []color.RGBA{
		{255, 0, 0, 255},
		{255, 255, 0, 255},
		{0, 255, 0, 255},
		{0, 255, 255, 255},
		{0, 0, 255, 255},
		{255, 0, 255, 255},
		{255, 255, 255, 255},
		{0, 0, 0, 255},
	}

	game.timeStr = make([]byte, 2)
	then := time.Now()
	game.status = IDLE

	for {
		switch game.status {
		case PLAY:
			gameStopped := uint8(0)
			k := uint8(0)

			game.newRound(0)
			gameStopped = 0

			for game.status == PLAY {
				if time.Since(then).Nanoseconds() > 8000000 {
					then = time.Now()
					game.display.ClearDisplay()

					if gameStopped < 20 {
						gameStopped++
					} else {
						game.ball.x += game.ball.vx
						game.ball.y += game.ball.vy

						for k = 0; k < 2; k++ {
							if (k == 1 && game.ball.x >= 61) || (k == 0 && game.ball.x <= 1) {
								if int16(game.ball.y) < game.players[k].y-1 || int16(game.ball.y) > game.players[k].y+7 {
									game.newRound((k + 1) % 2)
									gameStopped = 0
								} else {
									game.ball.vx = -game.ball.vx
									game.ball.vy += (game.ball.y - float32(game.players[k].y) - 3) * 0.2
									if game.ball.vy > 2.5 {
										game.ball.vy = 2.5
									}
									if game.ball.vy < -2.5 {
										game.ball.vy = -2.5
									}
								}
							}
						}

						if game.ball.y >= 30 || game.ball.y <= 0 {
							game.ball.vy = -game.ball.vy
						}

						if game.ball.y < 0 {
							game.ball.y = 0
						}
						if game.ball.y > 30 {
							game.ball.y = 30
						}
					}

					for k = 0; k < 2; k++ {
						game.players[k].y = int16((65535-game.players[k].pin.Get())/2048) - 4
						if game.players[k].y < 0 {
							game.players[k].y = 0
						}
						if game.players[k].y > 24 {
							game.players[k].y = 24
						}
					}

					// show stuff on the display
					game.drawNet()
					game.drawPlayers()
					game.drawBall()
					game.updateScore(game.players[0].score, game.players[1].score, false)
				}
				game.display.Display()
			}
			break
		case WINNER:
			game.display.ClearDisplay()
			game.status = IDLE
			then = time.Now()
			if game.players[0].score >= 50 {
				tinyfont.WriteLine(&game.display, &tinyfont.TomThumb, 10, 12, []byte("PLAYER 1"), game.colors[2])
			} else {
				tinyfont.WriteLine(&game.display, &tinyfont.TomThumb, 10, 12, []byte("PLAYER 2"), game.colors[2])
			}
			tinyfont.WriteLine(&game.display, &tinyfont.TomThumb, 30, 24, []byte("WINS"), game.colors[3])
			game.players[0].score = 0
			game.players[1].score = 0
			game.players[0].lastY = int16(game.players[0].pin.Get() / 16384)
			game.players[1].lastY = int16(game.players[1].pin.Get() / 16384)
			for {
				if time.Since(then).Seconds() > 5 {
					break
				}
				game.display.Display()
			}

			break
		case IDLE:
			game.dt, err = game.rtc.ReadTime()
			hour := game.dt.Hour()
			minute := game.dt.Minute()
			game.updateScore(uint8(hour), uint8(minute), true)

			game.ball.x = float32(31)
			game.ball.y = rand.Float32()*16 + 8
			game.players[0].targetY = game.ball.y
			game.players[1].targetY = game.ball.y
			game.players[0].y = int16(8)
			game.players[1].y = int16(18)
			game.ball.vx = float32(1)
			game.ball.vy = float32(0.5)
			if game.dt.Second() > 29 {
				game.ball.vy = -0.5
			}

			playerLoss := int8(0)
			gameStopped := uint8(0)

			for game.status == IDLE {
				if time.Since(then).Nanoseconds() > 8000000 {
					then = time.Now()
					game.display.ClearDisplay()

					if gameStopped < 20 {
						gameStopped++
					} else {

						game.ball.x += game.ball.vx
						game.ball.y += game.ball.vy

						if (game.ball.x >= 60 && playerLoss != 1) || (game.ball.x <= 2 && playerLoss != -1) {
							game.ball.vx = -game.ball.vx
							tmp := rand.Int31n(4) // perform a random, last second flick to inflict effect on the ball
							if tmp > 0 {
								tmp = rand.Int31n(2)
								if tmp == 0 {
									if game.ball.vy > 0 && game.ball.vy < 2.5 {
										game.ball.vy += 0.2
									} else if game.ball.vy < 0 && game.ball.vy > -2.5 {
										game.ball.vy -= 0.2
									}
									if game.ball.x >= 60 {
										game.players[1].targetY += 1 + 3*rand.Float32()
									} else {
										game.players[0].targetY += 1 + 3*rand.Float32()
									}
								} else {
									if game.ball.vy > 0.5 {
										game.ball.vy -= 0.2
									} else if game.ball.vy < -0.5 {
										game.ball.vy += 0.2
									}
									if game.ball.x >= 60 {
										game.players[1].targetY -= 1 + 3*rand.Float32()
									} else {
										game.players[0].targetY -= 1 + 3*rand.Float32()
									}
								}
								if game.players[0].targetY < 0 {
									game.players[0].targetY = 0
								}
								if game.players[0].targetY > 24 {
									game.players[0].targetY = 24
								}
								if game.players[1].targetY < 0 {
									game.players[1].targetY = 0
								}
								if game.players[1].targetY > 24 {
									game.players[1].targetY = 24
								}
							}
						} else if (game.ball.x > 62 && playerLoss == 1) || (game.ball.x < 0 && playerLoss == -1) {
							// RESET GAME
							game.ball.x = float32(31)
							game.ball.y = rand.Float32()*16 + 8
							game.ball.vx = float32(1)
							game.ball.vy = float32(0.5)
							if rand.Int31n(2) == 0 {
								game.ball.vy = -0.5
							}
							hour = game.dt.Hour()
							minute = game.dt.Minute()
							game.updateScore(uint8(hour), uint8(minute), true)
							playerLoss = 0
							gameStopped = 0
						}
						if game.ball.y >= 30 || game.ball.y <= 0 {
							game.ball.vy = -game.ball.vy
						}

						// when the ball is on the other side of the court, move the player "randomly" to simulate an AI
						if game.ball.x == float32(40+rand.Int31n(13)) {
							game.players[0].targetY = game.ball.y - 3
							if game.players[0].targetY < 0 {
								game.players[0].targetY = 0
							}
							if game.players[0].targetY > 24 {
								game.players[0].targetY = 24
							}
						}
						if game.ball.x == float32(8+rand.Int31n(13)) {
							game.players[1].targetY = game.ball.y - 3
							if game.players[1].targetY < 0 {
								game.players[1].targetY = 0
							}
							if game.players[1].targetY > 24 {
								game.players[1].targetY = 24
							}
						}

						if int16(game.players[0].targetY) > game.players[0].y {
							game.players[0].y++
						} else if int16(game.players[0].targetY) < game.players[0].y {
							game.players[0].y--
						}

						if int16(game.players[1].targetY) > game.players[1].y {
							game.players[1].y++
						} else if int16(game.players[1].targetY) < game.players[1].y {
							game.players[1].y--
						}

						// If the ball is in the middle, check if we need to lose and calculate the endpoint to avoid/hit the ball
						if game.ball.x == 32 {

							game.dt, err = game.rtc.ReadTime()
							if err != nil {
								println("Error reading date:", err)
								return
							}
							if minute != game.dt.Minute() && playerLoss == 0 { // needs to change one or the other
								if game.dt.Minute() == 0 { // need to change hour
									playerLoss = 1
								} else { // need to change the minute
									playerLoss = -1
								}
							}

							if game.ball.vx < 0 { // moving to the left
								game.players[0].targetY = calculateEndPoint(game.ball.x, game.ball.y, game.ball.vx, game.ball.vy, playerLoss != -1) - 3
								if playerLoss == -1 { // we need to lose
									if game.players[0].targetY < 16 {
										game.players[0].targetY = 19 + 5*rand.Float32()
									} else {
										game.players[0].targetY = 5 * rand.Float32()
									}
								}
								if game.players[0].targetY < 0 {
									game.players[0].targetY = 0
								}
								if game.players[0].targetY > 24 {
									game.players[0].targetY = 24
								}
							}
							if game.ball.vx > 0 { // moving to the right
								game.players[1].targetY = calculateEndPoint(game.ball.x, game.ball.y, game.ball.vx, game.ball.vy, playerLoss != 1) - 3
								if playerLoss == 1 { // we need to lose
									if game.players[1].targetY < 16 {
										game.players[1].targetY = 19 + 5*rand.Float32()
									} else {
										game.players[1].targetY = 5 * rand.Float32()
									}
								}
								if game.players[1].targetY < 0 {
									game.players[1].targetY = 0
								}
								if game.players[1].targetY > 24 {
									game.players[1].targetY = 24
								}
							}
						}

						if game.ball.y < 0 {
							game.ball.y = 0
						}
						if game.ball.y > 30 {
							game.ball.y = 30
						}
					}

					if game.players[0].lastY != int16(game.players[0].pin.Get()/16384) ||
						game.players[1].lastY != int16(game.players[1].pin.Get()/16384) {
						game.status = PLAY
					}
					game.players[0].lastY = int16(game.players[0].pin.Get() / 16384)
					game.players[1].lastY = int16(game.players[1].pin.Get() / 16384)

					// show stuff on the display
					game.drawNet()
					game.drawPlayers()
					game.drawBall()
					game.updateScore(uint8(hour), uint8(minute), true)
				}
				game.display.Display()
			}
			break
		}
	}
}

func (g *Game) newRound(winner uint8) {
	g.ball.x = float32(31)
	g.ball.y = rand.Float32()*16 + 8
	g.ball.vy = rand.Float32()*1 - 0.5
	if winner == 0 {
		g.ball.vx = float32(-1)
	} else {
		g.ball.vx = float32(1)
	}
	g.players[winner].score++
	if g.players[winner].score > 50 {
		g.status = WINNER
	}
}

func (g *Game) drawNet() {
	for i := int16(1); i < 32; i += 2 {
		g.display.SetPixel(31, i, g.colors[6])
	}
}

func (g *Game) drawPlayers() {
	for k := int16(0); k < 2; k++ {
		for i := int16(0); i < 2; i++ {
			for j := int16(0); j < 8; j++ {
				g.display.SetPixel((k*62)+i, g.players[k].y+j, g.colors[3])
			}
		}
	}
}

func (g *Game) drawPlayer(x int16, y int16) {
	for i := int16(0); i < 2; i++ {
		for j := int16(0); j < 8; j++ {
			g.display.SetPixel(x+i, y+j, g.colors[3])
		}
	}
}

func (g *Game) drawBall() {
	g.display.SetPixel(int16(g.ball.x), int16(g.ball.y), g.colors[1])
	g.display.SetPixel(int16(g.ball.x)+1, int16(g.ball.y), g.colors[1])
	g.display.SetPixel(int16(g.ball.x), int16(g.ball.y)+1, g.colors[1])
	g.display.SetPixel(int16(g.ball.x)+1, int16(g.ball.y)+1, g.colors[1])
}

func calculateEndPoint(x float32, y float32, vx float32, vy float32, hit bool) (ty float32) {
	for {
		x += vx
		y += vy
		if hit {
			if x >= 60 || x <= 2 {
				return y
			}
		} else {
			if x >= 62 || x <= 0 {
				return y
			}
		}
		if y >= 30 || y <= 0 {
			vy = -vy
		}
	}
}

func (g *Game) updateScore(scoreLeft uint8, scoreRight uint8, showTime bool) {
	g.timeStr[1] = 48 + (scoreLeft % 10)
	if scoreLeft > 9 {
		g.timeStr[0] = 48 + (scoreLeft / 10)
	} else {
		g.timeStr[0] = 32
	}
	tinyfont.WriteLine(&g.display, &tinyfont.TomThumb, 23, 5, g.timeStr, g.colors[6])

	g.timeStr[1] = 48 + (scoreRight % 10)
	if scoreRight > 9 {
		g.timeStr[0] = 48 + (scoreRight / 10)
	} else {
		if showTime {
			g.timeStr[0] = 48
		} else {
			g.timeStr[0] = 32
		}
	}
	tinyfont.WriteLine(&g.display, &tinyfont.TomThumb, 33, 5, g.timeStr, g.colors[6])
}
