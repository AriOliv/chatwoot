class CreateChannelSlack < ActiveRecord::Migration[7.1]
  def change
    create_table :channel_slack do |t|
      t.integer :account_id, null: false
      t.string :team_id, null: false
      t.string :team_name
      t.string :app_id
      t.string :bot_user_id, null: false
      t.string :bot_token, null: false
      t.string :scope
      t.jsonb :settings, null: false, default: {}
      t.timestamps
    end
    add_index :channel_slack, :team_id, unique: true
  end
end
