//go:build sveltego

package routes

type LayoutData struct {
	Title string `json:"title"`
	Theme string `json:"theme"`
}

func Load() (LayoutData, error) {
	return LayoutData{}, nil
}
