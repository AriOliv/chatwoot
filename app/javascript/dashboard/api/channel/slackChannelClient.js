/* global axios */
import ApiClient from '../ApiClient';

class SlackChannelClient extends ApiClient {
  constructor() {
    super('slack_channel', { accountScoped: true });
  }

  generateAuthorization() {
    return axios.post(`${this.url}/authorization`);
  }

  listChannels(inboxId) {
    return axios.get(
      `${this.baseUrl()}/inboxes/${inboxId}/slack_channel/channels`
    );
  }
}

export default new SlackChannelClient();
