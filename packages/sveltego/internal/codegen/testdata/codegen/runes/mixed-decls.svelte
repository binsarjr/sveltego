<script lang="go">
	import "strconv"

	func twice(n int) string { return strconv.Itoa(n * 2) }

	let { Title } = $props()
	let count = $state(5)
	let label = $derived(twice(count))
</script>
<h1>{props.Title}: {label}</h1>
