<script>
  let { type = 'text', name, value = '', placeholder = '', required = false } = $props();
</script>

<input
  {type}
  {name}
  {value}
  {placeholder}
  {required}
  class="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
/>
