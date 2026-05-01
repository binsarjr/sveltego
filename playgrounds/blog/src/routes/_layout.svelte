<!--
  Root layout for the blog. Provides shared chrome (header + nav) so
  pages render inside one consistent shell. Layout chain rendering
  works as of v0.2 (#24).
-->
<header class="border-b border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900">
  <div class="mx-auto max-w-3xl px-4 py-4 flex items-center justify-between">
    <div>
      <a href="/" class="text-xl font-bold text-gray-900 dark:text-gray-100 hover:text-indigo-600 dark:hover:text-indigo-400">sveltego blog</a>
      <span class="ml-2 text-sm text-gray-500 dark:text-gray-400">— markdown posts, comments, pagination</span>
    </div>
    <button
      id="dark-toggle"
      type="button"
      class="rounded-md px-3 py-1.5 text-sm font-medium bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-700 transition-colors"
      onclick="document.documentElement.classList.toggle('dark')"
    >
      toggle dark
    </button>
  </div>
</header>

<main class="mx-auto max-w-3xl px-4 py-8">
  <slot />
</main>

<footer class="border-t border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 mt-12">
  <div class="mx-auto max-w-3xl px-4 py-4 text-center text-sm text-gray-500 dark:text-gray-400">
    powered by sveltego — codegen, not interpretation
  </div>
</footer>
