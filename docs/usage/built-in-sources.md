# Built-in Sources

Legend: `Y` = supported, `N` = not supported, `F` = required (must be given). Sync level lists the supported levels in the form `line,word,none`.

| Source | Author | Album | ISRC | Duration | Sync level | Notes |
|--------|--------|-------|------|----------|-----------|-------|
| `lrclib` | Y | Y[^1] | N | Y[^1] | line,none | Searches [lrclib.net](https://lrclib.net); uses `/api/get` when artist is given, `/api/search` otherwise |
| `lyricsovh` | F | N | N | N | none | Uses [api.lyrics.ovh](https://api.lyrics.ovh); plain text only |
| `lrccx` | Y | Y | N | N | line,none | Searches [lrc.cx](https://lrc.cx) via its legacy `/jsonapi` endpoint; the response is always LRC-flavoured text, stripped of timestamps for plain output |
| `musixmatch` | Y[^2] | N | Y | N | line[^3],none | [Musixmatch](https://developer.musixmatch.com) via `track.get` (with `--isrc`), `matcher.lyrics.get`/`matcher.subtitle.get` (with `--author`) or `track.search` → `track.lyrics.get`/`track.subtitle.get` (title only). Requires the `MUSIXMATCH_API_KEY` custom parameter |
| `betterlyrics` | F | Y | N | Y | word,line,none | Aggregate [Better Lyrics API](https://lyrics-api.boidu.dev) source fronting two upstreams: `/ttml/getLyrics` (word-level TTML) or `/kugou/getLyrics` (line-level LRC). The served upstream is reported as the sub-source (`ttml` or `kugou`). Cache-first: uncached songs may 401/429[^4] |

[^1]: The Album and Duration filters only take effect with `--author`; without an author, each is dropped with an unsupported warning.
[^2]: The Author filter does not apply together with `--isrc`; the ISRC alone resolves the track and `--author` is dropped with an unsupported warning.
[^3]: Synced LRC (`line`) needs the paid Scale plan; on cheaper plans a `--sync-level line` request falls back to plain lyrics with a `warning[downgraded]`.
[^4]: The API issues no API keys right now: cached songs need none, but a cache miss can return 401 (key required) or 429 (rate limited). Such songs surface as `warning[fetch]` and fall through to the next source.

`musixmatch` is the only built-in source that declares a custom `--env` parameter: `MUSIXMATCH_API_KEY` (required, fallback to the `MUSIXMATCH_API_KEY` environment variable). Get a free Basic key at <https://developer.musixmatch.com>.