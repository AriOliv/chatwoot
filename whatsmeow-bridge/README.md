# whatsmeow-bridge

HTTP bridge between Chatwoot's `whatsmeow` WhatsApp provider and WhatsApp Web
multi-device, built on [AriOliv/whatsmeow](https://github.com/AriOliv/whatsmeow).

One session per Chatwoot inbox (`session_id` = inbox phone number). Sessions are
paired with a QR code or a phone pairing code and stored in Postgres.

## Environment

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | — | Postgres DSN (device store + bridge tables) |
| `WHATSMEOW_BRIDGE_TOKEN` | — | Bearer token Chatwoot uses to call the bridge |
| `LISTEN_ADDR` | `:8080` | HTTP listen address |
| `HISTORY_DAYS` | `30` | How far back history sync is forwarded |
| `LOG_LEVEL` | `INFO` | `DEBUG`, `INFO`, `WARN`, `ERROR` |

## API (Bearer auth)

- `POST /sessions` `{session_id, webhook_url, webhook_secret, import_history}`
- `GET /sessions/{id}/status`, `GET /sessions/{id}/qr`, `POST /sessions/{id}/pair-phone` `{phone}`
- `POST /sessions/{id}/reconnect`, `DELETE /sessions/{id}`
- `POST /sessions/{id}/messages` `{to, type, text, media_url, filename, mime, reply_to_id, interactive, interactive_mode}`
- `POST /sessions/{id}/reactions`, `POST /sessions/{id}/read`
- `GET /sessions/{id}/media/{msg_id}`, `GET /sessions/{id}/groups/{jid}`, `GET /sessions/{id}/contacts/{jid}/avatar`

## Webhooks

Events are POSTed to the session's `webhook_url` in the flat 360dialog format
(`contacts` / `messages` / `statuses`) plus a `session` object for connection
changes, signed with `X-Whatsmeow-Signature: sha256=<hmac(webhook_secret, body)>`.

Interactive messages are sent as native-flow buttons/lists; when that fails or
`interactive_mode` is `text`, a numbered text fallback is sent and the response
includes `"fallback": true`.
