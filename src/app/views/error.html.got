<section class="padded mt-5">
      <h1 class="text-4xl font-medium">{{.title}}</h1>
      <p class="mt-5">{{.message}}</p>
      {{ if .file }}
      <p class="mt-5">File:{{.file}}</p>
      {{ end }}
      {{ if .error }}
       <pre><code>
      Error:{{.error}}
      </code></pre>
      {{ end }}
      {{ if .currentUser.Anon }}
      <a href="/users/login" class="btn mb-5">Login</a>
      {{ end }}
      <a class="btn" type="submit" method="back">Back</a>
</section>