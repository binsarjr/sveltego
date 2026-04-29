<script lang="go">
import (
	"fmt"
	"strconv"
)

type Item struct {
	Name string
	Qty  int
}

func format(it Item) string {
	return fmt.Sprintf("%s x%s", it.Name, strconv.Itoa(it.Qty))
}
</script>
<p>{format(Item{Name: "widget", Qty: 3})}</p>
