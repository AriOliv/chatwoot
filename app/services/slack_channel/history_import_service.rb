# Imports the recent messages of a channel when it starts being monitored: each thread root
# and its replies go through IncomingMessageService in chronological order, keeping Slack timestamps.
class SlackChannel::HistoryImportService
  HISTORY_DAYS = 30

  pattr_initialize [:channel!, :slack_channel_id!]

  def perform
    roots.each do |root|
      import(root)
      replies(root).each { |reply| import(reply) } if root[:reply_count].to_i.positive?
    end
  rescue ::Slack::Web::Api::Errors::SlackError => e
    Rails.logger.warn("[SLACK_CHANNEL] history import failed for #{slack_channel_id}: #{e.message}")
  end

  private

  def roots
    messages = []
    channel.client.conversations_history(channel: slack_channel_id, oldest: HISTORY_DAYS.days.ago.to_i.to_s, limit: 200) do |page|
      messages.concat(page[:messages])
    end
    messages.sort_by { |m| m[:ts].to_f }
  end

  # The first entry of conversations.replies is the thread root itself.
  def replies(root)
    messages = []
    channel.client.conversations_replies(channel: slack_channel_id, ts: root[:ts], limit: 200) do |page|
      messages.concat(page[:messages])
    end
    messages.reject { |m| m[:ts] == root[:ts] }.sort_by { |m| m[:ts].to_f }
  end

  def import(message)
    event = message.to_h.merge('type' => 'message', 'channel' => slack_channel_id, 'channel_type' => 'channel').with_indifferent_access
    SlackChannel::IncomingMessageService.new(channel: channel, event: event, history: true).perform
  rescue StandardError => e
    Rails.logger.warn("[SLACK_CHANNEL] skipped history message #{message[:ts]} in #{slack_channel_id}: #{e.message}")
  end
end
