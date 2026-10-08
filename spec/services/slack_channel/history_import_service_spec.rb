require 'rails_helper'

describe SlackChannel::HistoryImportService do
  let!(:channel) { create(:channel_slack) }
  let(:inbox) { channel.inbox }
  let(:slack_client) { instance_double(Slack::Web::Client) }

  before do
    allow(Slack::Web::Client).to receive(:new).and_return(slack_client)
    allow(slack_client).to receive(:users_info).and_return({ user: { id: 'U1', team_id: channel.team_id, real_name: 'Joe', profile: {} } }
                                                             .with_indifferent_access)
    allow(slack_client).to receive(:conversations_history)
      .and_yield({ messages: [{ type: 'message', user: 'U1', text: 'second topic', ts: '1700000100.1' },
                              { type: 'message', user: 'U1', text: 'help', ts: '1700000000.1', reply_count: 1 }] }.with_indifferent_access)
    allow(slack_client).to receive(:conversations_replies)
      .and_yield({ messages: [{ type: 'message', user: 'U1', text: 'help', ts: '1700000000.1', thread_ts: '1700000000.1' },
                              { type: 'message', user: 'U1', text: 'details', ts: '1700000050.1', thread_ts: '1700000000.1' }] }
                   .with_indifferent_access)
  end

  it 'imports thread roots and replies as conversations with their original timestamps' do
    described_class.new(channel: channel, slack_channel_id: 'CSUPPORT').perform

    thread = inbox.conversations.find_by("additional_attributes ->> 'slack_thread_ts' = ?", '1700000000.1')
    expect(inbox.conversations.count).to eq(2)
    expect(thread.messages.order(:created_at).pluck(:content)).to eq(%w[help details])
    expect(thread.messages.first.created_at.to_i).to eq(1_700_000_000)
  end
end
