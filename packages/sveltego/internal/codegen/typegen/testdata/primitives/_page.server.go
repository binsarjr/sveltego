//go:build sveltego

package routes

func Load() (struct {
	Greeting string
	Count    int
	Score    float64
	Active   bool
}, error,
) {
	return struct {
		Greeting string
		Count    int
		Score    float64
		Active   bool
	}{}, nil
}
