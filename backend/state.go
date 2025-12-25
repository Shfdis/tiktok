package main

type Player int

const (
	Cross = iota
	Circle
	None
)

type LocalState struct {
	Values [3][3]Player `json:"values"`
	Winner Player       `json:"winner"`
}
type State struct {
	Values   [3][3]LocalState `json:"values"`
	ToMove   Player           `json:"to_move"`
	Location int              `json:"location"`
	Winner   Player           `json:"winner"`
}
type PlayerGettable interface {
	Get(a int, b int) Player
}

func (this State) Get(a int, b int) Player {
	if a >= 3 || b >= 3 {
		return None
	}
	return this.Values[a][b].Winner
}
func (this LocalState) Get(a int, b int) Player {
	if a >= 3 || b >= 3 {
		return None
	}
	return this.Values[a][b]
}
func GetWinner(this PlayerGettable) Player {
	for i := 0; i < 3; i++ {
		if this.Get(i, 0) != None {
			yes := true
			for j := 0; j < 3; j++ {
				value := this.Get(i, j)
				if value != this.Get(i, 0) {
					yes = false
				}
			}
			if yes {
				return this.Get(i, 0)
			}
		}
		if this.Get(0, i) != None {
			yes := true
			for j := 0; j < 3; j++ {
				if this.Get(j, i) != this.Get(0, i) {
					yes = false
				}
			}
			if yes {
				return this.Get(0, i)
			}
		}
	}
	yes := true
	if this.Get(0, 0) != None {
		for i := 0; i < 3; i++ {
			if this.Get(i, i) != this.Get(0, 0) {
				yes = false
				break
			}
		}
		if yes {
			return this.Get(0, 0)
		}
	}
	yes = true
	if this.Get(2, 0) != None {
		for i := 0; i < 3; i++ {
			if this.Get(2-i, i) != this.Get(2, 0) {
				yes = false
				break
			}
		}
		if yes {
			return this.Get(2, 0)
		}
	}
	return None
}
func (this *LocalState) Update() {
	this.Winner = GetWinner(this)
}
func (this *State) Update() {
	this.Winner = GetWinner(this)
}
