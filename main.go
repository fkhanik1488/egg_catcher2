package main

import (
	"bytes"
	"database/sql"
	"embed"
	"flag"
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/mp3"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	_ "github.com/lib/pq" // PostgreSQL driver
	"golang.org/x/crypto/bcrypt"

	"image/color"
	_ "image/png"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"strings"
	"time"

	_ "embed"
)

//go:embed avi/*.png
var imageFiles embed.FS

//go:embed music/*.mp3
var audioFiles embed.FS

const (
	screenWidth  = 800
	screenHeight = 600
	wolfWidth    = 50
	wolfHeight   = 50
	basketWidth  = 80 // Increased to expand right
	basketHeight = 60 // Reduced to upper rectangle
	henWidth     = 38 // Increased from 40 to 48
	henHeight    = 38 // Increased from 40 to 48
	eggSize      = 14 // Updated to match width of new egg textures (14x16)
	heartSize    = 30 // Increased to 30 for wider hearts
	buttonWidth  = 200
	buttonHeight = 50
)

var (
	db                *sql.DB
	audioContext      *audio.Context // Глобальная переменная для аудио-контекста
	imgBackgroundMenu *ebiten.Image  // Background for auth and Game Over screen
	imgBackgroundMain *ebiten.Image  // Background for game screen
	imgHen            *ebiten.Image  // Sprite for hens
	imgWolf           *ebiten.Image  // Sprite for wolf (replaces basket)
	imgHeart1         *ebiten.Image  // Sprite for full heart
	imgHeart2         *ebiten.Image  // Sprite for gray (lost) heart
	imgFakeEgg        *ebiten.Image  // Sprite for fake egg
	imgGoldEgg        *ebiten.Image  // Sprite for gold egg
	imgWhiteEgg       *ebiten.Image  // Sprite for white egg
	player            *audio.Player  // Глобальная переменная для управления музыкой
)

type Game struct {
	wolfX, wolfY      float64
	basketY           float64
	hens              [4]Hen
	eggs              []Egg
	level             int
	score             int
	record            int
	lives             int
	gameOver          bool
	showLeaderboard   bool
	playagainButton   Button
	quitButton        Button
	leaderboardButton Button
	playerID          int
	loseHeartPlayer   *audio.Player
	gainHeartPlayer   *audio.Player
	scoreHeartPlayer  *audio.Player
	isMoving          bool // Сохраняем флаг для предотвращения телепортации
	isPaused          bool // Флаг паузы
}

type Hen struct {
	x, y float64
}

type Egg struct {
	x, y        float64
	vx, vy      float64
	phase       string
	transitionX float64
	active      bool
	value       int
}

type Button struct {
	x, y, w, h float64
	label      string
	hovered    bool
}

func (b *Button) IsInside(x, y float64) bool {
	scale := 1.5
	return x >= b.x*scale && x <= (b.x+b.w)*scale && y >= b.y*scale && y <= (b.y+b.h)*scale
}

type Player struct {
	ID        int
	Name      string
	HighScore int
}

type AuthState struct {
	db              *sql.DB
	username        string
	password        string
	authPhase       string
	passwordEntered bool
	isRegister      bool
	loginButton     Button
	regButton       Button
	submitButton    Button
	errorMsg        string
	playerID        int
	done            bool
}

type GameWrapper struct {
	authState        *AuthState
	game             *Game
	loseHeartPlayer  *audio.Player
	gainHeartPlayer  *audio.Player
	scoreHeartPlayer *audio.Player
}

func NewGame(playerID int, loseHeartPlayer, gainHeartPlayer, scoreHeartPlayer *audio.Player) *Game {
	g := &Game{
		wolfX:            screenWidth/2 - wolfWidth/2,
		wolfY:            screenHeight - wolfHeight - 20,
		basketY:          460,
		level:            1,
		score:            0,
		record:           0,
		lives:            3,
		gameOver:         false,
		showLeaderboard:  false,
		playerID:         playerID,
		loseHeartPlayer:  loseHeartPlayer,
		gainHeartPlayer:  gainHeartPlayer,
		scoreHeartPlayer: scoreHeartPlayer,
		isMoving:         false,
		isPaused:         false,
	}
	loadPlayerData(g)
	g.hens[0] = Hen{x: 150, y: 58}
	g.hens[1] = Hen{x: 100, y: 108}
	g.hens[2] = Hen{x: 650, y: 58}
	g.hens[3] = Hen{x: 700, y: 108}
	g.playagainButton = Button{
		x:     screenWidth/3 - buttonWidth - 10,
		y:     screenHeight/3 + 20,
		w:     buttonWidth,
		h:     buttonHeight,
		label: "Play again",
	}
	g.quitButton = Button{
		x:     screenWidth/3 + 10,
		y:     screenHeight/3 + 20,
		w:     buttonWidth,
		h:     buttonHeight,
		label: "Quit",
	}
	g.leaderboardButton = Button{
		x:     screenWidth/3 - buttonWidth/2,
		y:     screenHeight/3 + 80,
		w:     buttonWidth,
		h:     buttonHeight,
		label: "Show Leaderboard",
	}
	return g
}

func (g *Game) spawnEgg() {
	probability := rand.Float64()
	var valueEgg int
	if probability < 0.1 {
		valueEgg = 0 // fake_egg
	} else if probability < 0.15 {
		valueEgg = 1 // white_egg
	} else {
		valueEgg = 2 // gold_egg
	}
	henIndex := rand.Intn(4)
	eggX := g.hens[henIndex].x + henWidth/2 - eggSize/2
	vx, transitionX := 0.0, 0.0
	baseSpeed := 1.0 + 1.0*float64(g.level-1)
	if eggX < screenWidth/2 {
		vx = baseSpeed / math.Sqrt(2)
		transitionX = eggX + 67.5
	} else {
		vx = -baseSpeed / math.Sqrt(2)
		transitionX = eggX - 67.5
	}
	g.eggs = append(g.eggs, Egg{
		x:           eggX,
		y:           g.hens[henIndex].y + float64(henHeight),
		vx:          vx,
		vy:          baseSpeed / math.Sqrt(2),
		phase:       "rolling",
		transitionX: transitionX,
		active:      true,
		value:       valueEgg,
	})
}

func initDB() (*sql.DB, error) {
	// Зашитая строка подключения
	dbURL := "postgres://eggcatcher:fimoz@185.207.1.110:5432/egg_catcher?sslmode=disable"
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}
	// Проверка подключения
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %v", err)
	}
	// Создание таблицы players
	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS players (
            id SERIAL PRIMARY KEY,
            name TEXT NOT NULL UNIQUE,
            high_score INTEGER DEFAULT 0,
            password TEXT NOT NULL
        )
    `)
	if err != nil {
		return nil, fmt.Errorf("failed to create players table: %v", err)
	}
	// Создание таблицы games
	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS games (
            id SERIAL PRIMARY KEY,
            player_id INTEGER NOT NULL,
            score INTEGER NOT NULL,
            lives INTEGER NOT NULL,
            date TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY (player_id) REFERENCES players(id) ON DELETE CASCADE
        )
    `)
	if err != nil {
		return nil, fmt.Errorf("failed to create games table: %v", err)
	}
	return db, nil
}

func authenticate(db *sql.DB, username, password string, isRegister bool) (int, error) {
	if username == "" {
		return 0, fmt.Errorf("username cannot be empty")
	}
	if password == "" && isRegister {
		return 0, fmt.Errorf("password cannot be empty")
	}
	var playerID int
	var exists int
	var storedPassword string
	err := db.QueryRow("SELECT COUNT(*) FROM players WHERE name = $1", username).Scan(&exists)
	if err != nil {
		log.Printf("Failed to check user existence for username '%s': %v", username, err)
		return 0, fmt.Errorf("failed to check user existence: %v", err)
	}

	if isRegister {
		if exists > 0 {
			return 0, fmt.Errorf("username already taken")
		}
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return 0, fmt.Errorf("failed to hash password: %v", err)
		}
		// Используем RETURNING id для получения ID новой записи
		err = db.QueryRow("INSERT INTO players (name, high_score, password) VALUES ($1, 0, $2) RETURNING id", username, string(hashedPassword)).Scan(&playerID)
		if err != nil {
			log.Printf("Failed to insert new player '%s': %v", username, err)
			return 0, fmt.Errorf("failed to insert new player: %v", err)
		}
		log.Printf("Successfully registered new player '%s' with ID %d", username, playerID)
		return playerID, nil
	}

	if exists == 0 {
		return 0, fmt.Errorf("user does not exist")
	}
	err = db.QueryRow("SELECT id, password FROM players WHERE name = $1", username).Scan(&playerID, &storedPassword)
	if err != nil {
		log.Printf("Failed to get player data for '%s': %v", username, err)
		return 0, fmt.Errorf("failed to get player data: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(storedPassword), []byte(password)); err != nil {
		return 0, fmt.Errorf("incorrect password")
	}
	log.Printf("Successfully authenticated player '%s' with ID %d", username, playerID)
	return playerID, nil
}

func loadPlayerData(g *Game) {
	if db == nil {
		return
	}
	var player Player
	err := db.QueryRow("SELECT id, name, high_score FROM players WHERE id = $1", g.playerID).Scan(&player.ID, &player.Name, &player.HighScore)
	if err != nil {
		if err == sql.ErrNoRows {
			_, err = db.Exec("INSERT INTO players (name, high_score, password) VALUES ($1, 0, $2)", "Player1", "")
			if err != nil {
				log.Printf("Error creating default player: %v", err)
				return
			}
			g.score = 0
			g.record = 0
		} else {
			log.Printf("Error loading player data for ID %d: %v", g.playerID, err)
			return
		}
	} else {
		g.record = player.HighScore
	}
}

func saveGameData(g *Game) error {
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec("INSERT INTO games (player_id, score, lives) VALUES ($1, $2, $3)", g.playerID, g.score, g.lives)
	if err != nil {
		log.Printf("Failed to save game data for player ID %d: %v", g.playerID, err)
		return fmt.Errorf("failed to save game data: %v", err)
	}
	var currentHighScore int
	err = db.QueryRow("SELECT high_score FROM players WHERE id = $1", g.playerID).Scan(&currentHighScore) // Fix: Use &currentHighScore
	if err != nil {
		log.Printf("Failed to get current high score for player ID %d: %v", g.playerID, err)
		return fmt.Errorf("failed to get current high score: %v", err)
	}
	if g.score > currentHighScore {
		_, err = db.Exec("UPDATE players SET high_score = $1 WHERE id = $2", g.score, g.playerID)
		if err != nil {
			log.Printf("Failed to update high score for player ID %d: %v", g.playerID, err)
			return fmt.Errorf("failed to update high score: %v", err)
		}
		g.record = g.score
	}
	return nil
}

func loadLeaderboard() []Player {
	if db == nil {
		fmt.Println("Database not initialized")
		return []Player{}
	}
	rows, err := db.Query("SELECT id, name, high_score FROM players ORDER BY high_score DESC LIMIT 5")
	if err != nil {
		log.Printf("Error loading leaderboard: %v", err)
		return []Player{}
	}
	defer rows.Close()

	var leaderboard []Player
	for rows.Next() {
		var p Player
		err := rows.Scan(&p.ID, &p.Name, &p.HighScore)
		if err != nil {
			log.Printf("Error scanning leaderboard row: %v", err)
			continue
		}
		leaderboard = append(leaderboard, p)
	}
	return leaderboard
}

func (a *AuthState) Update() error {
	runes := ebiten.AppendInputChars(nil)
	if a.authPhase == "username" || (a.authPhase == "register" && !a.passwordEntered) {
		for _, r := range runes {
			if len(a.username) < 20 {
				a.username += string(r)
			}
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && len(a.username) > 0 {
			a.username = a.username[:len(a.username)-1]
		}
	} else if a.authPhase == "password" || (a.authPhase == "register" && a.passwordEntered) {
		for _, r := range runes {
			if len(a.password) < 20 {
				a.password += string(r)
			}
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && len(a.password) > 0 {
			a.password = a.password[:len(a.password)-1]
		}
	}

	cx, cy := ebiten.CursorPosition()
	mx, my := float64(cx), float64(cy)
	a.loginButton.hovered = a.loginButton.IsInside(mx, my)
	a.regButton.hovered = a.regButton.IsInside(mx, my)
	a.submitButton.hovered = a.submitButton.IsInside(mx, my)

	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		if a.loginButton.hovered && a.authPhase == "username" {
			a.isRegister = false
			a.errorMsg = ""
			a.username = ""
			a.password = ""
			a.passwordEntered = false
		} else if a.regButton.hovered && a.authPhase == "username" {
			a.isRegister = true
			a.authPhase = "register"
			a.errorMsg = ""
			a.username = ""
			a.password = ""
			a.passwordEntered = false
		} else if a.submitButton.hovered {
			if a.authPhase == "username" && !a.isRegister {
				var exists int
				err := a.db.QueryRow("SELECT COUNT(*) FROM players WHERE name = $1", strings.TrimSpace(a.username)).Scan(&exists)
				if err != nil {
					log.Printf("Failed to check username '%s': %v", strings.TrimSpace(a.username), err)
					a.errorMsg = "Failed to check username"
				} else if exists == 0 {
					a.errorMsg = "User does not exist"
				} else {
					a.authPhase = "password"
					a.errorMsg = ""
					a.password = ""
					a.passwordEntered = false
				}
			} else if a.authPhase == "password" && a.passwordEntered {
				playerID, err := authenticate(a.db, strings.TrimSpace(a.username), strings.TrimSpace(a.password), false)
				if err != nil {
					a.errorMsg = err.Error()
					a.authPhase = "username"
					a.password = ""
					a.passwordEntered = false
				} else {
					a.playerID = playerID
					a.done = true
				}
			} else if a.authPhase == "register" && a.passwordEntered {
				playerID, err := authenticate(a.db, strings.TrimSpace(a.username), strings.TrimSpace(a.password), true)
				if err != nil {
					a.errorMsg = err.Error()
					a.authPhase = "username"
					a.username = ""
					a.password = ""
					a.passwordEntered = false
				} else {
					a.playerID = playerID
					a.done = true
				}
			}
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		if a.authPhase == "username" && !a.isRegister {
			var exists int
			err := a.db.QueryRow("SELECT COUNT(*) FROM players WHERE name = $1", strings.TrimSpace(a.username)).Scan(&exists)
			if err != nil {
				log.Printf("Failed to check username '%s': %v", strings.TrimSpace(a.username), err)
				a.errorMsg = "Failed to check username"
			} else if exists == 0 {
				a.errorMsg = "User does not exist"
			} else {
				a.authPhase = "password"
				a.errorMsg = ""
				a.password = ""
				a.passwordEntered = false
			}
		} else if a.authPhase == "password" {
			if !a.passwordEntered {
				a.passwordEntered = true
				a.errorMsg = ""
			} else {
				playerID, err := authenticate(a.db, strings.TrimSpace(a.username), strings.TrimSpace(a.password), false)
				if err != nil {
					a.errorMsg = err.Error()
					a.authPhase = "username"
					a.password = ""
					a.passwordEntered = false
				} else {
					a.playerID = playerID
					a.done = true
				}
			}
		} else if a.authPhase == "register" {
			if !a.passwordEntered {
				if strings.TrimSpace(a.username) == "" {
					a.errorMsg = "Username cannot be empty"
				} else {
					var exists int
					err := a.db.QueryRow("SELECT COUNT(*) FROM players WHERE name = $1", strings.TrimSpace(a.username)).Scan(&exists)
					if err != nil {
						log.Printf("Failed to check username '%s': %v", strings.TrimSpace(a.username), err)
						a.errorMsg = "Failed to check username"
					} else if exists > 0 {
						a.errorMsg = "Username already taken"
						a.username = ""
					} else {
						a.passwordEntered = true
						a.errorMsg = ""
						a.password = ""
					}
				}
			} else {
				playerID, err := authenticate(a.db, strings.TrimSpace(a.username), strings.TrimSpace(a.password), true)
				if err != nil {
					a.errorMsg = err.Error()
					a.authPhase = "username"
					a.username = ""
					a.password = ""
					a.passwordEntered = false
				} else {
					a.playerID = playerID
					a.done = true
				}
			}
		}
	}

	return nil
}

func (a *AuthState) Draw(screen *ebiten.Image) {
	// Use background_menu.png for auth screen
	if imgBackgroundMenu != nil {
		screen.DrawImage(imgBackgroundMenu, nil)
	} else {
		screen.Fill(color.RGBA{0, 128, 255, 255})
	}

	textImg := ebiten.NewImage(screenWidth, screenHeight)
	ebitenutil.DebugPrintAt(textImg, "Welcome to Egg Catcher: Wolf Edition!", screenWidth/3-100, screenHeight/3-100)
	if a.authPhase == "username" || (a.authPhase == "register" && !a.passwordEntered) {
		ebitenutil.DebugPrintAt(textImg, "Username: "+a.username+"_", screenWidth/3-50, screenHeight/3-50)
	}
	if a.authPhase == "password" || (a.authPhase == "register" && a.passwordEntered) {
		displayPassword := strings.Repeat("*", len(a.password))
		ebitenutil.DebugPrintAt(textImg, "Password: "+displayPassword+"_", screenWidth/3-50, screenHeight/3-20)
	}
	if a.errorMsg != "" {
		ebitenutil.DebugPrintAt(textImg, "Error: "+a.errorMsg, screenWidth/3-100, screenHeight/3-80)
	}

	if a.authPhase == "username" {
		a.drawButton(textImg, &a.loginButton)
		a.drawButton(textImg, &a.regButton)
	}
	a.drawButton(textImg, &a.submitButton)

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(1.5, 1.5)
	op.GeoM.Translate(0, 0)
	screen.DrawImage(textImg, op)
}

func (a *AuthState) drawButton(screen *ebiten.Image, b *Button) {
	buttonColor := color.RGBA{0, 128, 255, 255}
	if b.hovered {
		buttonColor = color.RGBA{0, 192, 255, 255}
	}
	ebitenutil.DrawRect(screen, b.x, b.y, b.w, b.h, buttonColor)
	ebitenutil.DebugPrintAt(screen, b.label, int(b.x+(b.w-float64(len(b.label)*7))/2), int(b.y+b.h/2))
}

func (a *AuthState) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func (w *GameWrapper) Update() error {
	if w.authState != nil && !w.authState.done {
		return w.authState.Update()
	}
	if w.authState != nil && w.authState.done {
		w.game = NewGame(w.authState.playerID, w.loseHeartPlayer, w.gainHeartPlayer, w.scoreHeartPlayer)
		w.authState = nil
	}
	if w.game != nil {
		return w.game.Update()
	}
	return nil
}

func (w *GameWrapper) Draw(screen *ebiten.Image) {
	if w.authState != nil {
		w.authState.Draw(screen)
	} else if w.game != nil {
		w.game.Draw(screen)
	}
}

func (w *GameWrapper) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func (g *Game) Update() error {
	if g.gameOver {
		cx, cy := ebiten.CursorPosition()
		mx, my := float64(cx), float64(cy)
		g.playagainButton.hovered = g.playagainButton.IsInside(mx, my)
		g.quitButton.hovered = g.quitButton.IsInside(mx, my)
		g.leaderboardButton.hovered = g.leaderboardButton.IsInside(mx, my)

		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			if g.playagainButton.hovered {
				if err := saveGameData(g); err != nil {
					log.Printf("Error saving game data: %v", err)
				}
				*g = *NewGame(g.playerID, g.loseHeartPlayer, g.gainHeartPlayer, g.scoreHeartPlayer)
			} else if g.quitButton.hovered {
				if err := saveGameData(g); err != nil {
					log.Printf("Error saving game data: %v", err)
				}
				os.Exit(0)
			} else if g.leaderboardButton.hovered {
				g.showLeaderboard = !g.showLeaderboard
			}
		}

		if inpututil.IsKeyJustPressed(ebiten.KeyR) {
			if err := saveGameData(g); err != nil {
				log.Printf("Error saving game data: %v", err)
			}
			*g = *NewGame(g.playerID, g.loseHeartPlayer, g.gainHeartPlayer, g.scoreHeartPlayer)
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyQ) {
			if err := saveGameData(g); err != nil {
				log.Printf("Error saving game data: %v", err)
			}
			os.Exit(0)
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyT) {
			g.showLeaderboard = !g.showLeaderboard
		}
		return nil
	}

	if g.isPaused {
		// Проверка выхода из паузы
		if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
			g.isPaused = false
			if player != nil {
				player.Play() // Возобновление музыки с текущей позиции
			}
		}
		return nil
	}

	// Переключение паузы
	if inpututil.IsKeyJustPressed(ebiten.KeyP) {
		g.isPaused = true
		if player != nil {
			player.Pause() // Остановка музыки
		}
		return nil
	}

	if g.score > 10*g.level && g.level < 20 {
		g.level++
	}

	// Wolf moves left and right as originally
	if ebiten.IsKeyPressed(ebiten.KeyA) && g.wolfX > 0 {
		if !g.isMoving {
			g.isMoving = true
		}
		g.wolfX -= 5
	} else if ebiten.IsKeyPressed(ebiten.KeyD) && g.wolfX < screenWidth-wolfWidth {
		if !g.isMoving {
			g.isMoving = true
		}
		g.wolfX += 5
	} else {
		g.isMoving = false
	}

	canDropEgg := true
	for _, egg := range g.eggs {
		if egg.active {
			canDropEgg = false
			break
		}
	}
	if canDropEgg {
		g.spawnEgg()
	}

	for i := range g.eggs {
		if g.eggs[i].active {
			if g.eggs[i].phase == "rolling" {
				accel := 0.03 + 0.03*float64(g.level)
				if g.eggs[i].vx > 0 {
					g.eggs[i].vx += accel / math.Sqrt(2)
				} else {
					g.eggs[i].vx -= accel / math.Sqrt(2)
				}
				g.eggs[i].vy += accel / math.Sqrt(2)
				g.eggs[i].x += g.eggs[i].vx
				g.eggs[i].y += g.eggs[i].vy
				if (g.eggs[i].vx > 0 && g.eggs[i].x >= g.eggs[i].transitionX) ||
					(g.eggs[i].vx < 0 && g.eggs[i].x <= g.eggs[i].transitionX) {
					g.eggs[i].phase = "falling"
				}
			} else {
				g.eggs[i].vy += 0.1
				g.eggs[i].x += g.eggs[i].vx
				g.eggs[i].y += g.eggs[i].vy
			}
			if g.eggs[i].y > screenHeight {
				g.eggs[i].active = false
				if g.eggs[i].value != 0 {
					g.lives--
					if g.loseHeartPlayer != nil {
						if err := g.loseHeartPlayer.Rewind(); err != nil {
							// No logging
						}
						g.loseHeartPlayer.Play()
					}
				}
			}
			if g.eggs[i].y >= g.basketY && g.eggs[i].y <= g.basketY+basketHeight &&
				g.eggs[i].x >= g.wolfX-basketWidth/2+wolfWidth/2 && g.eggs[i].x <= g.wolfX+basketWidth/2+wolfWidth/2 {
				g.eggs[i].active = false
				if g.eggs[i].value != 0 {
					g.score++
					if g.eggs[i].value == 2 && g.scoreHeartPlayer != nil {
						if err := g.scoreHeartPlayer.Rewind(); err != nil {
							// No logging
						}
						g.scoreHeartPlayer.Play()
					}
					if g.eggs[i].value == 1 && g.lives < 3 {
						g.lives++
						if g.gainHeartPlayer != nil {
							if err := g.gainHeartPlayer.Rewind(); err != nil {
								// No logging
							}
							g.gainHeartPlayer.Play()
						}
					}
				} else {
					g.lives--
					if g.loseHeartPlayer != nil {
						if err := g.loseHeartPlayer.Rewind(); err != nil {
							// No logging
						}
						g.loseHeartPlayer.Play()
					}
				}
				if g.score > g.record {
					g.record = g.score
				}
			}
		}
	}

	newEggs := make([]Egg, 0, len(g.eggs))
	for _, egg := range g.eggs {
		if egg.active {
			newEggs = append(newEggs, egg)
		}
	}
	g.eggs = newEggs

	if g.lives <= 0 {
		g.gameOver = true
	}

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	// Use background_menu.png for Game Over screen, background_main.png for active game
	if g.gameOver {
		if imgBackgroundMenu != nil {
			screen.DrawImage(imgBackgroundMenu, nil)
		} else {
			screen.Fill(color.RGBA{0, 128, 255, 255})
		}

		saveGameData(g)
		textImg := ebiten.NewImage(screenWidth, screenHeight)
		ebitenutil.DebugPrintAt(textImg, "Game Over", screenWidth/3-50, screenHeight/3-100-70)
		ebitenutil.DebugPrintAt(textImg, fmt.Sprintf("Your Score: %d", g.score), screenWidth/3-50, screenHeight/3-70-70)
		ebitenutil.DebugPrintAt(textImg, fmt.Sprintf("Your Record: %d", g.record), screenWidth/3-50, screenHeight/3-40-70)
		if g.showLeaderboard {
			leaderboard := loadLeaderboard()
			if len(leaderboard) == 0 {
				ebitenutil.DebugPrintAt(textImg, "No leaders yet", screenWidth/3-50, screenHeight/3-10)
			} else {
				for i, player := range leaderboard {
					if i >= 5 {
						break
					}
					leaderText := fmt.Sprintf("%d. %s - %d", i+1, player.Name, player.HighScore)
					ebitenutil.DebugPrintAt(textImg, leaderText, screenWidth/3-50, screenHeight/3-10+i*20-70)
				}
			}
		}
		g.drawButton(textImg, &g.playagainButton)
		g.drawButton(textImg, &g.quitButton)
		g.drawButton(textImg, &g.leaderboardButton)
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(1.5, 1.5)
		op.GeoM.Translate(0, 0)
		screen.DrawImage(textImg, op)
		return
	}

	// Use background_main.png for active game
	if imgBackgroundMain != nil {
		screen.DrawImage(imgBackgroundMain, nil)
	} else {
		screen.Fill(color.RGBA{0, 128, 255, 255})
	}

	// Draw wolf sprite with scaling, raised to align basket top
	basketX := float64(g.wolfX - basketWidth/2 + wolfWidth/2)
	if imgWolf != nil {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(2.0, 2.0)
		op.GeoM.Translate(basketX, g.basketY-20)
		screen.DrawImage(imgWolf, op)
	} else {
		ebitenutil.DrawRect(screen, basketX, g.basketY-20, float64(basketWidth), float64(basketHeight), color.RGBA{255, 0, 0, 255})
	}

	// Draw hens with sprite or fallback to yellow rectangles
	for _, hen := range g.hens {
		if imgHen != nil {
			op := &ebiten.DrawImageOptions{}
			if hen.x < screenWidth/2 {
				op.GeoM.Scale(-1, 1)
				op.GeoM.Translate(hen.x+henWidth, hen.y+5)
			} else {
				op.GeoM.Translate(hen.x, hen.y+9)
			}
			screen.DrawImage(imgHen, op)
		} else {
			if hen.x < screenWidth/2 {
				ebitenutil.DrawRect(screen, hen.x, hen.y+5, henWidth, henHeight, color.RGBA{255, 255, 0, 255})
			} else {
				ebitenutil.DrawRect(screen, hen.x, hen.y+9, henWidth, henHeight, color.RGBA{255, 255, 0, 255})
			}
		}
	}

	for _, hen := range g.hens {
		startX := hen.x + henWidth/2
		startY := hen.y + henHeight
		endX, endY := startX, startY
		if startX < screenWidth/2 {
			startY += 5
			endX = startX + 67.5
			endY = startY + 67.5
		} else {
			startY += 10
			endX = startX - 67.5
			endY = startY + 67.5
		}
		length := math.Hypot(endX-startX, endY-startY)
		tempImg := ebiten.NewImage(int(length), 10)
		ebitenutil.DrawRect(tempImg, 0, 0, length, 10, color.RGBA{160, 82, 45, 255})
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(-5, -5)
		op.GeoM.Rotate(math.Atan2(endY-startY, endX-startX))
		op.GeoM.Translate(startX, startY+eggSize/2)
		screen.DrawImage(tempImg, op)
	}

	// Draw eggs with realistic rolling and falling animation
	for _, egg := range g.eggs {
		if egg.active {
			var eggImg *ebiten.Image
			switch egg.value {
			case 0:
				eggImg = imgFakeEgg
			case 1:
				eggImg = imgWhiteEgg
			case 2:
				eggImg = imgGoldEgg
			}
			if eggImg != nil {
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Translate(-eggSize/2, -eggSize/2-3)
				var angle float64
				if egg.phase == "rolling" {
					angle += egg.vx * 1.0
				} else if egg.phase == "falling" {
					angle += egg.vy * 1.0
				}
				angle = math.Mod(angle, 2*math.Pi)
				op.GeoM.Rotate(angle)
				op.GeoM.Translate(egg.x+eggSize/2, egg.y+eggSize/2)
				screen.DrawImage(eggImg, op)
			} else {
				tempImg := ebiten.NewImage(int(eggSize), int(eggSize))
				ebitenutil.DrawRect(tempImg, 0, 0, eggSize, eggSize, color.RGBA{0, 0, 0, 255})
				if egg.value == 2 {
					ebitenutil.DrawRect(tempImg, 1, 1, eggSize-2, eggSize-2, color.RGBA{255, 255, 255, 255})
				} else if egg.value == 0 {
					ebitenutil.DrawRect(tempImg, 1, 1, eggSize-2, eggSize-2, color.RGBA{150, 75, 0, 255})
				} else {
					ebitenutil.DrawRect(tempImg, 1, 1, eggSize-2, eggSize-2, color.RGBA{255, 220, 0, 255})
				}
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Translate(-eggSize/2, -eggSize/2)
				op.GeoM.Rotate(egg.y / 20 * 2 * math.Pi)
				op.GeoM.Translate(egg.x+eggSize/2, egg.y+eggSize/2)
				screen.DrawImage(tempImg, op)
			}
		}
	}

	// Draw hearts with separate sprites, positioned max right
	for i := 0; i < 3; i++ {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(640.0+float64(i*55), -10.0)
		if imgHeart1 != nil && imgHeart2 != nil {
			if i < g.lives {
				screen.DrawImage(imgHeart1, op)
			} else {
				screen.DrawImage(imgHeart2, op)
			}
		} else {
			heartColor := color.RGBA{255, 0, 0, 255}
			if i >= g.lives {
				heartColor = color.RGBA{128, 128, 128, 255}
			}
			ebitenutil.DrawRect(screen, 600.0+float64(i*50), 0.0, heartSize, heartSize, heartColor)
		}
	}

	textImg := ebiten.NewImage(screenWidth, screenHeight)
	ebitenutil.DebugPrint(textImg, fmt.Sprintf("Score: %d Record: %d Lives: %d Level: %d", g.score, g.record, g.lives, g.level))
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(1.5, 1.5)
	op.GeoM.Translate(10, 10)
	screen.DrawImage(textImg, op)

	// Отображение паузы поверх всех элементов
	if g.isPaused {
		pauseTextImg := ebiten.NewImage(screenWidth, screenHeight)
		pauseOp := &ebiten.DrawImageOptions{}
		pauseOp.GeoM.Scale(3.0, 3.0) // Увеличение надписи для большей видимости
		pauseOp.GeoM.Translate(float64(screenWidth/2-150), float64(screenHeight/2-60))
		screen.DrawImage(pauseTextImg, pauseOp)
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func (g *Game) drawButton(screen *ebiten.Image, b *Button) {
	buttonColor := color.RGBA{0, 128, 255, 255}
	if b.hovered {
		buttonColor = color.RGBA{0, 192, 255, 255}
	}
	ebitenutil.DrawRect(screen, b.x, b.y, b.w, b.h, buttonColor)
	ebitenutil.DebugPrintAt(screen, b.label, int(b.x+(b.w-float64(len(b.label)*7))/2), int(b.y+b.h/2))
}

func clearDatabase(db *sql.DB) error {
	_, err := db.Exec("TRUNCATE TABLE games RESTART IDENTITY CASCADE")
	if err != nil {
		return fmt.Errorf("failed to clear games table: %v", err)
	}
	_, err = db.Exec("TRUNCATE TABLE players RESTART IDENTITY")
	if err != nil {
		return fmt.Errorf("failed to clear players table: %v", err)
	}
	return nil
}

func loadImage(path string) (*ebiten.Image, error) {
	file, err := imageFiles.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error opening embedded %s: %v", path, err)
	}
	defer file.Close()
	img, _, err := ebitenutil.NewImageFromReader(file)
	if err != nil {
		return nil, fmt.Errorf("error loading embedded %s: %v", path, err)
	}
	return img, nil
}

func loadAudio(path string) (*audio.Player, error) {
	file, err := audioFiles.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error opening embedded %s: %v", path, err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("error reading embedded %s: %v", path, err)
	}
	mp3Stream, err := mp3.DecodeWithSampleRate(44100, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("error decoding embedded %s: %v", path, err)
	}
	player, err := audioContext.NewPlayer(mp3Stream) // Используем глобальный audioContext
	if err != nil {
		return nil, fmt.Errorf("error creating player for %s: %v", path, err)
	}
	return player, nil
}

func main() {
	clear := flag.Bool("clear", false, "Clear all database data")
	flag.Parse()

	rand.Seed(time.Now().UnixNano())
	audioContext = audio.NewContext(44100) // Инициализация глобального аудио-контекста

	// Load background and hen images
	var err error
	imgBackgroundMenu, err = loadImage("avi/background_menu.png")
	if err != nil {
		log.Printf("Error: %v", err)
	}
	imgBackgroundMain, err = loadImage("avi/background_main.png")
	if err != nil {
		log.Printf("Error: %v", err)
	}
	imgHen, err = loadImage("avi/hen.png")
	if err != nil {
		log.Printf("Error: %v", err)
	}
	imgWolf, err = loadImage("avi/wolf.png")
	if err != nil {
		log.Printf("Error: %v", err)
	}
	imgFakeEgg, err = loadImage("avi/fake_egg.png")
	if err != nil {
		log.Printf("Error: %v", err)
	}
	imgWhiteEgg, err = loadImage("avi/white_egg.png")
	if err != nil {
		log.Printf("Error: %v", err)
	}
	imgGoldEgg, err = loadImage("avi/gold_egg.png")
	if err != nil {
		log.Printf("Error: %v", err)
	}
	imgHeart1, err = loadImage("avi/heart1.png")
	if err != nil {
		log.Printf("Error: %v", err)
	}
	imgHeart2, err = loadImage("avi/heart2.png")
	if err != nil {
		log.Printf("Error: %v", err)
	}

	// Load audio
	player, err = loadAudio("music/converted_new_music.mp3")
	if err != nil {
		log.Printf("Error: %v", err)
	} else if player != nil {
		player.SetVolume(1.0)
		player.Play()
	}

	loseHeartPlayer, err := loadAudio("music/lose_heart.mp3")
	if err != nil {
		log.Printf("Error: %v", err)
	}
	gainHeartPlayer, err := loadAudio("music/gain_heart.mp3")
	if err != nil {
		log.Printf("Error: %v", err)
	}
	scoreHeartPlayer, err := loadAudio("music/score_heart.mp3")
	if err != nil {
		log.Printf("Error: %v", err)
	}

	if loseHeartPlayer != nil {
		loseHeartPlayer.SetVolume(1.0)
	}
	if gainHeartPlayer != nil {
		gainHeartPlayer.SetVolume(1.0)
	}
	if scoreHeartPlayer != nil {
		scoreHeartPlayer.SetVolume(1.5)
	}

	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("Egg Catcher: Wolf Edition")

	db, err = initDB()
	if err != nil {
		log.Fatal("Error initializing database: %v", err)
	}
	defer db.Close()

	if *clear {
		err := clearDatabase(db)
		if err != nil {
			log.Fatal("Error clearing database: %v", err)
		}
		fmt.Println("Database cleared successfully")
		os.Exit(0)
	}

	wrapper := &GameWrapper{
		authState: &AuthState{
			db:        db,
			authPhase: "username",
			loginButton: Button{
				x:     screenWidth/3 - buttonWidth - 10,
				y:     screenHeight/3 + 20,
				w:     buttonWidth,
				h:     buttonHeight,
				label: "Login",
			},
			regButton: Button{
				x:     screenWidth/3 + 10,
				y:     screenHeight/3 + 20,
				w:     buttonWidth,
				h:     buttonHeight,
				label: "Register",
			},
			submitButton: Button{
				x:     screenWidth/3 - buttonWidth/2,
				y:     screenHeight/3 + 80,
				w:     buttonWidth,
				h:     buttonHeight,
				label: "Submit",
			},
		},
		loseHeartPlayer:  loseHeartPlayer,
		gainHeartPlayer:  gainHeartPlayer,
		scoreHeartPlayer: scoreHeartPlayer,
	}
	if err := ebiten.RunGame(wrapper); err != nil {
		log.Fatal(err)
	}
}
