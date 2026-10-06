module SlackChannel::IntegrationHelper
  BOT_SCOPES = %w[
    app_mentions:read channels:history channels:join channels:read chat:write chat:write.customize
    files:read files:write groups:history groups:read im:history im:read im:write mpim:history mpim:read
    reactions:read reactions:write team:read users:read users:read.email
  ].freeze

  def slack_channel_authorize_url(state)
    query = { client_id: client_id, scope: BOT_SCOPES.join(','), redirect_uri: slack_channel_redirect_uri, state: state }
    "https://slack.com/oauth/v2/authorize?#{query.to_query}"
  end

  def slack_channel_redirect_uri
    "#{ENV.fetch('FRONTEND_URL', '')}/slack_channel/callback"
  end

  def generate_slack_channel_state(account_id)
    JWT.encode({ sub: account_id, iat: Time.current.to_i, exp: 15.minutes.from_now.to_i }, client_secret, 'HS256')
  end

  def verify_slack_channel_state(token)
    JWT.decode(token, client_secret, true, algorithm: 'HS256').first['sub']
  rescue JWT::DecodeError
    nil
  end

  def client_id
    SlackChannel::IntegrationHelper.config('SLACK_CHANNEL_CLIENT_ID')
  end

  def client_secret
    SlackChannel::IntegrationHelper.config('SLACK_CHANNEL_CLIENT_SECRET')
  end

  # Blank seeded config rows make GlobalConfigService skip its ENV fallback.
  def self.config(key)
    GlobalConfigService.load(key, nil).presence || ENV.fetch(key, nil).presence
  end
end
