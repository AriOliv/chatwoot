require 'rails_helper'

describe SlackChannel::SendOnSlackService do
  let!(:channel) { create(:channel_slack) }
  let(:agent) { create(:user, account: channel.account, name: 'Agent Smith') }
  let(:conversation) do
    create(:conversation, inbox: channel.inbox, account: channel.account,
                          additional_attributes: { 'slack_channel_id' => 'CSUPPORT', 'slack_thread_ts' => '2.1' })
  end
  let(:slack_client) { instance_double(Slack::Web::Client) }

  before { allow(Slack::Web::Client).to receive(:new).and_return(slack_client) }

  it 'posts the reply in the thread as the agent and stores the ts' do
    message = create(:message, conversation: conversation, message_type: :outgoing, sender: agent, content: '**hi**')
    allow(slack_client).to receive(:chat_postMessage).and_return({ 'ts' => '2.5' })

    described_class.new(message: message).perform

    expect(slack_client).to have_received(:chat_postMessage)
      .with(hash_including(channel: 'CSUPPORT', thread_ts: '2.1', text: '*hi*', username: agent.available_name))
    expect(message.reload.source_id).to eq('2.5')
    expect(message.status).to eq('delivered')
  end

  it 'sends input_select as Block Kit buttons' do
    message = create(:message, conversation: conversation, message_type: :outgoing, content: 'Pick', content_type: 'input_select',
                               content_attributes: { items: [{ title: 'Yes', value: 'yes' }] })
    allow(slack_client).to receive(:chat_postMessage).and_return({ 'ts' => '2.6' })

    described_class.new(message: message).perform

    expect(slack_client).to have_received(:chat_postMessage) do |args|
      expect(JSON.parse(args[:blocks]).last['elements'].first['value']).to eq('yes')
    end
  end

  it 'marks the message failed and prompts reauthorization on auth errors' do
    message = create(:message, conversation: conversation, message_type: :outgoing, content: 'hi')
    error_response = Faraday::Response.new(status: 200, body: Slack::Messages::Message.new('ok' => false, 'error' => 'invalid_auth'))
    allow(slack_client).to receive(:chat_postMessage).and_raise(Slack::Web::Api::Errors::SlackError.new('invalid_auth', error_response))

    described_class.new(message: message).perform

    expect(message.reload.status).to eq('failed')
    expect(channel.reload.authorization_error_count).to eq(1)
  end
end
