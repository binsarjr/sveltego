//go:build sveltego

package routes

func Load() (struct {
	Greeting  string `json:"greeting"`
	UserCount int    // no tag — falls back to lowerFirst
	Skipped   string `json:"-"`
	Renamed   string `json:"display_name,omitempty"`
}, error,
) {
	return struct {
		Greeting  string `json:"greeting"`
		UserCount int
		Skipped   string `json:"-"`
		Renamed   string `json:"display_name,omitempty"`
	}{}, nil
}
