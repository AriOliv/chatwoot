# WhatsApp Web (multi-device) provider backed by the whatsmeow-bridge service.
# Recipients are the contact inbox source ids: phone digits, "<id>@g.us" groups or "<id>@lid".
class Whatsapp::Providers::WhatsmeowService < Whatsapp::Providers::BaseService
  attr_reader :last_error

  def send_message(phone_number, message)
    @message = message
    if message.attachments.present?
      send_attachments(phone_number, message)
    elsif %w[input_select cards].include?(message.content_type)
      send_interactive(phone_number, message)
    else
      deliver(phone_number, message, type: 'text', text: message.outgoing_content)
    end
  end

  # whatsmeow has no approved templates; the rendered template text is sent as a regular message.
  def send_template(phone_number, _template_info, message)
    deliver(phone_number, message, type: 'text', text: message.outgoing_content)
  end

  def sync_templates
    whatsapp_channel.mark_message_templates_updated
  end

  def validate_provider_config?
    session.healthy?
  end

  def api_headers
    session.headers
  end

  def media_url(message_id)
    session.media_url(message_id)
  end

  def error_message(response)
    response.try(:[], 'error') || response.to_s
  end

  private

  def session
    @session ||= whatsapp_channel.whatsmeow_session
  end

  def deliver(recipient, message, payload)
    response = session.send_message(payload.merge(to: recipient, reply_to_id: reply_to_id(message)).compact)
    record_fallback(message) if response['fallback']
    response['id']
  rescue StandardError => e
    @last_error = e.message
    message.update!(status: :failed, external_error: e.message)
    nil
  end

  # Caption goes with the first attachment; the first id becomes the message source id.
  def send_attachments(recipient, message)
    ids = message.attachments.each_with_index.map do |attachment, index|
      deliver(recipient, message, {
                type: attachment_type(attachment),
                media_url: attachment.download_url,
                filename: attachment.file.filename.to_s,
                mime: attachment.file.content_type,
                text: index.zero? ? message.outgoing_content : nil
              })
    end
    ids.compact.first
  end

  def attachment_type(attachment)
    case attachment.file_type
    when 'image', 'video' then attachment.file_type
    when 'audio' then attachment.file.content_type.to_s.include?('ogg') ? 'voice' : 'audio'
    else 'document'
    end
  end

  def send_interactive(recipient, message)
    deliver(recipient, message, {
              type: 'interactive',
              text: interactive_body(message),
              interactive: interactive_spec(message),
              interactive_mode: whatsapp_channel.provider_config['interactive_mode'].presence || 'native'
            })
  end

  def interactive_body(message)
    return message.outgoing_content unless message.content_type == 'cards'

    message.content_attributes['items'].map { |card| [card['title'], card['description']].compact_blank.join("\n") }.join("\n\n")
  end

  def interactive_spec(message)
    options = interactive_options(message)
    return { buttons: options } if options.length <= 3

    { list: { button_text: I18n.t('conversations.messages.whatsapp.list_button_label'), sections: [{ rows: options }] } }
  end

  def interactive_options(message)
    items = message.content_attributes['items']
    if message.content_type == 'cards'
      items.flat_map { |card| card['actions'].to_a }.map { |action| { id: action['payload'] || action['uri'], title: action['text'] } }
    else
      items.map { |item| { id: item['value'], title: item['title'] } }
    end
  end

  # Numbered replies to a text fallback are mapped back to these options on the way in.
  def record_fallback(message)
    message.update!(content_attributes: message.content_attributes.merge('whatsmeow_fallback_options' => interactive_options(message)))
  end

  def reply_to_id(message)
    message.content_attributes[:in_reply_to_external_id].presence
  end
end
