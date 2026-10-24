# lyrics.ovh API

Public lyrics API at <https://api.lyrics.ovh>. No authentication is documented.

Reference: <https://lyricsovh.docs.apiary.io/> (retrieved 2026-10-24).

## Endpoints

### `GET /v1/{artist}/{title}`

Looks up one song by artist and title. Both path segments are required; the
published API has no title-only lyrics lookup.

| Segment  | Type   | Description      |
| -------- | ------ | ---------------- |
| `artist` | string | Artist name      |
| `title`  | string | Song title       |

Example:

```http
GET https://api.lyrics.ovh/v1/Coldplay/Adventure%20of%20a%20Lifetime
```

### `GET /suggest/{query}`

Undocumented on the Apiary page, but present in the live API. It is a free-text
search that returns Deezer track metadata (`data`, `total`, `next`), not lyrics;
the CLI does not use it.

## Response

| Status | Body                         | Meaning                          |
| ------ | ---------------------------- | -------------------------------- |
| 200    | `{"lyrics": "..."}`          | Lyrics found                     |
| 404    | `{"error": "No lyrics found"}` | No lyrics for that artist/title |

The `lyrics` value is plain text; line breaks are part of the string. The API
may also return HTTP 200 with an `error` field, so callers should check `error`
before using `lyrics`.

No rate limits or usage terms are documented on the API page.

## Self-hosting

Yes. MIT licensed.
