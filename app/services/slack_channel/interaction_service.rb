# Handles Block Kit button clicks on messages sent from a Slack inbox: the selection is
# recorded on the original input_select message and as an incoming message from the clicker.
class SlackChannel::InteractionService
  pattr_initialize [:payload!]

  def perform
    item = selected_item
    return if item.blank?

    selection = { 'title' => item['title'], 'value' => item['value'] }
    original_message.update!(content_attributes: original_message.content_attributes.merge('submitted_values' => [selection]))
    SlackChannel::IncomingMessageService.new(channel: channel, event: synthetic_event(item)).perform
  end

  private

  def action
    @action ||= payload[:actions]&.first || {}
  end

  def original_message
    return @original_message if defined?(@original_message)

    message_id = action[:block_id].to_s.delete_prefix('cw_message_')
    @original_message = (channel.inbox.messages.find_by(id: message_id) if channel && message_id.present?)
  end

  def selected_item
    return if original_message.blank?

    Array(original_message.content_attributes['items']).find { |i| i['value'].to_s == action[:value].to_s }
  end

  def channel
    @channel ||= Channel::Slack.find_by(team_id: payload.dig(:team, :id))
  end

  def synthetic_event(item)
    container = payload[:container] || {}
    {
      type: 'message', user: payload.dig(:user, :id), channel: container[:channel_id], text: item['title'],
      ts: "#{container[:message_ts]}-#{payload[:trigger_id]}", thread_ts: container[:thread_ts] || container[:message_ts],
      channel_type: container[:channel_id].to_s.start_with?('D') ? 'im' : 'channel'
    }.with_indifferent_access
  end
end
