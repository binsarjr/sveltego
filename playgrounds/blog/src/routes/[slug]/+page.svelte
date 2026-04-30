<script lang="go"></script>

<article>
  <h1>{data.Title}</h1>
  <p><small>{data.Date}</small></p>
  {@html data.HTML}
</article>

<section>
  <h2>Comments ({len(data.Comments)})</h2>

  {#if len(data.Comments) == 0}
    <p>No comments yet. Be the first.</p>
  {:else}
    <ul>
      {#each data.Comments as c}
        <li>
          <strong>{c.Author}</strong>
          <small> — {c.Posted}</small>
          <p>{c.Body}</p>
        </li>
      {/each}
    </ul>
  {/if}

  <form method="post">
    {#if data.Form != nil}
      <p style="color:crimson">comment was rejected — please fill in name and body.</p>
    {/if}
    <p>
      <label>name <input name="author" required /></label>
    </p>
    <p>
      <label>comment
        <textarea name="body" required rows="3" cols="40"></textarea>
      </label>
    </p>
    <button type="submit">post comment</button>
  </form>
</section>

<p><a href="/">back to index</a></p>
