<script lang="go"></script>

<nav>
  <a href="/">Home</a>
  <a href="/dashboard">Dashboard</a>
  <form method="post" action="/?/logout" style="display:inline;margin:0">
    <button type="submit" class="secondary">Logout ({data.Username})</button>
  </form>
</nav>

<h1>Edit item</h1>

{#if data.FlashMsg != ""}
  <p class="error">{data.FlashMsg}</p>
{/if}

<p><strong>ID:</strong> <code>{data.Item.ID}</code></p>
<p><strong>Last updated:</strong> {data.Item.UpdatedAt}</p>

<form method="post" action={"/dashboard/items/" + data.Item.ID + "?/update"}>
  <p>
    <label>Title
      <input type="text" name="title" value={data.Item.Title} required>
    </label>
  </p>
  <p>
    <label>Note
      <input type="text" name="note" value={data.Item.Note}>
    </label>
  </p>
  <button type="submit">Save</button>
</form>

<form method="post" action={"/dashboard/items/" + data.Item.ID + "?/delete"} style="margin-top:1rem">
  <button type="submit" class="danger">Delete</button>
</form>

<p><a href="/dashboard">Back to dashboard</a></p>
