<div class="error">
  <h1>error {data.Code}</h1>
  <p>{data.Message}</p>
  {#if data.ID != ""}<small>ref {data.ID}</small>{/if}
</div>
