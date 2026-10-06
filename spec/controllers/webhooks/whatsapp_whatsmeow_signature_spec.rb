require 'rails_helper'

RSpec.describe 'Whatsmeow webhook signature', type: :request do
  let(:body) { { statuses: [{ id: 'X', status: 'read' }] }.to_json }

  before { create(:channel_whatsapp, :whatsmeow, phone_number: '+5511900000001', sync_templates: false, validate_provider_config: false) }

  def sign(payload)
    "sha256=#{OpenSSL::HMAC.hexdigest('SHA256', 'whatsmeow_secret', payload)}"
  end

  it 'accepts payloads signed with the channel webhook secret' do
    expect do
      post '/webhooks/whatsapp/+5511900000001', params: body,
                                                headers: { 'Content-Type' => 'application/json', 'X-Whatsmeow-Signature' => sign(body) }
    end.to have_enqueued_job(Webhooks::WhatsappEventsJob)
    expect(response).to have_http_status(:ok)
  end

  it 'rejects unsigned payloads' do
    post '/webhooks/whatsapp/+5511900000001', params: body, headers: { 'Content-Type' => 'application/json' }

    expect(response).to have_http_status(:unauthorized)
  end
end
