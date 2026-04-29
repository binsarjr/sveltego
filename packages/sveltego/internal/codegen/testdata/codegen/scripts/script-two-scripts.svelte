<script context="module" lang="go">
import "strings"

func upper(s string) string { return strings.ToUpper(s) }
</script>
<script lang="go">
import "strconv"

func count(n int) string { return strconv.Itoa(n) }
</script>
<p>{upper(count(2))}</p>
