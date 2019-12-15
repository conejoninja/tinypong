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

var display hub75.Device
var colors []color.RGBA
var rtc ds3231.Device
var dt time.Time
var err error
var timeStr []byte

func main() {
	// SPI for the hub75
	machine.SPI0.Configure(machine.SPIConfig{
		Frequency: 8000000,
		Mode:      0})
	// I2C for the RTC DS3231
	machine.I2C0.Configure(machine.I2CConfig{})

	display = hub75.New(machine.SPI0, machine.D6, machine.D5, machine.D8, machine.D10, machine.D9, machine.D7)
	display.Configure(hub75.Config{
		Width:      64,
		Height:     32,
		RowPattern: 16,
		ColorDepth: 2,
		FastUpdate: true,
	})

	rtc = ds3231.New(machine.I2C0)
	rtc.Configure()

	machine.InitADC()
	player1Pin := machine.ADC{machine.A1}
	player1Pin.Configure()

	player2Pin := machine.ADC{machine.A2}
	player2Pin.Configure()

	// Check if the RTC is working properly
	valid := rtc.IsTimeValid()
	if !valid {
		date := time.Date(2019, 12, 13, 20, 16, 22, 0, time.UTC)
		rtc.SetTime(date)
	}

	running := rtc.IsRunning()
	if !running {
		err := rtc.SetRunning(true)
		if err != nil {
			println("Error configuring RTC")
		}
	}

	colors = []color.RGBA{
		{255, 0, 0, 255},
		{255, 255, 0, 255},
		{0, 255, 0, 255},
		{0, 255, 255, 255},
		{0, 0, 255, 255},
		{255, 0, 255, 255},
		{255, 255, 255, 255},
		{0, 0, 0, 255},
	}

	display.ClearDisplay()
	display.SetBrightness(100)

	timeStr = make([]byte, 2)
	then := time.Now()

	dt, err = rtc.ReadTime()
	hour := dt.Hour()
	minute := dt.Minute()
	updateTime(uint8(hour), uint8(minute))

	ballX := float32(31)
	ballY := rand.Float32()*16 + 8
	leftPlayerTargetY := ballY
	rightPlayerTargetY := ballY
	leftPlayerY := int16(8)
	rightPlayerY := int16(18)
	ballVX := float32(1)
	ballVY := float32(0.5)
	if dt.Second() > 29 {
		ballVY = -0.5
	}

	playerLoss := int8(0)
	gameStopped := uint8(0)

	for {
		if time.Since(then).Nanoseconds() > 8000000 {
			then = time.Now()
			display.ClearDisplay()

			if gameStopped < 20 {
				gameStopped++
			} else {

				ballX += ballVX
				ballY += ballVY

				if (ballX >= 60 && playerLoss != 1) || (ballX <= 2 && playerLoss != -1) {
					ballVX = -ballVX
					tmp := rand.Int31n(4) // perform a random, last second flick to inflict effect on the ball
					if tmp > 0 {
						tmp = rand.Int31n(2)
						if tmp == 0 {
							if ballVY > 0 && ballVY < 2.5 {
								ballVY += 0.2
							} else if ballVY < 0 && ballVY > -2.5 {
								ballVY -= 0.2
							}
							if ballX >= 60 {
								rightPlayerTargetY += 1 + 3*rand.Float32()
							} else {
								leftPlayerTargetY += 1 + 3*rand.Float32()
							}
						} else {
							if ballVY > 0.5 {
								ballVY -= 0.2
							} else if ballVY < -0.5 {
								ballVY += 0.2
							}
							if ballX >= 60 {
								rightPlayerTargetY -= 1 + 3*rand.Float32()
							} else {
								leftPlayerTargetY -= 1 + 3*rand.Float32()
							}
						}
						if leftPlayerTargetY < 0 {
							leftPlayerTargetY = 0
						}
						if leftPlayerTargetY > 24 {
							leftPlayerTargetY = 24
						}
						if rightPlayerTargetY < 0 {
							rightPlayerTargetY = 0
						}
						if rightPlayerTargetY > 24 {
							rightPlayerTargetY = 24
						}
					}
				} else if (ballX > 62 && playerLoss == 1) || (ballX < 0 && playerLoss == -1) {
					// RESET GAME
					ballX = float32(31)
					ballY = rand.Float32()*16 + 8
					ballVX = float32(1)
					ballVY = float32(0.5)
					if rand.Int31n(2) == 0 {
						ballVY = -0.5
					}
					hour = dt.Hour()
					minute = dt.Minute()
					updateTime(uint8(hour), uint8(minute))
					playerLoss = 0
					gameStopped = 0
				}
				if ballY >= 30 || ballY <= 0 {
					ballVY = -ballVY
				}

				// when the ball is on the other side of the court, move the player "randomly" to simulate an AI
				if ballX == float32(40+rand.Int31n(13)) {
					leftPlayerTargetY = ballY - 3
					if leftPlayerTargetY < 0 {
						leftPlayerTargetY = 0
					}
					if leftPlayerTargetY > 24 {
						leftPlayerTargetY = 24
					}
				}
				if ballX == float32(8+rand.Int31n(13)) {
					rightPlayerTargetY = ballY - 3
					if rightPlayerTargetY < 0 {
						rightPlayerTargetY = 0
					}
					if rightPlayerTargetY > 24 {
						rightPlayerTargetY = 24
					}
				}

				if int16(leftPlayerTargetY) > leftPlayerY {
					leftPlayerY++
				} else if int16(leftPlayerTargetY) < leftPlayerY {
					leftPlayerY--
				}

				if int16(rightPlayerTargetY) > rightPlayerY {
					rightPlayerY++
				} else if int16(rightPlayerTargetY) < rightPlayerY {
					rightPlayerY--
				}

				// If the ball is in the middle, check if we need to lose and calculate the endpoint to avoid/hit the ball
				if ballX == 32 {

					dt, err = rtc.ReadTime()
					if err != nil {
						println("Error reading date:", err)
						return
					}
					if minute != dt.Minute() && playerLoss == 0 { // needs to change one or the other
						if dt.Minute() == 0 { // need to change hour
							playerLoss = 1
						} else { // need to change the minute
							playerLoss = -1
						}
					}

					if ballVX < 0 { // moving to the left
						leftPlayerTargetY = calculateEndPoint(ballX, ballY, ballVX, ballVY, playerLoss != -1) - 3
						if playerLoss == -1 { // we need to lose
							if leftPlayerTargetY < 16 {
								leftPlayerTargetY = 19 + 5*rand.Float32()
							} else {
								leftPlayerTargetY = 5 * rand.Float32()
							}
						}
						if leftPlayerTargetY < 0 {
							leftPlayerTargetY = 0
						}
						if leftPlayerTargetY > 24 {
							leftPlayerTargetY = 24
						}
					}
					if ballVX > 0 { // moving to the right
						rightPlayerTargetY = calculateEndPoint(ballX, ballY, ballVX, ballVY, playerLoss != 1) - 3
						if playerLoss == 1 { // we need to lose
							if rightPlayerTargetY < 16 {
								rightPlayerTargetY = 19 + 5*rand.Float32()
							} else {
								rightPlayerTargetY = 5 * rand.Float32()
							}
						}
						if rightPlayerTargetY < 0 {
							rightPlayerTargetY = 0
						}
						if rightPlayerTargetY > 24 {
							rightPlayerTargetY = 24
						}
					}
				}

				if ballY < 0 {
					ballY = 0
				}
				if ballY > 30 {
					ballY = 30
				}
			}

			println(player1Pin.Get(), player2Pin.Get())

			leftPlayerY = int16(player1Pin.Get()/2048) - 4
			rightPlayerY = int16(player2Pin.Get()/2048) - 4
			if leftPlayerY < 0 {
				leftPlayerY = 0
			}
			if leftPlayerY > 24 {
				leftPlayerY = 24
			}
			if rightPlayerY < 0 {
				rightPlayerY = 0
			}
			if rightPlayerY > 24 {
				rightPlayerY = 24
			}

			// show stuff on the display
			drawNet()
			drawPlayer(0, leftPlayerY)
			drawPlayer(62, rightPlayerY)
			drawBall(int16(ballX), int16(ballY))
			updateTime(uint8(hour), uint8(minute))
		}
		display.Display()
	}
}

func drawNet() {
	for i := int16(1); i < 32; i += 2 {
		display.SetPixel(31, i, colors[6])
	}
}

func drawPlayer(x int16, y int16) {
	for i := int16(0); i < 2; i++ {
		for j := int16(0); j < 8; j++ {
			display.SetPixel(x+i, y+j, colors[3])
		}
	}
}

func drawBall(x int16, y int16) {
	display.SetPixel(x, y, colors[1])
	display.SetPixel(x+1, y, colors[1])
	display.SetPixel(x, y+1, colors[1])
	display.SetPixel(x+1, y+1, colors[1])
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

func updateTime(hour uint8, minute uint8) {
	timeStr[1] = 48 + (hour % 10)
	if hour > 9 {
		timeStr[0] = 48 + (hour / 10)
	} else {
		timeStr[0] = 32
	}
	tinyfont.WriteLine(&display, &tinyfont.TomThumb, 23, 5, timeStr, colors[6])

	timeStr[1] = 48 + (minute % 10)
	if minute > 9 {
		timeStr[0] = 48 + (minute / 10)
	} else {
		timeStr[0] = 48
	}
	tinyfont.WriteLine(&display, &tinyfont.TomThumb, 33, 5, timeStr, colors[6])
}
