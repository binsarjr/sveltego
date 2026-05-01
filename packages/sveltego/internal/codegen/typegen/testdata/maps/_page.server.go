//go:build sveltego

package routes

func Load() (struct {
	Counts map[string]int    `json:"counts"`
	Labels map[string]string `json:"labels"`
}, error,
) {
	return struct {
		Counts map[string]int    `json:"counts"`
		Labels map[string]string `json:"labels"`
	}{}, nil
}
