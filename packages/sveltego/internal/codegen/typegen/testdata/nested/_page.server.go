//go:build sveltego

package routes

type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type Post struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type PageData struct {
	User  User   `json:"user"`
	Posts []Post `json:"posts"`
}

func Load() (PageData, error) {
	return PageData{}, nil
}
