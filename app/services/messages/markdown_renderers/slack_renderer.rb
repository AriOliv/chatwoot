# Slack mrkdwn: *bold*, _italic_, `code`, <url|text> links, ``` blocks.
class Messages::MarkdownRenderers::SlackRenderer < Messages::MarkdownRenderers::WhatsAppRenderer
  def link(node)
    text = node.first_child&.string_content
    out(text.present? && text != node.url ? "<#{node.url}|#{text}>" : "<#{node.url}>")
  end

  def code_block(node)
    out("```\n", node.string_content, '```')
  end
end
