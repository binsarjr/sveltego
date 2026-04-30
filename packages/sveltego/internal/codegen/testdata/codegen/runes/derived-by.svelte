<script lang="go">
	let count = $state(0)
	let summary = $derived.by(func() string {
		if count == 0 {
			return "empty"
		}
		return "items"
	})
</script>
<p>{summary}</p>
