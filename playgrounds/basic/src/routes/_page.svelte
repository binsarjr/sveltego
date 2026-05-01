<script lang="go"></script>

<h1>{data.Greeting}</h1>

{#if len(data.Posts) > 0}
  <ul>
    {#each data.Posts as p}
      <li><a href={"/post/" + p.ID}>{p.Title}</a></li>
    {/each}
  </ul>
{:else}
  <p>No posts.</p>
{/if}
