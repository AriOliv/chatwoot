require 'rails_helper'

describe Whatsapp::IncomingMessageWhatsmeowService do
  let!(:whatsapp_channel) { create(:channel_whatsapp, :whatsmeow, sync_templates: false, validate_provider_config: false) }
  let(:inbox) { whatsapp_channel.inbox }
  let(:contact_params) { [{ wa_id: '5511988887777', profile: { name: 'Joe' } }] }

  def process(payload)
    described_class.new(inbox: inbox, params: payload.with_indifferent_access).perform
  end

  it 'creates an incoming text message from a phone contact' do
    process(contacts: contact_params,
            messages: [{ id: 'M1', from: '5511988887777', timestamp: '1700000000', type: 'text', text: { body: 'hi' } }])

    message = inbox.messages.last
    expect(message.content).to eq('hi')
    expect(message.message_type).to eq('incoming')
    expect(message.sender.name).to eq('Joe')
    expect(message.conversation.contact_inbox.source_id).to eq('5511988887777')
  end

  it 'stores messages sent from the phone as outgoing echoes' do
    process(contacts: [{ wa_id: '5511988887777', profile: {} }],
            messages: [{ id: 'M2', from: '5511988887777', to: '5511988887777', from_me: true, timestamp: '1700000000',
                         type: 'text', text: { body: 'sent from phone' } }])

    message = inbox.messages.last
    expect(message.message_type).to eq('outgoing')
    expect(message.content_attributes['external_echo']).to be(true)
  end

  it 'uses the group as the contact and records the participant' do
    process(contacts: [{ wa_id: '120363000000000000@g.us', profile: { name: 'Team' } }],
            messages: [{ id: 'M3', from: '120363000000000000@g.us', timestamp: '1700000000', type: 'text', text: { body: 'hello group' },
                         group: { id: '120363000000000000@g.us', subject: 'Team', participant: '5511988887777', participant_name: 'Joe' } }])

    message = inbox.messages.last
    expect(message.sender.name).to eq('Team')
    expect(message.conversation.contact_inbox.source_id).to eq('120363000000000000@g.us')
    expect(message.content_attributes['external_sender']).to eq('name' => 'Joe', 'phone' => '5511988887777')
  end

  it 'skips group messages when groups are disabled' do
    whatsapp_channel.update!(provider_config: whatsapp_channel.provider_config.merge('groups_enabled' => false))
    expect do
      process(contacts: [{ wa_id: '120363000000000000@g.us', profile: { name: 'Team' } }],
              messages: [{ id: 'M4', from: '120363000000000000@g.us', timestamp: '1700000000', type: 'text', text: { body: 'x' },
                           group: { id: '120363000000000000@g.us', participant: '5511988887777' } }])
    end.not_to change(Message, :count)
  end

  it 'keeps the original timestamp for history messages' do
    process(contacts: contact_params,
            messages: [{ id: 'M5', from: '5511988887777', timestamp: '1600000000', history: true, type: 'text', text: { body: 'old' } }])

    expect(inbox.messages.last.created_at.to_i).to eq(1_600_000_000)
  end

  it 'records reactions as replies to the target message' do
    process(contacts: contact_params,
            messages: [{ id: 'TARGET', from: '5511988887777', timestamp: '1700000000', type: 'text', text: { body: 'hi' } }])
    target = inbox.messages.last

    process(contacts: contact_params,
            messages: [{ id: 'R1', from: '5511988887777', timestamp: '1700000001', type: 'reaction',
                         reaction: { message_id: 'TARGET', emoji: '👍' } }])

    reaction = inbox.messages.last
    expect(reaction.content).to eq('👍')
    expect(reaction.content_attributes['in_reply_to']).to eq(target.id)
    expect(reaction.content_attributes['is_reaction']).to be(true)
  end

  it 'maps a numeric answer to a text-fallback option' do
    process(contacts: contact_params,
            messages: [{ id: 'M6', from: '5511988887777', timestamp: '1700000000', type: 'text', text: { body: 'hi' } }])
    conversation = inbox.conversations.last
    create(:message, conversation: conversation, inbox: inbox, account: inbox.account, message_type: :outgoing, source_id: 'OUT',
                     content_attributes: { 'whatsmeow_fallback_options' => [{ 'id' => 'yes', 'title' => 'Yes' },
                                                                            { 'id' => 'no', 'title' => 'No' }] })

    process(contacts: contact_params,
            messages: [{ id: 'M7', from: '5511988887777', timestamp: '1700000002', type: 'text', text: { body: '2' } }])

    expect(inbox.messages.last.content_attributes['submitted_values']).to eq([{ 'title' => 'No', 'value' => 'no' }])
  end

  it 'updates delivery status' do
    process(contacts: contact_params,
            messages: [{ id: 'M8', from: '5511988887777', timestamp: '1700000000', type: 'text', text: { body: 'hi' } }])
    outgoing = create(:message, conversation: inbox.conversations.last, inbox: inbox, account: inbox.account,
                                message_type: :outgoing, source_id: 'OUT_ID', status: :sent)

    process(statuses: [{ id: 'OUT_ID', status: 'read' }])
    expect(outgoing.reload.status).to eq('read')
  end

  it 'stores session state changes on the channel' do
    process(session: { status: 'connected', jid: '5511900000000', name: 'Support' })

    expect(whatsapp_channel.reload.provider_config).to include('connection_status' => 'connected', 'paired_jid' => '5511900000000')
  end
end
