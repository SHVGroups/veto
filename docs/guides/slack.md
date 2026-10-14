# Slack

Veto can post each held action to a Slack channel with a button for each
option, so reviewers can decide without opening the console.

## Requirements

- Veto must be reachable from the internet over HTTPS at `VETO_PUBLIC_URL`,
  so Slack can send button clicks to it.
- Each reviewer's Slack account email must match their Veto account email,
  and their Veto role must allow reviewing. Clicks from anyone else are
  refused with a private message.

## Set up

1. In the console, open **Slack**. Copy the app manifest shown there; it
   already contains your interaction address,
   `<VETO_PUBLIC_URL>/v1/integrations/slack/interactions/<workspace id>`.
2. At [api.slack.com/apps](https://api.slack.com/apps), choose **Create New
   App**, **From a manifest**, pick your Slack workspace and paste it.
3. Install the app to your Slack workspace.
4. Copy the **Bot User OAuth Token** (`xoxb-...`) from **OAuth &
   Permissions** and the **Signing Secret** from **Basic Information**.
5. In Veto's **Slack** page, enter the channel id (in Slack, the channel's
   details show it at the bottom, such as `C0123456789`), the token and the
   secret, and save. Veto checks the token with Slack before saving.
6. Invite the app to the channel: `/invite @Veto`.

The app needs the `chat:write`, `users:read` and `users:read.email`
scopes, which the manifest requests.

## Behaviour

- Each held action is posted once, with the tool, agent, who it acts for,
  the request, the reasons, the options with their changes, and a link to
  the console. **Notify the channel** adds `@here`.
- A click is verified with the signing secret, matched to a Veto member by
  email, and recorded like a console review with channel `slack`.
- When the action is decided anywhere, by Slack, the console, the model on
  a retry, or expiry, the message is updated to show the result and the
  buttons are removed.

## Troubleshooting

See [Troubleshooting: Slack](../troubleshooting.md#slack).
