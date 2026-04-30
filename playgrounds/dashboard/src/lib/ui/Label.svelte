<script>
  let { for: htmlFor = '', children } = $props();
</script>

<label for={htmlFor} class="block text-sm font-medium text-gray-700 mb-1">
  {@render children?.()}
</label>
