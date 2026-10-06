# HTTP client for the whatsmeow-bridge service (see whatsmeow-bridge/ in this repo).
# Sessions are keyed by the channel phone number.
class Whatsapp::WhatsmeowSessionService
  class BridgeError < StandardError; end

  def initialize(channel)
    @channel = channel
  end

  def register
    request(:post, '/sessions', {
              session_id: session_id,
              webhook_url: webhook_url,
              webhook_secret: @channel.provider_config['webhook_secret'],
              import_history: @channel.provider_config['import_history'] != false
            })
  end

  def status
    request(:get, "#{session_path}/status")
  end

  def qr
    request(:get, "#{session_path}/qr")
  end

  def pair_phone(phone)
    request(:post, "#{session_path}/pair-phone", { phone: phone })
  end

  def reconnect
    request(:post, "#{session_path}/reconnect")
  end

  def logout
    request(:delete, "#{session_path}/")
  rescue StandardError => e
    # before_destroy must never block a channel delete
    Rails.logger.error "[WHATSMEOW] Logout failed for channel #{@channel.id}: #{e.message}"
  end

  def send_message(payload)
    request(:post, "#{session_path}/messages", payload)
  end

  def react(payload)
    request(:post, "#{session_path}/reactions", payload)
  end

  def healthy?
    HTTParty.get("#{base_url}/healthz", timeout: 5).success?
  rescue StandardError
    false
  end

  def media_url(message_id)
    "#{base_url}#{session_path}/media/#{ERB::Util.url_encode(message_id)}"
  end

  def headers
    { 'Authorization' => "Bearer #{GlobalConfigService.load('WHATSMEOW_BRIDGE_TOKEN', nil)}", 'Content-Type' => 'application/json' }
  end

  private

  def request(method, path, body = nil)
    options = { headers: headers, timeout: 60 }
    options[:body] = body.to_json if body
    response = HTTParty.send(method, "#{base_url}#{path}", options)
    raise BridgeError, (response.parsed_response.try(:[], 'error') || "bridge returned #{response.code}") unless response.success?

    response.parsed_response.presence || {}
  end

  def session_id
    @channel.phone_number.delete_prefix('+')
  end

  def session_path
    "/sessions/#{ERB::Util.url_encode(session_id)}"
  end

  def base_url
    GlobalConfigService.load('WHATSMEOW_BRIDGE_URL', nil).to_s.chomp('/')
  end

  def webhook_url
    base = GlobalConfigService.load('WHATSMEOW_WEBHOOK_BASE_URL', nil).presence || ENV.fetch('FRONTEND_URL', '')
    "#{base.chomp('/')}/webhooks/whatsapp/#{@channel.phone_number}"
  end
end
