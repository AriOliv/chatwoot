class Webhooks::SlackChannelEventsJob < MutexApplicationJob
  queue_as :default
  retry_on LockAcquisitionError, wait: 2.seconds, attempts: 8

  def perform(params)
    params = params.with_indifferent_access
    event = params[:event]
    return if event.blank? || duplicate?(params[:event_id])

    channel = Channel::Slack.find_by(team_id: params[:team_id])
    return if channel.blank? || !channel.account.active?

    thread_key = "#{event[:channel] || event.dig(:item, :channel)}:#{event[:thread_ts] || event[:ts]}"
    key = format(::Redis::Alfred::SLACK_CHANNEL_EVENT_MUTEX, team_id: channel.team_id, thread_key: thread_key)
    with_lock(key, 15.seconds) do
      SlackChannel::IncomingMessageService.new(channel: channel, event: event).perform
    end
  end

  private

  def duplicate?(event_id)
    return false if event_id.blank?

    key = format(::Redis::Alfred::SLACK_CHANNEL_EVENT_DEDUP, event_id: event_id)
    !Redis::Alfred.set(key, 1, nx: true, ex: 1.hour.to_i)
  end
end
