package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type apiError struct {
	Error string `json:"error"`
}

func normalizeBaseURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/")
}

func readAPIResponse[T any](res *http.Response) (T, error) {
	var zero T
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return zero, err
	}

	if res.StatusCode >= 400 {
		var e apiError
		if json.Unmarshal(body, &e) == nil && e.Error != "" {
			return zero, errors.New(e.Error)
		}
		return zero, fmt.Errorf("api error %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := json.Unmarshal(body, &zero); err != nil {
		return zero, err
	}
	return zero, nil
}

// StartGame tries to create a new game session and returns the initial MyState (including the assigned id).
//
// Different backends implement this differently; we try POST /play first, then fall back to GET /play.
func StartGame(ctx context.Context, baseURL string) (MyState, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("%s/play", normalizeBaseURL(baseURL))

	// Try POST /play
	{
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
		if err != nil {
			return MyState{}, err
		}
		res, err := client.Do(req)
		if err == nil {
			defer res.Body.Close()
			if res.StatusCode < 400 {
				return readAPIResponse[MyState](res)
			}
			// If POST is not supported, fall back to GET.
			if res.StatusCode != http.StatusNotFound && res.StatusCode != http.StatusMethodNotAllowed {
				_, e := readAPIResponse[MyState](res)
				return MyState{}, e
			}
		}
	}

	// Fall back to GET /play
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return MyState{}, err
	}
	res, err := client.Do(req)
	if err != nil {
		return MyState{}, err
	}
	defer res.Body.Close()
	return readAPIResponse[MyState](res)
}

// GetStateByID calls backend GET /play?id=... and returns the MyState (includes Role derived from id).
func GetStateByID(ctx context.Context, baseURL string, id int64) (MyState, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("%s/play?id=%d", normalizeBaseURL(baseURL), id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return MyState{}, err
	}
	res, err := client.Do(req)
	if err != nil {
		return MyState{}, err
	}
	defer res.Body.Close()
	return readAPIResponse[MyState](res)
}

// SendMove calls backend PUT /play?id=... with the provided move and returns the updated State.
func SendMove(ctx context.Context, baseURL string, id int64, mv Move) (State, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("%s/play?id=%d", normalizeBaseURL(baseURL), id)

	b, err := json.Marshal(mv)
	if err != nil {
		return State{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(b))
	if err != nil {
		return State{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return State{}, err
	}
	defer res.Body.Close()
	return readAPIResponse[State](res)
}

// PlayBestMove fetches state for this id, finds the best move for ms.Role, and submits it.
// Returns the move played and the resulting state.
func PlayBestMove(ctx context.Context, baseURL string, id int64, depth int) (Move, State, error) {
	ms, err := GetStateByID(ctx, baseURL, id)
	if err != nil {
		return Move{}, State{}, err
	}

	st := ms.GameState
	if st.Winner != None {
		return Move{}, st, errors.New("game already finished")
	}
	if ms.Role == None {
		return Move{}, st, errors.New("invalid role for id")
	}
	if st.ToMove != ms.Role {
		return Move{}, st, errors.New("not your turn")
	}

	// IMPORTANT: when location == -1 the branching factor is huge; the search can take a long time.
	// Use context-bounded search so we don't "freeze" past action timeout.
	mv, ok := BestMoveCtx(ctx, st, depth)
	if !ok {
		return Move{}, st, errors.New("no legal moves")
	}
	// Ensure we send the correct player (backend validates it).
	mv.Player = ms.Role

	next, err := SendMove(ctx, baseURL, id, mv)
	if err != nil {
		return Move{}, State{}, err
	}
	return mv, next, nil
}
