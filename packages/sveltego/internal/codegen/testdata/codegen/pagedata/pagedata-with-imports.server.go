package fixture

import "time"

func Load() (struct {
	CreatedAt time.Time
}, error,
) {
	return struct {
		CreatedAt time.Time
	}{CreatedAt: time.Now()}, nil
}
