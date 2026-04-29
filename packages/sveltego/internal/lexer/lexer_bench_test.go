package lexer

import (
	"strings"
	"testing"
)

var benchSource = buildBenchSource()

// buildBenchSource assembles a ~100KB synthetic .svelte resembling a real
// page: nested elements, attributes, mustache expressions, blocks, and a
// script island. Used by BenchmarkLexer to track the <5ms target from
// issue #7. The source is not gated as a regression check; it is a
// measurement.
func buildBenchSource() []byte {
	const target = 100 * 1024
	chunk := `<section class="card" data-id="42">
  <h2>{Data.Title}</h2>
  <p>{Data.Body}</p>
  {#if Data.ShowList}
    <ul>
      {#each Data.Items as item, i}
        <li class={item.Class}>{i}: {item.Name} ({len(item.Tags)})</li>
      {/each}
    </ul>
  {:else}
    <p>nothing yet</p>
  {/if}
</section>
`
	scriptIsland := "<script>\n" +
		"  let count = $state(0);\n" +
		"  let doubled = $derived(count * 2);\n" +
		"</script>\n"
	var b strings.Builder
	b.Grow(target + len(chunk))
	b.WriteString(scriptIsland)
	for b.Len() < target {
		b.WriteString(chunk)
	}
	return []byte(b.String())
}

func BenchmarkLexer(b *testing.B) {
	b.SetBytes(int64(len(benchSource)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l := New(benchSource)
		for {
			t := l.Next()
			if t.Kind == TokenEOF {
				break
			}
		}
	}
}
