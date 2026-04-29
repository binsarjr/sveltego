package fixture

func Load() (struct {
	Name string
	Age  int
}, error,
) {
	return struct {
		Name string
		Age  int
	}{Name: "ada", Age: 36}, nil
}
