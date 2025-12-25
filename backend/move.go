package main

import "errors"

type Move struct {
	Player Player `json:"player"`
	CellX  int    `json:"cellX"`
	CellY  int    `json:"cellY"`
	FinalX int    `json:"finalX"`
	FinalY int    `json:"finalY"`
}

func PerformMove(current State, move Move) (State, error) {
	if move.Player == None {
		return current, errors.New("Player cannot be none")
	}
	if move.Player != current.ToMove {
		return current, errors.New("Not your turn")
	}
	if move.CellX < 0 || move.CellX >= 3 || move.CellY < 0 || move.CellY >= 3 {
		return current, errors.New("Invalid cell coordinates")
	}
	if move.FinalX < 0 || move.FinalX >= 3 || move.FinalY < 0 || move.FinalY >= 3 {
		return current, errors.New("Invalid final coordinates")
	}
	if current.Location != -1 && (move.CellX != current.Location/3 || move.CellY != current.Location%3) {
		return current, errors.New("Illegal move")
	}
	if current.Get(move.CellX, move.CellY) != None {
		return current, errors.New("Illegal move")
	}
	if current.Values[move.CellX][move.CellY].Get(move.FinalX, move.FinalY) != None {
		return current, errors.New("Illegal move")
	}
	if current.Winner != None {
		return current, errors.New("Game already finished")
	}

	// Place the move
	current.Values[move.CellX][move.CellY].Values[move.FinalX][move.FinalY] = move.Player
	current.Values[move.CellX][move.CellY].Update()

	// Switch turn
	if current.ToMove == Cross {
		current.ToMove = Circle
	} else {
		current.ToMove = Cross
	}

	current.Location = move.FinalX*3 + move.FinalY
	if current.Get(move.FinalX, move.FinalY) != None {
		current.Location = -1
	}
	current.Update()
	return current, nil
}
