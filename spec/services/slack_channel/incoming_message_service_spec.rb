require 'rails_helper'

describe SlackChannel::IncomingMessageService do
  let!(:channel) { create(:channel_slack) }
  let(:inbox) { channel.inbox }
  let(:slack_client) { instance_double(Slack::Web::Client) }

  before do
    allow(Slack::Web::Client).to receive(:new).and_return(slack_client)
    allow(slack_client).to receive(:conversations_join)
    allow(slack_client).to receive(:users_info).and_return(
      { user: { id: 'U1', team_id: channel.team_id, real_name: 'Joe', profile: { display_name: 'joe', email: 'joe@example.com' } } }
                                                             .with_indifferent_access
    )
  end

  def process(event)
    described_class.new(channel: channel, event: event.with_indifferent_access).perform
  end

  it 'creates a contact and conversation for a DM' do
    process(type: 'message', channel_type: 'im', channel: 'D1', user: 'U1', text: 'hello', ts: '1.1')

    message = inbox.messages.last
    expect(message.content).to eq('hello')
    expect(message.message_type).to eq('incoming')
    expect(message.sender.email).to eq('joe@example.com')
    expect(message.conversation.additional_attributes).to include('slack_channel_id' => 'D1', 'slack_channel_type' => 'dm')
  end

  it 'reuses the open DM conversation' do
    process(type: 'message', channel_type: 'im', channel: 'D1', user: 'U1', text: 'one', ts: '1.1')
    process(type: 'message', channel_type: 'im', channel: 'D1', user: 'U1', text: 'two', ts: '1.2')

    expect(inbox.conversations.count).to eq(1)
  end

  it 'creates one conversation per thread in monitored channels' do
    process(type: 'message', channel_type: 'channel', channel: 'CSUPPORT', user: 'U1', text: 'help', ts: '2.1')
    process(type: 'message', channel_type: 'channel', channel: 'CSUPPORT', user: 'U1', text: 'more', ts: '2.2', thread_ts: '2.1')
    process(type: 'message', channel_type: 'channel', channel: 'CSUPPORT', user: 'U1', text: 'new topic', ts: '3.1')

    expect(inbox.conversations.count).to eq(2)
    expect(inbox.conversations.first.messages.count).to eq(2)
    expect(inbox.conversations.first.additional_attributes['slack_thread_ts']).to eq('2.1')
  end

  it 'ignores channels that are not monitored, bot messages and duplicates' do
    process(type: 'message', channel_type: 'channel', channel: 'COTHER', user: 'U1', text: 'x', ts: '4.1')
    process(type: 'message', channel_type: 'im', channel: 'D1', user: 'UBOT', text: 'echo', ts: '4.2')
    process(type: 'message', channel_type: 'im', channel: 'D1', user: 'U1', text: 'once', ts: '4.3')
    process(type: 'message', channel_type: 'im', channel: 'D1', user: 'U1', text: 'once', ts: '4.3')

    expect(inbox.messages.count).to eq(1)
  end

  it 'only opens conversations on mention when mention_only is enabled' do
    channel.update!(settings: channel.settings.merge('mention_only' => true))
    process(type: 'message', channel_type: 'channel', channel: 'CSUPPORT', user: 'U1', text: 'chatter', ts: '5.1')
    process(type: 'message', channel_type: 'channel', channel: 'CSUPPORT', user: 'U1', text: '<@UBOT> help', ts: '5.2')

    expect(inbox.conversations.count).to eq(1)
  end

  it 'updates edited messages and records reactions' do
    process(type: 'message', channel_type: 'im', channel: 'D1', user: 'U1', text: 'typo', ts: '6.1')
    process(type: 'message', subtype: 'message_changed', channel: 'D1', message: { ts: '6.1', text: 'fixed' })
    process(type: 'reaction_added', user: 'U1', reaction: 'thumbsup', event_ts: '6.2', item: { channel: 'D1', ts: '6.1' })

    original = inbox.messages.find_by(source_id: '6.1')
    expect(original.content).to eq('fixed')
    expect(inbox.messages.last.content_attributes['in_reply_to']).to eq(original.id)
  end

  it 'ignores messages from other apps unless the inbox includes them' do
    event = { type: 'message', subtype: 'bot_message', channel_type: 'channel', channel: 'CSUPPORT', bot_id: 'BOTHER',
              username: 'Pedro · Globex', text: 'saque travado', ts: '7.1' }
    process(event)
    expect(inbox.messages.count).to eq(0)

    channel.update!(settings: channel.settings.merge('include_app_messages' => true))
    process(event)
    process(event.merge(username: 'Julia · Globex', text: 'KYB', ts: '7.2'))

    expect(inbox.messages.pluck(:content)).to eq(['saque travado', 'KYB'])
    expect(inbox.contacts.pluck(:name)).to contain_exactly('Pedro · Globex', 'Julia · Globex')
  end

  it 'never captures messages posted by its own app' do
    channel.update!(app_id: 'AOWN', settings: channel.settings.merge('include_app_messages' => true))
    process(type: 'message', channel_type: 'channel', channel: 'CSUPPORT', bot_id: 'BOWN', app_id: 'AOWN', user: 'UOTHER', text: 'reply', ts: '8.1')

    expect(inbox.messages.count).to eq(0)
  end

  it 'keeps the Slack timestamp for imported history' do
    described_class.new(channel: channel, history: true,
                        event: { type: 'message', channel_type: 'channel', channel: 'CSUPPORT', user: 'U1', text: 'old', ts: '1700000000.1' }
                          .with_indifferent_access).perform

    message = inbox.messages.last
    expect(message.created_at.to_i).to eq(1_700_000_000)
    expect(message.conversation.created_at.to_i).to eq(1_700_000_000)
  end
end
