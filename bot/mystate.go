package main

type MyState struct {
	GameState State  `json:"game_state"`
	Role      Player `json:"role"`
	Id        int64  `json:"id"`
}
