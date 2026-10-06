FactoryBot.define do
  factory :channel_slack, class: 'Channel::Slack' do
    account
    sequence(:team_id) { |n| "T#{n.to_s.rjust(8, '0')}" }
    team_name { 'Acme' }
    bot_user_id { 'UBOT' }
    bot_token { 'xoxb-test' }
    settings { { 'accept_dms' => true, 'monitored_channel_ids' => ['CSUPPORT'] } }

    after(:create) do |channel|
      create(:inbox, channel: channel, account: channel.account)
    end
  end
end
