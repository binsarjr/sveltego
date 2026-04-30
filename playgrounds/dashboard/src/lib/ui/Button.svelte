<script>
  let { type = 'button', variant = 'primary', children } = $props();

  const variants = {
    primary: 'bg-blue-600 text-white border-blue-600 hover:bg-blue-700',
    secondary: 'bg-white text-blue-600 border-blue-600 hover:bg-blue-50',
    danger: 'bg-red-600 text-white border-red-600 hover:bg-red-700',
  };
</script>

<button
  {type}
  class="inline-flex items-center rounded-md border px-3 py-1.5 text-sm font-medium cursor-pointer {variants[variant]}"
>
  {@render children?.()}
</button>
