//go:build sveltego

package routes

type User struct {
	Name string `json:"name"`
}

func Load() (struct {
	Owner  *User    `json:"owner"`
	Tags   []string `json:"tags"`
	Backup *string  `json:"backup"`
}, error,
) {
	return struct {
		Owner  *User    `json:"owner"`
		Tags   []string `json:"tags"`
		Backup *string  `json:"backup"`
	}{}, nil
}
