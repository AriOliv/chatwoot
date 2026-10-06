require 'rails_helper'

RSpec.describe 'Whatsmeow inbox API', type: :request do
  let(:account) { create(:account) }
  let(:admin) { create(:user, account: account, role: :administrator) }
  let(:agent) { create(:user, account: account, role: :agent) }
  let!(:channel) do
    create(:channel_whatsapp, :whatsmeow, account: account, phone_number: '+5511900000002', sync_templates: false, validate_provider_config: false)
  end
  let(:inbox) { channel.inbox }
  let(:base_path) { "/api/v1/accounts/#{account.id}/inboxes/#{inbox.id}/whatsmeow" }
  let(:session_url) { 'http://bridge.test/sessions/5511900000002' }

  before do
    create(:installation_config, name: 'WHATSMEOW_BRIDGE_URL', value: 'http://bridge.test')
    create(:installation_config, name: 'WHATSMEOW_BRIDGE_TOKEN', value: 'token')
  end

  it 'returns the bridge connection status' do
    stub_request(:get, "#{session_url}/status").to_return(status: 200, body: { status: 'qr_pending', qr: 'CODE' }.to_json,
                                                          headers: { 'Content-Type' => 'application/json' })

    get "#{base_path}/status", headers: admin.create_new_auth_token, as: :json

    expect(response).to have_http_status(:ok)
    expect(response.parsed_body).to eq('status' => 'qr_pending', 'qr' => 'CODE')
  end

  it 'returns a QR code' do
    stub_request(:get, "#{session_url}/qr").to_return(status: 200, body: { qr: 'CODE' }.to_json, headers: { 'Content-Type' => 'application/json' })

    get "#{base_path}/qr", headers: admin.create_new_auth_token, as: :json

    expect(response.parsed_body).to eq('qr' => 'CODE')
  end

  it 'surfaces bridge errors as 422' do
    stub_request(:get, "#{session_url}/qr").to_return(status: 409, body: { error: 'already paired' }.to_json,
                                                      headers: { 'Content-Type' => 'application/json' })

    get "#{base_path}/qr", headers: admin.create_new_auth_token, as: :json

    expect(response).to have_http_status(:unprocessable_entity)
    expect(response.parsed_body['error']).to eq('already paired')
  end

  it 'is restricted to administrators' do
    get "#{base_path}/status", headers: agent.create_new_auth_token, as: :json

    expect(response).to have_http_status(:unauthorized)
  end
end
