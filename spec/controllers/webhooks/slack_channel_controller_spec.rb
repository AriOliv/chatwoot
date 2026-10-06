require 'rails_helper'

RSpec.describe Webhooks::SlackChannelController, type: :request do
  let(:secret) { 'slack_secret' }
  let(:body) { { type: 'event_callback', team_id: 'T1', event_id: 'Ev1', event: { type: 'message' } }.to_json }

  before { create(:installation_config, name: 'SLACK_CHANNEL_SIGNING_SECRET', value: secret) }

  def signed_headers(payload, timestamp: Time.current.to_i)
    signature = "v0=#{OpenSSL::HMAC.hexdigest('SHA256', secret, "v0:#{timestamp}:#{payload}")}"
    { 'Content-Type' => 'application/json', 'X-Slack-Request-Timestamp' => timestamp.to_s, 'X-Slack-Signature' => signature }
  end

  it 'answers the url verification challenge' do
    payload = { type: 'url_verification', challenge: 'abc' }.to_json
    post '/webhooks/slack_channel/events', params: payload, headers: signed_headers(payload)

    expect(response.parsed_body['challenge']).to eq('abc')
  end

  it 'enqueues signed events' do
    expect do
      post '/webhooks/slack_channel/events', params: body, headers: signed_headers(body)
    end.to have_enqueued_job(Webhooks::SlackChannelEventsJob)
    expect(response).to have_http_status(:ok)
  end

  it 'rejects invalid signatures' do
    post '/webhooks/slack_channel/events', params: body,
                                           headers: signed_headers(body).merge('X-Slack-Signature' => 'v0=bad')

    expect(response).to have_http_status(:unauthorized)
  end
end
