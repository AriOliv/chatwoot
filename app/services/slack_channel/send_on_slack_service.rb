# Posts agent replies from a Slack inbox back into the DM or thread, as the app bot
# with the agent's name and avatar.
class SlackChannel::SendOnSlackService < Base::SendOnChannelService
  AUTH_ERRORS = %w[invalid_auth account_inactive token_revoked token_expired missing_scope not_authed].freeze

  private

  def channel_class
    Channel::Slack
  end

  def perform_reply
    response = message.attachments.present? ? upload_files : post_message
    message.update!(source_id: response_ts(response))
    Messages::StatusUpdateService.new(message, 'delivered').perform
  rescue ::Slack::Web::Api::Errors::SlackError => e
    channel.authorization_error! if AUTH_ERRORS.include?(e.message)
    Messages::StatusUpdateService.new(message, 'failed', e.message).perform
  end

  def post_message
    channel.client.chat_postMessage(
      channel: slack_channel_id, thread_ts: thread_ts, text: message.outgoing_content.presence || ' ',
      blocks: blocks&.to_json, username: sender_name, icon_url: sender_avatar, unfurl_links: false
    ).compact
  end

  def upload_files
    files = message.attachments.map do |attachment|
      content = attachment.file.blob.open(&:read)
      { filename: attachment.file.filename.to_s, content: content, title: attachment.file.filename.to_s }
    end
    channel.client.files_upload_v2(files: files, channel_id: slack_channel_id, thread_ts: thread_ts,
                                   initial_comment: message.outgoing_content.presence)
  end

  # files_upload_v2 doesn't return the posted message ts; fall back to the file id.
  def response_ts(response)
    response['ts'] || response.dig('files', 0, 'id') || response.dig('file', 'id')
  end

  # input_select is sent as Block Kit buttons; clicks come back through the interactivity endpoint.
  def blocks
    return unless message.content_type == 'input_select'

    buttons = message.content_attributes['items'].first(25).map do |item|
      { type: 'button', text: { type: 'plain_text', text: item['title'].to_s.first(75) }, value: item['value'].to_s,
        action_id: "cw_select_#{message.id}_#{item['value']}" }
    end
    [{ type: 'section', text: { type: 'mrkdwn', text: message.outgoing_content.presence || ' ' } },
     { type: 'actions', block_id: "cw_message_#{message.id}", elements: buttons }]
  end

  def slack_channel_id
    conversation.additional_attributes['slack_channel_id']
  end

  def thread_ts
    conversation.additional_attributes['slack_thread_ts']
  end

  def conversation
    message.conversation
  end

  def sender_name
    message.sender.try(:available_name).presence || message.sender.try(:name)
  end

  def sender_avatar
    message.sender.try(:avatar_url).presence
  end
end
