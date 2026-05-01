//go:build sveltego

package routes

import "time"

func Load() (struct {
	CreatedAt time.Time  `json:"createdAt"`
	Updated   *time.Time `json:"updated"`
}, error,
) {
	return struct {
		CreatedAt time.Time  `json:"createdAt"`
		Updated   *time.Time `json:"updated"`
	}{}, nil
}
