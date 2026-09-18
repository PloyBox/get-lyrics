# lyricsovh

Backed by the public lyrics.ovh API:

```
GET https://api.lyrics.ovh/v1/{artist}/{title}
```

- Returns `{"lyrics": "..."}` on success, or 404 with `{"error": "No lyrics found"}` when no
  match exists.
- Because the path requires an artist, this adapter requires `--author`: a fetch without it
  cannot form a valid request, so the fetch layer enforces the requirement during precheck
  (exit code 6). The adapter does not validate the author itself.
- The API sometimes pairs a 200 with an `error` field; the adapter honors it.
- 10s timeout.
