package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var addr = flag.String("addr", ":8080", "http service address")
var dbPointer *sql.DB
var matchUpMutex sync.Mutex
var first = false

type matchMsg struct {
	state MyState
	err   string
}

// Buffer of 1 prevents deadlocks if one side disconnects or if game creation fails.
var matchState chan matchMsg = make(chan matchMsg, 1)

func play(ctx *gin.Context) {
	matchUpMutex.Lock()
	if first {
		state, myId, otherId, err := CreateGame(dbPointer)
		if err != nil || myId == 0 {
			// Reset first so next player can try again, and unblock waiting player
			first = false
			matchState <- matchMsg{err: "Couldn't create game"}
			matchUpMutex.Unlock()
			ctx.JSON(500, gin.H{"error": "Couldn't create game"})
			return
		}
		first = false
		matchState <- matchMsg{state: MyState{Id: otherId, GameState: *state, Role: Circle}}
		ctx.IndentedJSON(200, MyState{Id: myId, GameState: *state, Role: Cross})
		matchUpMutex.Unlock()
		return
	}
	first = true
	matchUpMutex.Unlock()
	msg := <-matchState
	if msg.err != "" {
		ctx.JSON(500, gin.H{"error": msg.err})
		return
	}
	ctx.IndentedJSON(200, msg.state)
}
func move(ctx *gin.Context) {
	var moveData Move
	if err := ctx.ShouldBindJSON(&moveData); err != nil {
		ctx.JSON(400, gin.H{"error": "Invalid move data"})
		return
	}

	var idParam struct {
		Id int64 `form:"id" binding:"required"`
	}
	if err := ctx.ShouldBindQuery(&idParam); err != nil {
		ctx.JSON(400, gin.H{"error": "Missing id parameter"})
		return
	}
	id := idParam.Id

	state, err := MakeMove(dbPointer, id, moveData)
	if err != nil {
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}

	ctx.IndentedJSON(200, state)
}

func getState(ctx *gin.Context) {
	var idParam struct {
		Id int64 `form:"id" binding:"required"`
	}
	if err := ctx.ShouldBindQuery(&idParam); err != nil {
		ctx.JSON(400, gin.H{"error": "Missing id parameter"})
		return
	}
	id := idParam.Id

	tx, err := dbPointer.Begin()
	if err != nil {
		ctx.JSON(500, gin.H{"error": "Database error"})
		return
	}
	defer tx.Rollback()

	state, player, err := GetState(tx, id)
	if err != nil {
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}

	tx.Commit()
	ctx.IndentedJSON(200, MyState{Id: id, GameState: *state, Role: player})
}

func startDailyCleanup(ctx context.Context, db *sql.DB) {
	go func() {
		// Run once a day, aligned to midnight in the container's local time.
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		timer := time.NewTimer(time.Until(next))
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			if err := ClearGames(db); err != nil {
				log.Printf("daily cleanup failed: %v", err)
			} else {
				log.Printf("daily cleanup: cleared games table")
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func main() {
	db, err := SetUpDatabase()
	dbPointer = db
	if err != nil {
		panic("Database creation failed")
	}
	defer CleanupDatabase(db)

	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	defer cleanupCancel()
	startDailyCleanup(cleanupCtx, dbPointer)

	log.SetFlags(0)
	flag.Parse()
	if envAddr := os.Getenv("ADDR"); envAddr != "" {
		*addr = envAddr
	}
	r := gin.Default()

	r.POST("/play", play)
	r.PUT("/play", move)
	r.GET("/play", getState)
	r.Run(*addr)
}
