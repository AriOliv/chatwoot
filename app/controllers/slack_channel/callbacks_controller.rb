class SlackChannel::CallbacksController < ApplicationController
  include SlackChannel::IntegrationHelper

  def show
    return redirect_to_error(params[:error]) if params[:error].present?

    account_id = verify_slack_channel_state(params[:state])
    return redirect_to_error('invalid_state') if account_id.blank?

    @account = Account.find(account_id)
    inbox, already_exists = find_or_create_inbox
    path = already_exists ? "settings/inboxes/#{inbox.id}" : "settings/inboxes/new/#{inbox.id}/agents"
    redirect_to "#{ENV.fetch('FRONTEND_URL', '')}/app/accounts/#{@account.id}/#{path}"
  rescue CustomExceptions::Inbox::LimitExceeded, ::Slack::Web::Api::Errors::SlackError => e
    Rails.logger.error("Slack channel creation error: #{e.message}")
    redirect_to_error(e.message)
  end

  private

  def find_or_create_inbox
    channel = Channel::Slack.find_by(team_id: oauth[:team][:id], account: @account)
    exists = channel.present?
    exists ? channel.update!(channel_attributes) : channel = create_channel_with_inbox
    channel.reauthorized!
    [channel.inbox, exists]
  end

  def create_channel_with_inbox
    ActiveRecord::Base.transaction do
      channel = @account.slack_channels.create!(channel_attributes.merge(team_id: oauth[:team][:id]))
      @account.inboxes.create!(channel: channel, name: "Slack - #{oauth[:team][:name]}")
      channel
    end
  end

  def channel_attributes
    { bot_user_id: oauth[:bot_user_id], bot_token: oauth[:access_token], scope: oauth[:scope],
      app_id: oauth[:app_id], team_name: oauth[:team][:name] }
  end

  def oauth
    @oauth ||= ::Slack::Web::Client.new.oauth_v2_access(
      client_id: client_id, client_secret: client_secret, code: params[:code], redirect_uri: slack_channel_redirect_uri
    ).with_indifferent_access
  end

  def redirect_to_error(message)
    account_path = @account ? "app/accounts/#{@account.id}/settings/inboxes/new/slack" : 'app'
    redirect_to "#{ENV.fetch('FRONTEND_URL', '')}/#{account_path}?error_message=#{ERB::Util.url_encode(message)}"
  end
end
