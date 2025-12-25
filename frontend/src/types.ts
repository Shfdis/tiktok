export type Player = 0 | 1 | 2; // Cross=0, Circle=1, None=2

export type LocalState = {
  values: Player[][];
  winner: Player;
};

export type State = {
  values: LocalState[][];
  to_move: Player;
  location: number;
  winner: Player;
};

export type MyState = {
  game_state: State;
  role: Player;
  id: number;
};

export type Move = {
  player: Player;
  cellX: number;
  cellY: number;
  finalX: number;
  finalY: number;
};

export function playerLabel(p: Player): "X" | "O" | "-" {
  if (p === 0) return "X";
  if (p === 1) return "O";
  return "-";
}


