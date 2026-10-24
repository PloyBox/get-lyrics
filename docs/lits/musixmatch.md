# Musixmatch API

Commercial lyrics and music-metadata API. Every call is a `GET` under
`https://api.musixmatch.com/ws/1.1/` and requires an API key.

References:

- <https://docs.musixmatch.com/> (retrieved 2026-10-24)
- <https://developer.musixmatch.com/documentation>

## Authentication

Pass the key as the `apikey` query parameter on every request:

```http
GET https://api.musixmatch.com/ws/1.1/track.get?commontrack_id=5920049&apikey=YOUR_KEY
```

Keys are issued through the Musixmatch developer portal. Subtitle endpoints
require a plan that includes subtitle access; restricted plans receive `402` or
`403`.

## Endpoints used by the CLI

| Endpoint                 | Purpose                                      | Key parameters |
| ------------------------ | -------------------------------------------- | -------------- |
| `matcher.lyrics.get`     | Fuzzy lyrics lookup by title and artist      | `q_track`, `q_artist`, `track_isrc` |
| `matcher.subtitle.get`   | Fuzzy synced-lyrics lookup by title/artist   | `q_track`, `q_artist`, `track_isrc`, `f_subtitle_length`, `f_subtitle_length_max_deviation` |
| `track.get`              | Track metadata by ID or ISRC                 | one of `commontrack_id`, `track_isrc`, `track_spotify_id`, `track_itunes_id` |
| `track.search`           | Search tracks by title/artist/lyrics         | `q_track`, `q_artist`, `q_lyrics`, `q`, `f_has_lyrics`, `s_track_rating`, `page`, `page_size` |
| `track.lyrics.get`       | Plain lyrics by track identifier             | `commontrack_id` or `track_isrc` (also Spotify/iTunes IDs) |
| `track.subtitle.get`     | Synced lyrics (LRC or DFXP) by identifier    | `commontrack_id`, `track_id` or `track_isrc`; `subtitle_format`, `f_subtitle_length`, `f_subtitle_length_max_deviation` |

Additional shared parameters include `page`/`page_size` (default 10, range
1-100), `s_track_rating` (`ASC`/`DESC`), and `subtitle_format` (`LRC`/`DFXP`,
default `LRC`).

## Response envelope

All endpoints return a JSON envelope; failures may still arrive with HTTP 200,
so read the body status code:

```json
{
  "message": {
    "header": {
      "status_code": 200,
      "execute_time": 0.0148
    },
    "body": {
      "lyrics": {
        "lyrics_body": "...",
        "lyrics_copyright": "...",
        "explicit": 1
      }
    }
  }
}
```

Common body fields:

| Object       | Fields |
| ------------ | ------ |
| `track`      | `track_name`, `artist_name`, `album_name`, `commontrack_id`, `has_lyrics`, `has_subtitles`, `instrumental` |
| `lyrics`     | `lyrics_body`, `lyrics_copyright`, `explicit`, `lyrics_language` |
| `subtitle`   | `subtitle_body`, `subtitle_length`, `subtitle_language` |
| `track_list` | Array of `{ "track": { ... } }` entries from `track.search` |

`lyrics_body` is the lyrics text; it may include the API's usage notice, which
the CLI strips. The CLI treats an empty body, `[]`, or `{}` on a successful
response as a no-match signal.

## Status codes

| Code | Meaning |
| ---- | ------- |
| 200  | Success |
| 400  | Bad syntax or impossible request |
| 401  | Authentication failed (missing/invalid key) |
| 402  | Usage limit reached or insufficient balance |
| 403  | Operation not authorized for the plan |
| 404  | Resource not found |
| 405  | Method not found |
| 500  | Server error |
| 503  | Temporarily busy |

## Usage terms

The API requires compliance with Musixmatch content restrictions (country
availability) and lyrics-view tracking rules. See the official
[content restrictions](https://docs.musixmatch.com/content-restrictions) and
[lyrics views tracking](https://docs.musixmatch.com/lyrics-views-tracking)
pages.

## Self-hosting

No. Proprietary commercial license.
