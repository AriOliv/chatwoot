require 'rails_helper'

describe Whatsapp::Providers::WhatsmeowService do
  subject(:service) { described_class.new(whatsapp_channel: whatsapp_channel) }

  let!(:whatsapp_channel) do
    create(:channel_whatsapp, :whatsmeow, phone_number: '+5511900000000', sync_templates: false, validate_provider_config: false)
  end
  let(:bridge_url) { 'http://bridge.test' }
  let(:messages_url) { "#{bridge_url}/sessions/5511900000000/messages" }
  let(:json_headers) { { 'Content-Type' => 'application/json' } }

  before do
    create(:installation_config, name: 'WHATSMEOW_BRIDGE_URL', value: bridge_url)
    create(:installation_config, name: 'WHATSMEOW_BRIDGE_TOKEN', value: 'bridge_token')
  end

  describe '#send_message' do
    it 'sends a text message with the bridge token and returns the message id' do
      message = create(:message, message_type: :outgoing, content: 'hello', inbox: whatsapp_channel.inbox)
      request = stub_request(:post, messages_url)
                .with(headers: { 'Authorization' => 'Bearer bridge_token' },
                      body: { type: 'text', text: 'hello', to: '5511988887777' }.to_json)
                .to_return(status: 200, body: { id: 'WA_ID' }.to_json, headers: json_headers)

      expect(service.send_message('5511988887777', message)).to eq('WA_ID')
      expect(request).to have_been_requested
    end

    it 'quotes the original message when replying' do
      original = create(:message, message_type: :incoming, content: 'question', inbox: whatsapp_channel.inbox, source_id: 'ORIGINAL_ID')
      message = create(:message, message_type: :outgoing, content: 'reply', inbox: whatsapp_channel.inbox,
                                 conversation: original.conversation, content_attributes: { in_reply_to: original.id })
      stub_request(:post, messages_url)
        .with(body: hash_including('reply_to_id' => 'ORIGINAL_ID'))
        .to_return(status: 200, body: { id: 'WA_ID' }.to_json, headers: json_headers)

      expect(service.send_message('5511988887777', message)).to eq('WA_ID')
    end

    it 'sends input_select items as interactive buttons' do
      message = create(:message, message_type: :outgoing, content: 'Pick one', inbox: whatsapp_channel.inbox,
                                 content_type: 'input_select',
                                 content_attributes: { items: [{ title: 'Yes', value: 'yes' }, { title: 'No', value: 'no' }] })
      stub_request(:post, messages_url)
        .with(body: hash_including('type' => 'interactive', 'interactive_mode' => 'native',
                                   'interactive' => { 'buttons' => [{ 'id' => 'yes', 'title' => 'Yes' }, { 'id' => 'no', 'title' => 'No' }] }))
        .to_return(status: 200, body: { id: 'WA_ID' }.to_json, headers: json_headers)

      expect(service.send_message('5511988887777', message)).to eq('WA_ID')
    end

    it 'records fallback options when the bridge sends numbered text' do
      message = create(:message, message_type: :outgoing, content: 'Pick one', inbox: whatsapp_channel.inbox,
                                 content_type: 'input_select',
                                 content_attributes: { items: [{ title: 'Yes', value: 'yes' }] })
      stub_request(:post, messages_url)
        .to_return(status: 200, body: { id: 'WA_ID', fallback: true }.to_json, headers: json_headers)

      service.send_message('5511988887777', message)
      expect(message.reload.content_attributes['whatsmeow_fallback_options']).to eq([{ 'id' => 'yes', 'title' => 'Yes' }])
    end

    it 'marks the message as failed when the bridge rejects it' do
      message = create(:message, message_type: :outgoing, content: 'hello', inbox: whatsapp_channel.inbox)
      stub_request(:post, messages_url).to_return(status: 502, body: { error: 'not connected' }.to_json, headers: json_headers)

      expect(service.send_message('5511988887777', message)).to be_nil
      expect(message.reload.status).to eq('failed')
      expect(message.external_error).to eq('not connected')
    end
  end

  describe '#sync_templates' do
    it 'only marks templates as updated' do
      expect { service.sync_templates }.to(change { whatsapp_channel.reload.message_templates_last_updated })
    end
  end

  describe '#media_url' do
    it 'points to the bridge media endpoint' do
      expect(service.media_url('MSG/1')).to eq("#{bridge_url}/sessions/5511900000000/media/MSG%2F1")
    end
  end
end
