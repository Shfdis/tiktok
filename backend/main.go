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
var matchEpoch uint64

type matchMsg struct {
	epoch uint64
	state MyState
	err   string
}

// Buffer >1 helps avoid rare blocking if a stale message is left behind.
var matchState chan matchMsg = make(chan matchMsg, 8)

func play(ctx *gin.Context) {
	matchUpMutex.Lock()
	if first {
		epoch := matchEpoch
		state, myId, otherId, err := CreateGame(dbPointer)
		if err != nil || myId == 0 {
			// Reset first so next player can try again, and unblock waiting player
			first = false
			select {
			case matchState <- matchMsg{epoch: epoch, err: "Couldn't create game"}:
			default:
			}
			matchUpMutex.Unlock()
			ctx.JSON(500, gin.H{"error": "Couldn't create game"})
			return
		}
		first = false
		// If the waiting request got cancelled, nobody may be receiving anymore; never block here.
		select {
		case matchState <- matchMsg{epoch: epoch, state: MyState{Id: otherId, GameState: *state, Role: Circle}}:
		default:
		}
		ctx.IndentedJSON(200, MyState{Id: myId, GameState: *state, Role: Cross})
		matchUpMutex.Unlock()
		return
	}
	first = true
	matchEpoch++
	myEpoch := matchEpoch
	matchUpMutex.Unlock()

	for {
		select {
		case <-ctx.Request.Context().Done():
			// Cleanup waiting slot if the client cancels while waiting for a match.
			matchUpMutex.Lock()
			if first && matchEpoch == myEpoch {
				first = false
				matchEpoch++
			}
			matchUpMutex.Unlock()

			// Drain any stale messages so they don't affect the next match.
			for {
				select {
				case <-matchState:
				default:
					ctx.JSON(408, gin.H{"error": "Match cancelled"})
					return
				}
			}

		case msg := <-matchState:
			// Ignore stale messages from a previously cancelled waiting request.
			if msg.epoch != myEpoch {
				continue
			}
			if msg.err != "" {
				ctx.JSON(500, gin.H{"error": msg.err})
				return
			}
			ctx.IndentedJSON(200, msg.state)
			return
		}
	}
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
