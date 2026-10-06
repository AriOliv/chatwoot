# Verifies Slack request signatures (https://api.slack.com/authentication/verifying-requests-from-slack).
module SlackSignatureVerifiable
  SIGNATURE_TOLERANCE = 5.minutes.to_i

  private

  def valid_slack_signature?(secret)
    timestamp = request.headers['X-Slack-Request-Timestamp']
    signature = request.headers['X-Slack-Signature']
    return false if timestamp.blank? || signature.blank?
    return false if (Time.current.to_i - timestamp.to_i).abs > SIGNATURE_TOLERANCE

    # Build over raw bytes so a payload with invalid UTF-8 can't raise on interpolation.
    basestring = "v0:#{timestamp}:".b << request.raw_post
    expected = "v0=#{OpenSSL::HMAC.hexdigest('SHA256', secret, basestring)}"
    ActiveSupport::SecurityUtils.secure_compare(expected, signature)
  end
end
