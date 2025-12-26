package main

import (
	"context"
	"sort"
)

func localHasEmpty(local LocalState) bool {
	for x := range 3 {
		for y := range 3 {
			if local.Values[x][y] == None {
				return true
			}
		}
	}
	return false
}

func localPlayable(local LocalState) bool {
	return local.Winner == None && localHasEmpty(local)
}

// LegalMoves generates all legal moves for state.ToMove.
func LegalMoves(state State) []Move {
	moves := make([]Move, 0, 81)
	player := state.ToMove

	// Determine which local boards are allowed (forced location unless that board is not playable).
	allowed := make([][2]int, 0, 9)
	if state.Location != -1 {
		cx, cy := state.Location/3, state.Location%3
		if localPlayable(state.Values[cx][cy]) {
			allowed = append(allowed, [2]int{cx, cy})
		} else {
			for i := range 3 {
				for j := range 3 {
					if localPlayable(state.Values[i][j]) {
						allowed = append(allowed, [2]int{i, j})
					}
				}
			}
		}
	} else {
		for i := range 3 {
			for j := range 3 {
				if localPlayable(state.Values[i][j]) {
					allowed = append(allowed, [2]int{i, j})
				}
			}
		}
	}

	for _, cell := range allowed {
		cx, cy := cell[0], cell[1]
		local := state.Values[cx][cy]
		for fx := range 3 {
			for fy := range 3 {
				if local.Values[fx][fy] == None {
					moves = append(moves, Move{
						Player: player,
						CellX:  cx,
						CellY:  cy,
						FinalX: fx,
						FinalY: fy,
					})
				}
			}
		}
	}

	return moves
}

type scoredChild struct {
	mv    Move
	next  State
	score int
}

func orderChildrenCtx(ctx context.Context, state State, moves []Move, root Player, maximizing bool) []scoredChild {
	children := make([]scoredChild, 0, len(moves))
	for _, mv := range moves {
		select {
		case <-ctx.Done():
			// Return whatever we've collected so far.
			return children
		default:
		}
		next, err := PerformMove(state, mv)
		if err != nil {
			continue
		}
		s := EvaluateFor(next, root)
		children = append(children, scoredChild{mv: mv, next: next, score: s})
	}
	if maximizing {
		sort.Slice(children, func(i, j int) bool { return children[i].score > children[j].score })
	} else {
		sort.Slice(children, func(i, j int) bool { return children[i].score < children[j].score })
	}
	return children
}

func alphaBeta(state State, depth int, alpha int, beta int, root Player) int {
	return alphaBetaCtx(context.Background(), state, depth, alpha, beta, root)
}

func alphaBetaCtx(ctx context.Context, state State, depth int, alpha int, beta int, root Player) int {
	moves := LegalMoves(state)
	return alphaBetaPlyCtx(ctx, state, depth, alpha, beta, root, 0, moves)
}

func alphaBetaPlyCtx(ctx context.Context, state State, depth int, alpha int, beta int, root Player, ply int, moves []Move) int {
	select {
	case <-ctx.Done():
		return EvaluateFor(state, root)
	default:
	}

	// Terminal: win/loss with mate distance (prefer fast win / slow loss).
	if state.Winner == root {
		return abInf - ply
	}
	if state.Winner == 1-root {
		return -(abInf - ply)
	}
	// Terminal: no moves and no winner -> draw.
	if len(moves) == 0 {
		return 0
	}
	// Leaf: heuristic.
	if depth == 0 {
		return EvaluateFor(state, root)
	}

	if state.ToMove == root {
		// Maximize for root
		children := orderChildrenCtx(ctx, state, moves, root, true)
		best := -abInf
		for _, ch := range children {
			select {
			case <-ctx.Done():
				return EvaluateFor(state, root)
			default:
			}
			nextMoves := LegalMoves(ch.next)
			score := alphaBetaPlyCtx(ctx, ch.next, depth-1, alpha, beta, root, ply+1, nextMoves)
			if score > best {
				best = score
			}
			if score > alpha {
				alpha = score
			}
			if alpha >= beta {
				break
			}
		}
		return best
	}

	// Minimize for root
	children := orderChildrenCtx(ctx, state, moves, root, false)
	best := abInf
	for _, ch := range children {
		select {
		case <-ctx.Done():
			return EvaluateFor(state, root)
		default:
		}
		nextMoves := LegalMoves(ch.next)
		score := alphaBetaPlyCtx(ctx, ch.next, depth-1, alpha, beta, root, ply+1, nextMoves)
		if score < best {
			best = score
		}
		if score < beta {
			beta = score
		}
		if alpha >= beta {
			break
		}
	}
	return best
}

// BestMove returns the best move for state.ToMove using alpha-beta search.
func BestMove(state State, depth int) (Move, bool) {
	moves := LegalMoves(state)
	if len(moves) == 0 {
		return Move{}, false
	}

	root := state.ToMove
	bestScore := -abInf
	bestMove := moves[0]
	found := false
	for _, mv := range moves {
		next, err := PerformMove(state, mv)
		if err != nil {
			continue
		}
		nextMoves := LegalMoves(next)
		score := alphaBetaPlyCtx(context.Background(), next, depth-1, -abInf, abInf, root, 1, nextMoves)
		if !found || score > bestScore {
			bestScore = score
			bestMove = mv
			found = true
		}
	}
	return bestMove, found
}

// BestMoveCtx is a cancellation/time-bounded variant. It returns the best move found so far if ctx is cancelled.
func BestMoveCtx(ctx context.Context, state State, depth int) (Move, bool) {
	moves := LegalMoves(state)
	if len(moves) == 0 {
		return Move{}, false
	}
	root := state.ToMove

	// Order root moves for better early pruning and better "best so far" when time runs out.
	children := orderChildrenCtx(ctx, state, moves, root, true)
	if len(children) == 0 {
		// If we got cancelled very early, still return a legal move.
		return moves[0], true
	}

	bestScore := -abInf
	bestMove := children[0].mv
	for _, ch := range children {
		select {
		case <-ctx.Done():
			return bestMove, true
		default:
		}
		nextMoves := LegalMoves(ch.next)
		score := alphaBetaPlyCtx(ctx, ch.next, depth-1, -abInf, abInf, root, 1, nextMoves)
		if score > bestScore {
			bestScore = score
			bestMove = ch.mv
		}
		if bestScore == abInf {
			break
		}
	}
	return bestMove, true
}

func evalGlobal(state State, recursion int) State {
	if recursion <= 0 || state.Winner != None {
		return state
	}
	mv, ok := BestMove(state, recursion)
	if !ok {
		return state
	}
	next, err := PerformMove(state, mv)
	if err != nil {
		return state
	}
	return next
}
