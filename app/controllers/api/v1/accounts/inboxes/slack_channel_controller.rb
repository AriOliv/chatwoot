class Api::V1::Accounts::Inboxes::SlackChannelController < Api::V1::Accounts::BaseController
  before_action :fetch_inbox

  # Lists public and private channels the bot can see, for the monitored-channels picker.
  def channels
    channels = []
    cursor = nil
    loop do
      response = @inbox.channel.client.conversations_list(types: 'public_channel,private_channel', exclude_archived: true,
                                                          limit: 1000, cursor: cursor)
      channels.concat(response['channels'].map { |c| { id: c['id'], name: c['name'], is_private: c['is_private'], is_member: c['is_member'] } })
      cursor = response.dig('response_metadata', 'next_cursor')
      break if cursor.blank?
    end
    render json: { channels: channels.sort_by { |c| c[:name] } }
  rescue ::Slack::Web::Api::Errors::SlackError => e
    render json: { error: e.message }, status: :unprocessable_entity
  end

  private

  def fetch_inbox
    @inbox = Current.account.inboxes.find(params[:inbox_id])
    authorize @inbox, :update?
    render json: { error: 'Only available for Slack inboxes' }, status: :unprocessable_entity unless @inbox.slack?
  end
end
