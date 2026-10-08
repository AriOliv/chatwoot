# Turns Slack Events API payloads into Chatwoot messages for a Slack inbox.
# - Each Slack user is a contact (source id "<team_id>:<user_id>"); when the inbox includes app messages,
#   each app persona (bot_id + username) is a contact too (source id "<team_id>:<bot_id>:<username>").
# - A DM (im/mpim) is one conversation; in monitored channels each thread is one conversation.
# - The thread is tracked in conversation.additional_attributes (identifier is used by the Slack integration).
# - history: true marks messages imported by SlackChannel::HistoryImportService; they keep the Slack timestamp.
class SlackChannel::IncomingMessageService
  SYSTEM_SUBTYPES = %w[channel_join channel_leave channel_topic channel_purpose channel_name thread_broadcast].freeze

  pattr_initialize [:channel!, :event!, { history: false }]

  def perform
    case event[:type]
    when 'message', 'app_mention' then handle_message
    when 'reaction_added' then handle_reaction
    end
  end

  private

  delegate :inbox, :account, to: :channel

  def handle_message
    return update_message if event[:subtype] == 'message_changed'
    return delete_message if event[:subtype] == 'message_deleted'
    return if ignorable_message?
    return if Message.exists?(inbox_id: inbox.id, source_id: event[:ts])

    conversation = find_or_create_conversation
    return if conversation.blank?

    create_message(conversation)
  end

  def ignorable_message?
    return true if unsupported_sender?
    return true if event[:type] == 'app_mention' && dm? # DMs also deliver a message event
    return !channel.accept_dms? if dm?

    channel.monitored_channel_ids.exclude?(event[:channel])
  end

  def unsupported_sender?
    return true if SYSTEM_SUBTYPES.include?(event[:subtype]) || own_message?

    app_message? ? !channel.include_app_messages? : event[:user].blank?
  end

  def own_message?
    event[:user] == channel.bot_user_id || (channel.app_id.present? && event[:app_id] == channel.app_id)
  end

  # Posts made through another app's token: bot_message subtype, or an app's bot user (bot_id set).
  def app_message?
    event[:subtype] == 'bot_message' || event[:bot_id].present?
  end

  def dm?
    %w[im mpim].include?(event[:channel_type]) || event[:channel].to_s.start_with?('D')
  end

  def thread_ts
    event[:thread_ts].presence || event[:ts]
  end

  def find_or_create_conversation
    conversations = inbox.conversations.where("additional_attributes ->> 'slack_channel_id' = ?", event[:channel])
    if dm?
      conversations.where.not(status: :resolved).last || create_conversation
    else
      existing = conversations.where("additional_attributes ->> 'slack_thread_ts' = ?", thread_ts).last
      existing || (thread_root? && opens_conversation? ? create_conversation : nil)
    end
  end

  def thread_root?
    event[:thread_ts].blank? || event[:thread_ts] == event[:ts]
  end

  def opens_conversation?
    !channel.mention_only? || event[:text].to_s.include?("<@#{channel.bot_user_id}>")
  end

  def create_conversation
    attributes = { slack_channel_id: event[:channel], slack_channel_type: dm? ? 'dm' : 'channel' }
    attributes[:slack_thread_ts] = thread_ts unless dm?
    conversation = ::Conversation.new(account_id: account.id, inbox_id: inbox.id, contact_id: contact.id,
                                      contact_inbox_id: contact_inbox.id, additional_attributes: attributes)
    conversation.created_at = slack_time if history
    conversation.save!
    conversation
  end

  def slack_time
    Time.zone.at(event[:ts].to_f)
  end

  def create_message(conversation)
    message = conversation.messages.build(
      account_id: account.id, inbox_id: inbox.id, message_type: :incoming, sender: contact,
      content: format_text(event[:text]), source_id: event[:ts], content_attributes: message_attributes
    )
    message.created_at = slack_time if history
    attach_files(message)
    message.save!
  end

  def message_attributes
    { slack_ts: event[:ts], slack_thread_ts: event[:thread_ts], external_created_at: (event[:ts].to_i if history) }.compact
  end

  def update_message
    edited = event[:message] || {}
    message = inbox.messages.find_by(source_id: edited[:ts])
    message&.update!(content: format_text(edited[:text]))
  end

  def delete_message
    message = inbox.messages.find_by(source_id: event[:deleted_ts])
    message&.update!(content: I18n.t('conversations.messages.deleted'), content_attributes: message.content_attributes.merge(deleted: true))
  end

  def handle_reaction
    return if event[:user] == channel.bot_user_id

    target = inbox.messages.find_by(source_id: event.dig(:item, :ts))
    return if target.blank?

    target.conversation.messages.create!(
      account_id: account.id, inbox_id: inbox.id, message_type: :incoming, sender: contact,
      content: Integrations::Slack::EmojiFormatter.format(":#{event[:reaction]}:"),
      source_id: "reaction:#{event[:event_ts]}",
      content_attributes: { in_reply_to: target.id, in_reply_to_external_id: target.source_id, is_reaction: true }
    )
  end

  def format_text(text)
    Integrations::Slack::EmojiFormatter.format(::Slack::Messages::Formatting.unescape(text.to_s))
  end

  def attach_files(message)
    Array(event[:files]).each do |file|
      download = Down::NetHttp.download(file[:url_private], headers: { 'Authorization' => "Bearer #{channel.bot_token}" })
      message.attachments.new(
        account_id: account.id, file_type: file_type(file), external_url: file[:url_private],
        file: { io: download, filename: file[:name].presence || download.original_filename, content_type: file[:mimetype] }
      )
    end
  end

  def file_type(file)
    case file[:mimetype].to_s
    when %r{\Aimage/} then :image
    when %r{\Avideo/} then :video
    when %r{\Aaudio/} then :audio
    else :file
    end
  end

  def contact_inbox
    @contact_inbox ||= ::ContactInboxWithContactBuilder.new(
      source_id: contact_source_id, inbox: inbox, contact_attributes: app_persona? ? app_contact_attributes : contact_attributes
    ).perform
  end

  # bot_message posts have no user; apps posting with a custom username are one contact per persona.
  def app_persona?
    event[:user].blank? && event[:bot_id].present?
  end

  def contact_source_id
    return "#{channel.team_id}:#{event[:user]}" unless app_persona?

    persona = event[:username].presence || event.dig(:bot_profile, :name)
    ["#{channel.team_id}:#{event[:bot_id]}", persona.to_s.parameterize.presence].compact.join(':')
  end

  def app_contact_attributes
    name = event[:username].presence || event.dig(:bot_profile, :name).presence || event[:bot_id]
    avatar = event.dig(:icons, :image_72).presence || event.dig(:icons, :image_48).presence || event.dig(:bot_profile, :icons, :image_72)
    { name: name, avatar_url: avatar, additional_attributes: { slack_bot_id: event[:bot_id], slack_app: true } }.compact
  end

  def contact
    contact_inbox.contact
  end

  def contact_attributes
    user = channel.client.users_info(user: event[:user])[:user]
    profile = user[:profile] || {}
    {
      name: profile[:display_name].presence || user[:real_name].presence || user[:name],
      email: profile[:email].presence,
      avatar_url: profile[:image_192].presence,
      additional_attributes: slack_user_attributes(user, profile)
    }.compact
  rescue ::Slack::Web::Api::Errors::SlackError => e
    Rails.logger.warn("[SLACK_CHANNEL] users_info failed for #{event[:user]}: #{e.message}")
    { name: event[:user] }
  end

  def slack_user_attributes(user, profile)
    { slack_user_id: user[:id], slack_team_id: user[:team_id], title: profile[:title].presence,
      external_workspace: user[:team_id].present? && user[:team_id] != channel.team_id }.compact
  end
end
