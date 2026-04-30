<script lang="go">
	let { Value = $bindable(0), Label } = $props()
</script>
<label>{props.Label}: {props.Value}</label>
