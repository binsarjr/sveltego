package parser

import (
	"strings"
	"testing"
)

var benchSource = buildBenchSource()

// buildBenchSource assembles a representative ~50KB .svelte template that
// exercises elements, attributes, mustaches, blocks, and a script island.
// Used by BenchmarkParser as a measurement; not gated as a regression
// check.
func buildBenchSource() []byte {
	const target = 50 * 1024
	chunk := `<section class="card" data-id="42">
  <h2>{Data.Title}</h2>
  <p>{Data.Body}</p>
  {#if Data.ShowList}
    <ul>
      {#each Data.Items as item, i (item.ID)}
        <li class:active={item.Hot}>{i}: {item.Name} ({len(item.Tags)})</li>
      {/each}
    </ul>
  {:else}
    <p>nothing yet</p>
  {/if}
</section>
`
	scriptIsland := "<script lang=\"go\">\n" +
		"  var Count = 0\n" +
		"  var Doubled = Count * 2\n" +
		"</script>\n"
	var b strings.Builder
	b.Grow(target + len(chunk))
	b.WriteString(scriptIsland)
	for b.Len() < target {
		b.WriteString(chunk)
	}
	return []byte(b.String())
}

func BenchmarkParser(b *testing.B) {
	b.SetBytes(int64(len(benchSource)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		frag, errs := Parse(benchSource)
		if len(errs) != 0 {
			b.Fatalf("parse errors: %v", errs)
		}
		if frag == nil {
			b.Fatal("nil fragment")
		}
	}
}
