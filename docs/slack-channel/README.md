# Slack inbox and WhatsApp (QR code) inbox

This fork adds two inbox types on top of upstream Chatwoot.

## WhatsApp (QR Code) — `whatsmeow` provider

A fourth WhatsApp provider backed by `whatsmeow-bridge/` (Go, built on
[AriOliv/whatsmeow](https://github.com/AriOliv/whatsmeow)). Numbers are paired with a QR
code or pairing code from **Settings > Inboxes > Add Inbox > WhatsApp > WhatsApp (QR Code)**.

- Super Admin > Settings > **WhatsApp (QR Code)**: `WHATSMEOW_BRIDGE_URL`, `WHATSMEOW_BRIDGE_TOKEN`,
  and optionally `WHATSMEOW_WEBHOOK_BASE_URL` (how the bridge reaches Chatwoot, e.g. `http://rails:3000`).
- Supports text, media, voice notes, replies, reactions, groups (the group is the contact,
  the participant is shown on each message), history import and buttons/lists (native first,
  numbered text fallback; numeric answers map back to the option).
- No 24h messaging window.
- Using an unofficial WhatsApp Web client can get a number banned. Use numbers you can afford to lose.

## Slack inbox — `Channel::Slack`

A Slack workspace as an inbox. DMs to the app and threads in monitored channels (including
Slack Connect channels) become conversations; agents reply from Chatwoot and the reply is
posted in Slack as the app, with the agent's name and avatar.

1. Create a Slack app from `slack-app-manifest.yml` (replace the URLs with your `FRONTEND_URL`; Slack requires HTTPS).
2. Super Admin > Settings > **Slack Inbox**: set `SLACK_CHANNEL_CLIENT_ID`, `SLACK_CHANNEL_CLIENT_SECRET`, `SLACK_CHANNEL_SIGNING_SECRET`.
3. Settings > Inboxes > Add Inbox > **Slack** > Continue with Slack.
4. In the inbox Configuration tab, pick the channels to monitor. Private channels need `/invite @Chatwoot Inbox`.

This is independent of the existing Slack *integration* (which mirrors conversations into a Slack
channel for agents); both can be used at the same time.
