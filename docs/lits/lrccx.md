# lrc.cx API

Public lyrics API from the LrcAPI project. The legacy endpoint is served at
<https://api.lrc.cx>; the CLI uses its batch-search API.

Reference: <https://docs.lrc.cx/docs/legacy/lyrics/> (retrieved 2026-10-24).
Chinese-language documentation.

## Endpoints

### `GET /jsonapi`

Returns multiple matches as JSON, ranked by match score in descending order.

| Parameter | Required | Type   | Description                                                  |
| --------- | -------- | ------ | ------------------------------------------------------------ |
| `title`   | yes      | string | Song title                                                    |
| `artist`  | no       | string | Artist name; supports multiple separators                     |
| `album`   | no       | string | Album name; the literal `[Unknown Album]` is treated as empty |
| `path`    | no       | string | Local music-file path, used to read `.lrc` files and tags      |

The `path` parameter does not apply to the CLI and is never sent.

### `GET /lyrics`

Returns the single best match as raw LRC text. The documentation lists
`text/html`; live responses use `text/plain`. The CLI does not use this
endpoint.

## Response

A `200` response from `/jsonapi` is a JSON array; an empty or missing result may
instead produce `404` with `{"message": "未找到歌词"}` ("lyrics not found").

Each array entry contains:

| Field         | Type           | Description                                             |
| ------------- | -------------- | ------------------------------------------------------- |
| `id`          | string         | Lyrics ID (hash of the LRC text)                        |
| `title`       | string         | Track title                                             |
| `artist`      | string         | Artist name                                             |
| `album`       | string         | Album name                                              |
| `lrc`         | string \| null | LRC lyrics text; may be `null` for instrumental/missing entries |
| `score`       | number         | Match score; entries are sorted descending              |
| `from`        | string         | Upstream provider, e.g. `Netease`                        |
| `cover`       | string         | Cover image URL                                          |
| `hash`        | string         | Content hash                                             |
| `timestamp`   | number         | Record timestamp                                         |
| `create_time` | string         | ISO-8601 creation time                                   |

### Documentation discrepancy

The legacy documentation lists the lyrics text under a `lyrics` field. The live
`/jsonapi` response uses `lrc` instead, and the CLI reads `lrc`. Treat the
documentation field name as outdated.

## Notes

- No authentication is documented.
- The service is a community mirror/aggregator; `from` identifies the upstream
  source.
- No rate limit is documented.

## Self-hosting

Yes. GPL-3.0 licensed.
