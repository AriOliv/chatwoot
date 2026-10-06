class Api::V1::Accounts::SlackChannel::AuthorizationsController < Api::V1::Accounts::OauthAuthorizationController
  include SlackChannel::IntegrationHelper

  def create
    return render json: { success: false }, status: :unprocessable_entity if client_id.blank? || client_secret.blank?

    render json: { success: true, url: slack_channel_authorize_url(generate_slack_channel_state(Current.account.id)) }
  end
end
