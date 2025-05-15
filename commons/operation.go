package commons

type Operation struct {
	Type     string `json:"type"`
	Position int    `json:"position"`
	Value    string `json:"value"`
}
