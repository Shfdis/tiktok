package main

const (
	abInf = int(1e9)
)

func countRow(value PlayerGettable, line int, player Player) int {
	ans := 0
	for i := range 3 {
		if value.Get(line, i) == player {
			ans++
		} else if value.Get(line, i) == 1-player {
			ans--
		}
	}
	return ans
}
func countColumn(value PlayerGettable, column int, player Player) int {
	ans := 0
	for i := range 3 {
		if value.Get(i, column) == player {
			ans++
		} else if value.Get(i, column) == 1-player {
			ans--
		}
	}
	return ans
}
func evalLocal(value PlayerGettable, player Player) int {
	ans := 0
	countDiagonal := 0
	countSideDiagonal := 0
	for i := range 3 {
		switch value := countColumn(value, i, player); value {
		case 2:
			ans++
		case -2:
			ans--
		}
		switch value := countRow(value, i, player); value {
		case 2:
			ans++
		case -2:
			ans--
		}
		if value.Get(i, i) == player {
			countDiagonal++
		} else if value.Get(i, i) == 1-player {
			countDiagonal--
		}
		if value.Get(i, 2-i) == player {
			countSideDiagonal++
		} else if value.Get(i, 2-i) == 1-player {
			countSideDiagonal--
		}
	}
	if countDiagonal == 2 {
		ans++
	}
	if countDiagonal == -2 {
		ans--
	}
	if countSideDiagonal == 2 {
		ans++
	}
	if countSideDiagonal == -2 {
		ans--
	}
	return ans
}

// EvaluateFor returns a heuristic evaluation from the perspective of player (higher is better for player).
func EvaluateFor(state State, player Player) int {
	if state.Winner == player {
		return abInf
	}
	if state.Winner == 1-player {
		return -abInf
	}

	answer := 8 * evalLocal(state, player) // global alignment
	for i := range 3 {                     // local occupancy + local threats
		for j := range 3 {
			if player == state.Get(i, j) {
				answer += 8
			} else if player == 1-state.Get(i, j) {
				answer -= 8
			} else {
				answer += evalLocal(state.Values[i][j], player)
			}
		}
	}
	return answer
}

// evaluate is kept for backwards compatibility: it evaluates from the perspective of state.ToMove.
func evaluate(state State) int {
	return EvaluateFor(state, state.ToMove)
}
