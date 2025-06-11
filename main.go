package main

import (
	"database/sql"
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/mp3"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	_ "github.com/mattn/go-sqlite3"
	"image/color"
	_ "io"
	"log"
	"math"
	"math/rand"
	"os"
	"strings"
	"time"
)

const (
	screenWidth  = 800
	screenHeight = 600
	wolfWidth    = 50
	wolfHeight   = 50
	basketWidth  = 80
	basketHeight = 20
	henWidth     = 40
	henHeight    = 40
	eggSize      = 10
	heartSize    = 20
	buttonWidth  = 200
	buttonHeight = 50
)

var db *sql.DB

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
	replayButton      Button
	quitButton        Button
	leaderboardButton Button
	playerID          int
}

type Hen struct {
	x, y float64
}

type Egg struct {
	x, y        float64
	vx, vy      float64 // Velocity components
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

type Player struct {
	ID        int
	Name      string
	HighScore int
}

type AuthState struct {
	db           *sql.DB
	username     string
	isRegister   bool // True for registration, false for login
	loginButton  Button
	regButton    Button
	submitButton Button
	errorMsg     string
	playerID     int  // Set on successful auth
	done         bool // True when auth is complete
}

type GameWrapper struct {
	authState *AuthState
	game      *Game
}

func NewGame(playerID int) *Game {
	g := &Game{
		wolfX:           screenWidth/2 - wolfWidth/2,
		wolfY:           screenHeight - wolfHeight - 20,
		basketY:         screenHeight - wolfHeight - basketHeight - 10,
		level:           1,
		score:           0,
		record:          0,
		lives:           3,
		gameOver:        false,
		showLeaderboard: false,
		playerID:        playerID,
	}
	loadPlayerData(g)
	g.hens[0] = Hen{x: 150, y: 50}
	g.hens[1] = Hen{x: 100, y: 100}
	g.hens[2] = Hen{x: 650, y: 50}
	g.hens[3] = Hen{x: 700, y: 100}
	g.replayButton = Button{
		x:     screenWidth/3 - buttonWidth - 10,
		y:     screenHeight/3 + 20,
		w:     buttonWidth,
		h:     buttonHeight,
		label: "Replay",
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
		valueEgg = 0
	} else if probability < 0.15 {
		valueEgg = 1
	} else {
		valueEgg = 2
	}
	henIndex := rand.Intn(4)
	eggX := g.hens[henIndex].x + henWidth/2 - eggSize/2
	vx, transitionX := 0.0, 0.0
	baseSpeed := 1.0 + 0.3*float64(g.level-1)
	if eggX < screenWidth/2 {
		vx = baseSpeed / math.Sqrt(2)
		transitionX = eggX + 67.5
	} else {
		vx = -baseSpeed / math.Sqrt(2)
		transitionX = eggX - 82.5
	}
	var offset float64
	if vx > 0 {
		offset = -5
	} else {
		offset = -15
	}
	g.eggs = append(g.eggs, Egg{
		x:           eggX,
		y:           g.hens[henIndex].y + henHeight + offset,
		vx:          vx,
		vy:          baseSpeed / math.Sqrt(2),
		phase:       "rolling",
		transitionX: transitionX,
		active:      true,
		value:       valueEgg,
	})
}

func initDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "egg_catcher.db")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS players (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			high_score INTEGER DEFAULT 0
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to create players table: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS games (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			player_id INTEGER NOT NULL,
			score INTEGER NOT NULL,
			lives INTEGER NOT NULL,
			date DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (player_id) REFERENCES players(id)
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to create games table: %v", err)
	}
	return db, nil
}

func authenticate(db *sql.DB, username string, isRegister bool) (int, error) {
	if username == "" {
		return 0, fmt.Errorf("username cannot be empty")
	}
	var playerID int
	var exists int
	err := db.QueryRow("SELECT COUNT(*) FROM players WHERE name = ?", username).Scan(&exists)
	if err != nil {
		return 0, fmt.Errorf("failed to check user existence: %v", err)
	}

	if isRegister {
		if exists > 0 {
			return 0, fmt.Errorf("username already taken")
		}
		result, err := db.Exec("INSERT INTO players (name, high_score) VALUES (?, 0)", username)
		if err != nil {
			return 0, fmt.Errorf("failed to insert new player: %v", err)
		}
		playerID64, err := result.LastInsertId()
		if err != nil {
			return 0, fmt.Errorf("failed to get new player ID: %v", err)
		}
		playerID = int(playerID64)
		return playerID, nil
	}

	if exists == 0 {
		return 0, fmt.Errorf("user does not exist")
	}
	err = db.QueryRow("SELECT id FROM players WHERE name = ?", username).Scan(&playerID)
	if err != nil {
		return 0, fmt.Errorf("failed to get existing player ID: %v", err)
	}
	return playerID, nil
}

func loadPlayerData(g *Game) {
	if db == nil {
		return
	}
	var player Player
	err := db.QueryRow("SELECT id, name, high_score FROM players WHERE id = ?", g.playerID).Scan(&player.ID, &player.Name, &player.HighScore)
	if err != nil {
		if err == sql.ErrNoRows {
			_, err = db.Exec("INSERT INTO players (name, high_score) VALUES (?, 0)", "Player1")
			if err != nil {
				log.Printf("Error creating default player: %v", err)
				return
			}
			g.score = 0
			g.record = 0
		} else {
			log.Printf("Error loading player data: %v", err)
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
	_, err := db.Exec("INSERT INTO games (player_id, score, lives) VALUES (?, ?, ?)", g.playerID, g.score, g.lives)
	if err != nil {
		return fmt.Errorf("failed to save game data: %v", err)
	}
	var currentHighScore int
	err = db.QueryRow("SELECT high_score FROM players WHERE id = ?", g.playerID).Scan(&currentHighScore)
	if err != nil {
		return fmt.Errorf("failed to get current high score: %v", err)
	}
	if g.score > currentHighScore {
		_, err = db.Exec("UPDATE players SET high_score = ? WHERE id = ?", g.score, g.playerID)
		if err != nil {
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
	// Handle text input
	runes := ebiten.AppendInputChars(nil)
	for _, r := range runes {
		if len(a.username) < 20 { // Limit username length
			a.username += string(r)
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && len(a.username) > 0 {
		a.username = a.username[:len(a.username)-1]
	}

	// Handle mouse input
	cx, cy := ebiten.CursorPosition()
	mx, my := float64(cx), float64(cy)
	a.loginButton.hovered = a.loginButton.IsInside(mx, my)
	a.regButton.hovered = a.regButton.IsInside(mx, my)
	a.submitButton.hovered = a.submitButton.IsInside(mx, my)

	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		if a.loginButton.hovered {
			a.isRegister = false
			a.errorMsg = ""
		} else if a.regButton.hovered {
			a.isRegister = true
			a.errorMsg = ""
		} else if a.submitButton.hovered {
			playerID, err := authenticate(a.db, strings.TrimSpace(a.username), a.isRegister)
			if err != nil {
				a.errorMsg = err.Error()
			} else {
				a.playerID = playerID
				a.done = true
			}
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		playerID, err := authenticate(a.db, strings.TrimSpace(a.username), a.isRegister)
		if err != nil {
			a.errorMsg = err.Error()
		} else {
			a.playerID = playerID
			a.done = true
		}
	}

	return nil
}

func (b *Button) IsInside(x, y float64) bool {
	scale := 1.5
	return x >= b.x*scale && x <= (b.x+b.w)*scale && y >= b.y*scale && y <= (b.y+b.h)*scale
}

func (a *AuthState) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{0, 128, 255, 255})

	textImg := ebiten.NewImage(screenWidth, screenHeight)
	ebitenutil.DebugPrintAt(textImg, "Welcome to Egg Catcher: Wolf Edition!", screenWidth/3-100, screenHeight/3-100)
	ebitenutil.DebugPrintAt(textImg, "Username: "+a.username+"_", screenWidth/3-50, screenHeight/3-50)
	if a.errorMsg != "" {
		ebitenutil.DebugPrintAt(textImg, "Error: "+a.errorMsg, screenWidth/3-70, screenHeight/3-20)
	}

	// Draw buttons
	a.drawButton(textImg, &a.loginButton)
	a.drawButton(textImg, &a.regButton)
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
		w.game = NewGame(w.authState.playerID)
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
		// Handle mouse input
		cx, cy := ebiten.CursorPosition()
		mx, my := float64(cx), float64(cy)
		g.replayButton.hovered = g.replayButton.IsInside(mx, my)
		g.quitButton.hovered = g.quitButton.IsInside(mx, my)
		g.leaderboardButton.hovered = g.leaderboardButton.IsInside(mx, my)

		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			if g.replayButton.hovered {
				if err := saveGameData(g); err != nil {
					log.Printf("Error saving game data: %v", err)
				}
				*g = *NewGame(g.playerID)
			} else if g.quitButton.hovered {
				if err := saveGameData(g); err != nil {
					log.Printf("Error saving game data: %v", err)
				}
				os.Exit(0)
			} else if g.leaderboardButton.hovered {
				g.showLeaderboard = !g.showLeaderboard
			}
		}

		// Preserve keyboard input
		if inpututil.IsKeyJustPressed(ebiten.KeyR) {
			if err := saveGameData(g); err != nil {
				log.Printf("Error saving game data: %v", err)
			}
			*g = *NewGame(g.playerID)
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

	// Update level every 5 points
	if g.score > 10*g.level && g.level < 20 {
		g.level++
	}

	// Control wolf and basket
	if ebiten.IsKeyPressed(ebiten.KeyA) && g.wolfX > 0 {
		g.wolfX -= 5
	}
	if ebiten.IsKeyPressed(ebiten.KeyD) && g.wolfX < screenWidth-wolfWidth {
		g.wolfX += 5
	}
	if ebiten.IsKeyPressed(ebiten.KeyW) && g.basketY > g.wolfY-basketHeight {
		g.basketY -= 5
	}
	if ebiten.IsKeyPressed(ebiten.KeyS) && g.basketY < screenHeight-basketHeight {
		g.basketY += 5
	}

	// Egg spawning and movement
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
				// Accelerate along 45° chute
				accel := 0.03 + 0.01*float64(g.level) // Gravity component along chute
				if g.eggs[i].vx > 0 {
					g.eggs[i].vx += accel / math.Sqrt(2)
					g.eggs[i].vy += accel / math.Sqrt(2)
				} else {
					g.eggs[i].vx -= accel / math.Sqrt(2)
					g.eggs[i].vy += accel / math.Sqrt(2)
				}
				g.eggs[i].x += g.eggs[i].vx
				g.eggs[i].y += g.eggs[i].vy
				if (g.eggs[i].vx > 0 && g.eggs[i].x >= g.eggs[i].transitionX) ||
					(g.eggs[i].vx < 0 && g.eggs[i].x <= g.eggs[i].transitionX) {
					g.eggs[i].phase = "falling"
				}
			} else { // falling
				g.eggs[i].vy += 0.1 // Gravitational acceleration
				g.eggs[i].x += g.eggs[i].vx
				g.eggs[i].y += g.eggs[i].vy
			}
			if g.eggs[i].y > screenHeight {
				g.eggs[i].active = false
				if g.eggs[i].value != 0 {
					g.lives--
				}
			}
			if g.eggs[i].y >= g.basketY && g.eggs[i].y <= g.basketY+basketHeight &&
				g.eggs[i].x >= g.wolfX-basketWidth/2+wolfWidth/2 && g.eggs[i].x <= g.wolfX+basketWidth/2+wolfWidth/2 {
				g.eggs[i].active = false
				if g.eggs[i].value != 0 {
					g.score++
					if g.eggs[i].value == 1 && g.lives < 3 {
						g.lives++
					}
				} else {
					g.lives--
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
	screen.Fill(color.RGBA{0, 128, 255, 255})

	if g.gameOver {
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
		g.drawButton(textImg, &g.replayButton)
		g.drawButton(textImg, &g.quitButton)
		g.drawButton(textImg, &g.leaderboardButton)
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(1.5, 1.5)
		op.GeoM.Translate(0, 0)
		screen.DrawImage(textImg, op)
		return
	}

	// Draw wolf
	ebitenutil.DrawRect(screen, g.wolfX, g.wolfY, wolfWidth, wolfHeight, color.RGBA{0, 255, 0, 255})
	// Draw basket
	basketX := g.wolfX - basketWidth/2 + wolfWidth/2
	ebitenutil.DrawRect(screen, basketX, g.basketY, basketWidth, basketHeight, color.RGBA{255, 0, 0, 255})

	// Draw hens
	for _, hen := range g.hens {
		ebitenutil.DrawRect(screen, hen.x, hen.y, henWidth, henHeight, color.RGBA{255, 255, 0, 255})
	}

	// Draw chutes
	for _, hen := range g.hens {
		startX := hen.x + henWidth/2 - eggSize/2
		startY := hen.y + henHeight
		endX, endY := startX, startY
		if startX < screenWidth/2 {
			endX = startX + 67.5
			endY = startY + 67.5
		} else {
			endX = startX - 82.5
			endY = startY + 82.5
		}
		length := math.Hypot(endX-startX, endY-startY)
		tempImg := ebiten.NewImage(int(length), 10)
		ebitenutil.DrawRect(tempImg, 0, 0, length, 10, color.RGBA{160, 82, 45, 255})
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(-5, -5)
		op.GeoM.Rotate(math.Atan2(endY-startY, endX-startX))
		op.GeoM.Translate(startX, startY)
		screen.DrawImage(tempImg, op)
	}

	// Draw eggs
	for _, egg := range g.eggs {
		if egg.active {
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
			yOffset := 0.0
			if egg.phase == "rolling" {
				yOffset = -4 // Adjusted for top alignment
			}
			op.GeoM.Translate(egg.x+eggSize/2, egg.y+eggSize/2+yOffset)
			screen.DrawImage(tempImg, op)
		}
	}

	// Draw lives
	for i := 0; i < 3; i++ {
		heartColor := color.RGBA{255, 0, 0, 255}
		if i >= g.lives {
			heartColor = color.RGBA{128, 128, 128, 255}
		}
		ebitenutil.DrawRect(screen, float64(screenWidth-100+i*30), 20, heartSize, heartSize, heartColor)
	}

	// Draw score, record, lives, level
	textImg := ebiten.NewImage(screenWidth, screenHeight)
	ebitenutil.DebugPrint(textImg, fmt.Sprintf("Score: %d Record: %d Lives: %d Level: %d", g.score, g.record, g.lives, g.level))
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(1.5, 1.5)
	op.GeoM.Translate(10, 10)
	screen.DrawImage(textImg, op)
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

func main() {
	rand.Seed(time.Now().UnixNano())
	// Initialize audio context
	audioContext := audio.NewContext(44100)
	// Load MP3 file
	mp3File, err := os.Open("converted_new_music.mp3")
	if err != nil {
		log.Fatal("Error opening MP3 file: %v", err)
	}
	defer mp3File.Close()
	mp3Stream, err := mp3.DecodeWithSampleRate(44100, mp3File)
	if err != nil {
		log.Fatal("Error decoding MP3: %v", err)
	}
	// Create looping player
	player, err := audioContext.NewPlayer(audio.NewInfiniteLoop(mp3Stream, mp3Stream.Length()))
	if err != nil {
		log.Fatal("Error creating audio player: %v", err)
	}
	// Start playing
	player.Play()
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("Egg Catcher: Wolf Edition")
	db, err = initDB()
	if err != nil {
		log.Fatal("Error initializing database: %v", err)
	}
	defer db.Close()
	wrapper := &GameWrapper{
		authState: &AuthState{
			db: db,
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
	}
	if err := ebiten.RunGame(wrapper); err != nil {
		log.Fatal(err)
	}
}
