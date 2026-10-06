# Handles webhooks from the whatsmeow-bridge. Payloads use the flat 360dialog shape with extensions:
# - messages[].from_me: sent from the paired phone; stored as an outgoing echo
# - messages[].history: imported from history sync; keeps the original timestamp
# - messages[].group: group chats become one contact (the group) with the participant on each message
# - messages[].reaction: emoji reactions to an earlier message
# - session: connection state changes for the paired device
class Whatsapp::IncomingMessageWhatsmeowService < Whatsapp::IncomingMessageBaseService
  NON_PHONE_SOURCE_ID = /\A\d{1,32}@(g\.us|lid)\z/

  def perform
    return update_session_state if params[:session].present?

    super
  end

  private

  def outgoing_echo
    messages_data&.first&.dig(:from_me).present?
  end

  def process_messages
    return if message_type == 'reaction' && !reaction_target
    return if group_message? && !groups_enabled?

    super
  end

  def unprocessable_message_type?(type)
    %w[ephemeral request_welcome].include?(type)
  end

  def set_contact
    contact_params = @processed_params[:contacts]&.first
    return if contact_params.blank?

    source_id = contact_params[:wa_id].to_s
    return super unless source_id.match?(NON_PHONE_SOURCE_ID)

    @contact_inbox = ::ContactInboxWithContactBuilder.new(
      source_id: source_id, inbox: inbox, contact_attributes: { name: contact_params.dig(:profile, :name).presence || source_id }
    ).perform
    @contact = @contact_inbox.contact
    update_group_name(contact_params) if group_message?
  end

  def update_group_name(contact_params)
    subject = contact_params.dig(:profile, :name)
    @contact.update!(name: subject) if subject.present? && @contact.name != subject
  end

  # whatsmeow addresses each conversation through the contact inbox it arrived on.
  def addressable_identifiers?
    true
  end

  def create_messages
    return create_reaction if message_type == 'reaction'

    super
  end

  # Applies the whatsmeow extensions before the base service saves the message.
  def create_message(message, **)
    super
    payload = messages_data.first
    attrs = @message.content_attributes
    attrs = attrs.merge(external_sender: group_sender(payload)) if payload[:group].present?
    attrs = attrs.merge(submitted_values: [fallback_selection]) if fallback_selection
    if payload[:history]
      attrs = attrs.merge(external_created_at: payload[:timestamp].to_i)
      @message.created_at = Time.zone.at(payload[:timestamp].to_i)
    end
    @message.content_attributes = attrs
  end

  def group_sender(payload)
    group = payload[:group]
    { name: group[:participant_name].presence || "+#{group[:participant]}", phone: group[:participant] }.compact
  end

  # A numeric answer ("2") to a text fallback of buttons resolves to that option.
  def fallback_selection
    return @fallback_selection if defined?(@fallback_selection)

    @fallback_selection = nil
    return if outgoing_echo

    choice = @message.content.to_s.strip
    return unless choice.match?(/\A\d{1,2}\z/)

    options = last_outgoing_message&.content_attributes&.dig('whatsmeow_fallback_options')
    option = options&.at(choice.to_i - 1)
    @fallback_selection = option && { 'title' => option['title'], 'value' => option['id'] }
  end

  def last_outgoing_message
    @conversation.messages.outgoing.reorder(created_at: :desc).first
  end

  def reaction_target
    @reaction_target ||= Message.find_by(source_id: messages_data.first.dig(:reaction, :message_id), inbox_id: inbox.id)
  end

  def create_reaction
    reaction = messages_data.first[:reaction]
    @message = @conversation.messages.create!(
      content: reaction[:emoji].presence || I18n.t('conversations.messages.whatsapp.reaction_removed'),
      account_id: @inbox.account_id,
      inbox_id: @inbox.id,
      message_type: outgoing_echo ? :outgoing : :incoming,
      status: outgoing_echo ? :delivered : :sent,
      sender: outgoing_echo ? nil : @contact,
      source_id: messages_data.first[:id].to_s,
      content_attributes: { in_reply_to: reaction_target.id, in_reply_to_external_id: reaction_target.source_id, is_reaction: true,
                            external_echo: outgoing_echo.presence }.compact
    )
  end

  def group_message?
    messages_data&.first&.dig(:group).present?
  end

  def groups_enabled?
    inbox.channel.provider_config['groups_enabled'] != false
  end

  def update_session_state
    session = params[:session]
    channel = inbox.channel
    channel.provider_config = channel.provider_config.merge(
      'connection_status' => session[:status],
      'paired_jid' => session[:jid].presence || channel.provider_config['paired_jid'],
      'paired_name' => session[:name].presence || channel.provider_config['paired_name']
    ).compact
    channel.save!(validate: false)
    channel.reauthorized! if session[:status] == 'connected' && channel.reauthorization_required?
    channel.prompt_reauthorization! if session[:status] == 'logged_out'
  end
end
