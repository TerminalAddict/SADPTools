package platform

type RouteDetail struct {
	Source    string `json:"source"`
	Interface string `json:"interface"`
	Gateway   string `json:"gateway"`
	Prefix    string `json:"prefix"`
	Evidence  string `json:"evidence"`
}
