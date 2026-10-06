class Webhooks::SlackChannelController < ActionController::API
  include SlackSignatureVerifiable

  before_action :verify_signature!

  def events
    return render json: { challenge: params[:challenge] } if params[:type] == 'url_verification'
    # Slack retries when we don't ack within 3s; the first delivery is already enqueued.
    return head :ok if request.headers['X-Slack-Retry-Num'].present?

    Webhooks::SlackChannelEventsJob.perform_later(params.to_unsafe_hash.except(:controller, :action))
    head :ok
  end

  # Block Kit button clicks arrive form-encoded with a JSON `payload`.
  def interactivity
    payload = JSON.parse(params.require(:payload))
    SlackChannel::InteractionService.new(payload: payload.with_indifferent_access).perform if payload['type'] == 'block_actions'
    head :ok
  end

  private

  def verify_signature!
    secret = GlobalConfigService.load('SLACK_CHANNEL_SIGNING_SECRET', nil)
    head :unauthorized unless secret.present? && valid_slack_signature?(secret)
  end
end
