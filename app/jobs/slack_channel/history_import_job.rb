class SlackChannel::HistoryImportJob < ApplicationJob
  queue_as :low

  def perform(channel, slack_channel_id)
    SlackChannel::HistoryImportService.new(channel: channel, slack_channel_id: slack_channel_id).perform
  end
end
