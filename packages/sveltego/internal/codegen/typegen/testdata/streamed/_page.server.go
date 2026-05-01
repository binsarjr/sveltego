//go:build sveltego

package routes

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

type Post struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func Load() (struct {
	SlowPosts kit.Streamed[Post] `json:"slowPosts"`
	Count     int                `json:"count"`
}, error,
) {
	return struct {
		SlowPosts kit.Streamed[Post] `json:"slowPosts"`
		Count     int                `json:"count"`
	}{}, nil
}
