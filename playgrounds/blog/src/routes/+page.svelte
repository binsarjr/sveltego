<script lang="go"></script>

<h1>Posts</h1>

{#if len(data.Posts) == 0}
  <p>No posts on this page.</p>
{:else}
  <ul>
    {#each data.Posts as p}
      <li>
        <a href={"/" + p.Slug}>{p.Title}</a>
        <small> — {p.Summary}</small>
      </li>
    {/each}
  </ul>
{/if}

<nav>
  {#if data.HasPrev}
    <a href={data.PrevHref}>prev</a>
  {/if}
  <span> page {data.Page} of {data.TotalPages} </span>
  {#if data.HasNext}
    <a href={data.NextHref}>next</a>
  {/if}
</nav>
