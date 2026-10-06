class Api::V1::Accounts::Inboxes::WhatsmeowController < Api::V1::Accounts::BaseController
  before_action :fetch_inbox

  rescue_from Whatsapp::WhatsmeowSessionService::BridgeError do |e|
    render json: { error: e.message }, status: :unprocessable_entity
  end

  def status
    render json: session.status
  end

  def qr
    render json: session.qr
  end

  def pair_phone
    render json: session.pair_phone(params.require(:phone))
  end

  def reconnect
    session.register
    render json: session.reconnect
  end

  def logout
    session.logout
    session.register
    head :ok
  end

  private

  def fetch_inbox
    @inbox = Current.account.inboxes.find(params[:inbox_id])
    authorize @inbox, :update?
    return if @inbox.channel.try(:whatsmeow?)

    render json: { error: 'Only available for WhatsApp (QR code) inboxes' }, status: :unprocessable_entity
  end

  def session
    @inbox.channel.whatsmeow_session
  end
end
