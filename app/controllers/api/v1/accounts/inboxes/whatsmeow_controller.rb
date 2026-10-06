class Api::V1::Accounts::Inboxes::WhatsmeowController < Api::V1::Accounts::BaseController
  before_action :fetch_inbox

  rescue_from Whatsapp::WhatsmeowSessionService::BridgeError do |e|
    render json: { error: e.message }, status: :unprocessable_entity
  end

  # `status` and `session` are ActionController methods, so the action and the helper use other names.
  def connection_status
    render json: bridge_session.status
  end

  def qr
    render json: bridge_session.qr
  end

  def pair_phone
    render json: bridge_session.pair_phone(params.require(:phone))
  end

  def reconnect
    bridge_session.register
    render json: bridge_session.reconnect
  end

  def logout
    bridge_session.logout
    bridge_session.register
    head :ok
  end

  private

  def fetch_inbox
    @inbox = Current.account.inboxes.find(params[:inbox_id])
    authorize @inbox, :update?
    return if @inbox.channel.try(:whatsmeow?)

    render json: { error: 'Only available for WhatsApp (QR code) inboxes' }, status: :unprocessable_entity
  end

  def bridge_session
    @inbox.channel.whatsmeow_session
  end
end
