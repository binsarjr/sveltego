package fixture

type PageData struct {
	Title string
}

func Load() (PageData, error) {
	return PageData{Title: "home"}, nil
}
