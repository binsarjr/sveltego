<!--
  Root layout for the blog. Provides shared chrome (header + nav) so
  pages render inside one consistent shell. Layout chain rendering
  works as of v0.2 (#24).
-->
<header>
  <a href="/"><strong>sveltego blog</strong></a>
  <span> — markdown posts, comments, pagination</span>
</header>
<main>
  <slot />
</main>
<footer>
  <small>powered by sveltego — codegen, not interpretation</small>
</footer>
