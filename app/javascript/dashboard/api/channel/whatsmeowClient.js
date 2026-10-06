/* global axios */
import ApiClient from '../ApiClient';

class WhatsmeowClient extends ApiClient {
  constructor() {
    super('inboxes', { accountScoped: true });
  }

  status(inboxId) {
    return axios.get(`${this.url}/${inboxId}/whatsmeow/status`);
  }

  qr(inboxId) {
    return axios.get(`${this.url}/${inboxId}/whatsmeow/qr`);
  }

  pairPhone(inboxId, phone) {
    return axios.post(`${this.url}/${inboxId}/whatsmeow/pair_phone`, { phone });
  }

  reconnect(inboxId) {
    return axios.post(`${this.url}/${inboxId}/whatsmeow/reconnect`);
  }

  logout(inboxId) {
    return axios.delete(`${this.url}/${inboxId}/whatsmeow/logout`);
  }
}

export default new WhatsmeowClient();
