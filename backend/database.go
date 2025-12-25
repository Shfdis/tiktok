package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func SetUpDatabase() (*sql.DB, error) {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "data.db"
	}
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return db, err
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS games (
			cross_id INTEGER,
			circle_id INTEGER,
			state TEXT,
			PRIMARY KEY (cross_id, circle_id));`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func CreateGame(db *sql.DB) (*State, int64, int64, error) {
	emptyLocal := LocalState{Winner: None}
	for i := range 3 {
		for j := range 3 {
			emptyLocal.Values[i][j] = None
		}
	}
	// IMPORTANT: Winner must start as None, otherwise the game is considered already finished
	// and all moves will be rejected with "Game already finished".
	state := State{ToMove: Cross, Winner: None}
	for i := range 3 {
		for j := range 3 {
			state.Values[i][j] = emptyLocal
		}
	}
	state.Location = -1

	// Seed PRNG from time to avoid repeating ID sequences across restarts.
	seed := uint64(time.Now().UnixNano())
	r := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))

	// Generate IDs within JS-safe integer range (<= 2^53-1) so browsers can round-trip them.
	// Otherwise the frontend can lose precision and later query a different id, causing "Not a valid game".
	const maxSafeJSInt int64 = (1 << 53) - 1

	// Generate IDs, ensuring they're not zero
	var crossId, circleId int64
	for crossId == 0 {
		crossId = r.Int64N(maxSafeJSInt-1) + 1
	}
	for circleId == 0 || circleId == crossId {
		circleId = r.Int64N(maxSafeJSInt-1) + 1
	}

	stateString, err := json.Marshal(state)
	if err != nil {
		return &state, 0, 0, err
	}
	result, err := db.Exec(`INSERT INTO games(cross_id, circle_id, state) VALUES (?, ?, ?)`, crossId, circleId, string(stateString))
	if err != nil {
		// Log the error for debugging
		errorMsg := fmt.Sprintf("ERROR: Failed to insert game: %v (crossId: %d, circleId: %d, stateLen: %d)\n", err, crossId, circleId, len(stateString))
		os.Stderr.Write([]byte(errorMsg))
		log.Printf("Failed to insert game: %v (crossId: %d, circleId: %d)", err, crossId, circleId)
		return &state, 0, 0, err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		errorMsg := fmt.Sprintf("ERROR: INSERT succeeded but 0 rows affected (crossId: %d, circleId: %d)\n", crossId, circleId)
		os.Stderr.Write([]byte(errorMsg))
		log.Printf("INSERT succeeded but 0 rows affected")
		return &state, 0, 0, errors.New("insert succeeded but affected 0 rows")
	}
	return &state, crossId, circleId, nil
}
func SelectOneRow(transaction *sql.Tx, query string, args ...interface{}) (*string, error) {
	rows, err := transaction.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	var row string
	err = rows.Scan(&row)
	if err != nil {
		return nil, err
	}
	return &row, nil
}
func SelectOneRowDB(db *sql.DB, query string, args ...interface{}) (*string, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	var row string
	err = rows.Scan(&row)
	if err != nil {
		return nil, err
	}
	return &row, nil
}
func GetState(transaction *sql.Tx, id int64) (*State, Player, error) {
	stateCircle, err := SelectOneRow(transaction, "SELECT state FROM games WHERE circle_id = ?", id)
	if err != nil {
		return nil, None, err
	}
	var stateCross *string
	stateCross, err = SelectOneRow(transaction, "SELECT state FROM games WHERE cross_id = ?", id)
	if err != nil {
		return nil, None, err
	}
	var result State
	if stateCircle != nil {
		err = json.Unmarshal([]byte(*stateCircle), &result)
		if err != nil {
			return nil, None, err
		}
		return &result, Circle, nil
	} else if stateCross != nil {
		err = json.Unmarshal([]byte(*stateCross), &result)
		if err != nil {
			return nil, None, err
		}
		return &result, Cross, nil
	}
	return nil, None, errors.New("Not a valid game")
}
func GetStateDB(db *sql.DB, id int64) (*State, Player, error) {
	stateCircle, err := SelectOneRowDB(db, "SELECT state FROM games WHERE circle_id = ?", id)
	if err != nil {
		return nil, None, err
	}
	var stateCross *string
	stateCross, err = SelectOneRowDB(db, "SELECT state FROM games WHERE cross_id = ?", id)
	if err != nil {
		return nil, None, err
	}
	var result State
	if stateCircle != nil {
		err = json.Unmarshal([]byte(*stateCircle), &result)
		if err != nil {
			return nil, None, err
		}
		return &result, Circle, nil
	} else if stateCross != nil {
		err = json.Unmarshal([]byte(*stateCross), &result)
		if err != nil {
			return nil, None, err
		}
		return &result, Cross, nil
	}
	return nil, None, errors.New("Not a valid game")
}
func MakeMove(db *sql.DB, id int64, move Move) (*State, error) {
	transaction, err := db.Begin()
	if err != nil {
		return &State{}, err
	}
	var state *State
	var player Player
	state, player, err = GetState(transaction, id)
	if err != nil {
		transaction.Rollback()
		return nil, err
	}
	if state.ToMove != player {
		transaction.Rollback()
		return nil, errors.New("Not your move")
	}
	*state, err = PerformMove(*state, move)
	if err != nil {
		transaction.Rollback()
		return nil, err
	}
	var stateString []byte
	stateString, err = json.Marshal(*state)
	if err != nil {
		transaction.Rollback()
		return nil, err
	}
	if player == Circle {
		_, err = transaction.Exec(`UPDATE games SET state = ? WHERE circle_id = ?`, string(stateString), id)
	} else {
		_, err = transaction.Exec(`UPDATE games SET state = ? WHERE cross_id = ?`, string(stateString), id)
	}
	if err != nil {
		transaction.Rollback()
		return nil, err
	}
	err = transaction.Commit()
	if err != nil {
		return nil, err
	}
	return state, nil
}
func CleanupDatabase(db *sql.DB) {
	db.Close()
}

func ClearGames(db *sql.DB) error {
	_, err := db.Exec(`DELETE FROM games;`)
	return err
}
