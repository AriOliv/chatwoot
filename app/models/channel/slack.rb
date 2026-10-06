# == Schema Information
#
# Table name: channel_slack
#
#  id          :bigint           not null, primary key
#  bot_token   :string           not null
#  scope       :text
#  settings    :jsonb            not null
#  team_name   :string
#  created_at  :datetime         not null
#  updated_at  :datetime         not null
#  account_id  :integer          not null
#  app_id      :string
#  bot_user_id :string           not null
#  team_id     :string           not null
#
# Indexes
#
#  index_channel_slack_on_team_id  (team_id) UNIQUE
#
# A Slack workspace as an inbox: DMs with the app bot and messages in monitored channels
# become conversations, and agent replies are posted back as the bot.
class Channel::Slack < ApplicationRecord
  include Channelable
  include Reauthorizable

  self.table_name = 'channel_slack'
  EDITABLE_ATTRS = [{ settings: [:accept_dms, :mention_only, { monitored_channel_ids: [] }] }].freeze

  # TODO: Remove guard once encryption keys become mandatory (target 3-4 releases out).
  encrypts :bot_token if Chatwoot.encryption_configured?

  validates :team_id, presence: true, uniqueness: true
  validates :bot_user_id, :bot_token, presence: true

  # The bot only receives events from channels it is a member of.
  after_update_commit :join_monitored_channels, if: :saved_change_to_settings?

  def name
    'Slack'
  end

  def client
    ::Slack::Web::Client.new(token: bot_token)
  end

  def monitored_channel_ids
    Array(settings['monitored_channel_ids'])
  end

  def accept_dms?
    settings['accept_dms'] != false
  end

  def mention_only?
    settings['mention_only'] == true
  end

  private

  def join_monitored_channels
    monitored_channel_ids.each do |channel_id|
      client.conversations_join(channel: channel_id)
    rescue ::Slack::Web::Api::Errors::SlackError => e
      # Private channels can't be joined by the bot; an admin has to /invite it.
      Rails.logger.warn("[SLACK_CHANNEL] could not join #{channel_id}: #{e.message}")
    end
  end
end
