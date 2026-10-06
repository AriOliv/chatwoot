class ChangeChannelSlackScopeToText < ActiveRecord::Migration[7.1]
  def up
    change_column :channel_slack, :scope, :text
  end

  def down
    change_column :channel_slack, :scope, :string
  end
end
