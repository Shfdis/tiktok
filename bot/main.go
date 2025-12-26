package main

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

const (
	defaultBaseURL   = "http://localhost:8080"
	searchDepth      = 5
	matchmakeEvery   = 1 * time.Minute
	startGameTimeout = 1 * time.Second
	actionTimeout    = 1 * time.Second
	pollInterval     = 250 * time.Millisecond
	gameMaxDuration  = 1 * time.Hour
)

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func runSinglePlayer(ctx context.Context, baseURL string, id int64, depth int, poll time.Duration, actionTimeout time.Duration) error {
	lastStatusLog := time.Now()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		reqCtx, cancel := context.WithTimeout(ctx, actionTimeout)
		ms, err := GetStateByID(reqCtx, baseURL, id)
		cancel()
		if err != nil {
			return err
		}
		st := ms.GameState
		if st.Winner != None {
			fmt.Printf("game finished: id=%d winner=%d\n", id, st.Winner)
			return nil
		}
		if st.ToMove != ms.Role {
			// Heartbeat so it doesn't look "frozen" while waiting for opponent.
			if time.Since(lastStatusLog) > 10*time.Second {
				fmt.Printf("waiting: id=%d myRole=%d toMove=%d location=%d\n", id, ms.Role, st.ToMove, st.Location)
				lastStatusLog = time.Now()
			}
			if err := sleepCtx(ctx, poll); err != nil {
				return err
			}
			continue
		}

		reqCtx, cancel = context.WithTimeout(ctx, actionTimeout)
		thinkStart := time.Now()
		mv, next, err := PlayBestMove(reqCtx, baseURL, id, depth)
		cancel()
		if err != nil {
			fmt.Printf("think failed: id=%d role=%d toMove=%d location=%d err=%v\n", id, ms.Role, st.ToMove, st.Location, err)
			if err := sleepCtx(ctx, poll); err != nil {
				return err
			}
			continue
		}
		_ = thinkStart // reserved for future timing logs if needed
		fmt.Printf("played id=%d role=%d move=(%d,%d)->(%d,%d) nextToMove=%d winner=%d\n",
			id, ms.Role, mv.CellX, mv.CellY, mv.FinalX, mv.FinalY, next.ToMove, next.Winner)
	}
}

func main() {
	ctx := context.Background()

	baseURL := os.Getenv("BOT_BASE_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	fmt.Printf("bot started: base=%s depth=%d matchmakingEvery=%s gameTimeout=%s\n",
		baseURL, searchDepth, matchmakeEvery, gameMaxDuration)

	var activeGames int64

	startGame := func() {
		startCtx, cancel := context.WithTimeout(ctx, startGameTimeout)
		ms, err := StartGame(startCtx, baseURL)
		cancel()
		if err != nil {
			fmt.Printf("start game failed (retry in %s): %v\n", matchmakeEvery, err)
			return
		}

		n := atomic.AddInt64(&activeGames, 1)
		fmt.Printf("entered game: id=%d role=%d toMove=%d location=%d active=%d\n",
			ms.Id, ms.Role, ms.GameState.ToMove, ms.GameState.Location, n)

		go func(id int64) {
			defer func() {
				n := atomic.AddInt64(&activeGames, -1)
				fmt.Printf("game goroutine ended: id=%d active=%d\n", id, n)
			}()
			gameCtx, cancel := context.WithTimeout(context.Background(), gameMaxDuration)
			defer cancel()
			err := runSinglePlayer(gameCtx, baseURL, id, searchDepth, pollInterval, actionTimeout)
			if err == context.DeadlineExceeded {
				fmt.Printf("game timed out: id=%d after=%s\n", id, gameMaxDuration)
				return
			}
			if err != nil && err != context.Canceled {
				fmt.Printf("game ended with error: id=%d err=%v\n", id, err)
			}
		}(ms.Id)
	}

	// Try immediately, then every minute.
	startGame()
	ticker := time.NewTicker(matchmakeEvery)
	defer ticker.Stop()
	for range ticker.C {
		startGame()
	}
}
