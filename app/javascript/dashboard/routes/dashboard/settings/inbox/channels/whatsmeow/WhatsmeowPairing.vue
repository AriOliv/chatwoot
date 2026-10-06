<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import QRCode from 'qrcode';
import WhatsmeowAPI from 'dashboard/api/channel/whatsmeowClient';
import NextButton from 'dashboard/components-next/button/Button.vue';
import Input from 'dashboard/components-next/input/Input.vue';
import { useAlert } from 'dashboard/composables';

const props = defineProps({
  inboxId: { type: [Number, String], required: true },
  phoneNumber: { type: String, default: '' },
});
const emit = defineEmits(['connected']);

const POLL_INTERVAL_MS = 3000;
const MODES = { QR: 'qr', CODE: 'code' };

const { t } = useI18n();
const status = ref('');
const pairedJid = ref('');
const qrDataUrl = ref('');
const pairingCode = ref('');
const pairingPhone = ref(props.phoneNumber);
const mode = ref(MODES.QR);
const isLoading = ref(false);
let pollTimer = null;

const isConnected = computed(() => status.value === 'connected');

const statusLabel = computed(() => {
  const labels = {
    connected: t('INBOX_MGMT.WHATSMEOW.STATUS.connected'),
    disconnected: t('INBOX_MGMT.WHATSMEOW.STATUS.disconnected'),
    qr_pending: t('INBOX_MGMT.WHATSMEOW.STATUS.qr_pending'),
    logged_out: t('INBOX_MGMT.WHATSMEOW.STATUS.logged_out'),
    unavailable: t('INBOX_MGMT.WHATSMEOW.STATUS.unavailable'),
  };
  return labels[status.value] || t('INBOX_MGMT.WHATSMEOW.STATUS.unknown');
});

const pairedNumber = computed(() =>
  /^\d+$/.test(pairedJid.value) ? `+${pairedJid.value}` : pairedJid.value
);

const renderQr = async code => {
  qrDataUrl.value = code ? await QRCode.toDataURL(code, { margin: 1 }) : '';
};

const refreshStatus = async () => {
  try {
    const { data } = await WhatsmeowAPI.status(props.inboxId);
    status.value = data.status;
    pairedJid.value = data.jid || '';
    if (data.status === 'qr_pending' && mode.value === MODES.QR) {
      await renderQr(data.qr);
    }
    if (data.status === 'connected') {
      qrDataUrl.value = '';
      emit('connected', data);
    }
  } catch (error) {
    status.value = 'unavailable';
  }
};

const startPolling = () => {
  clearInterval(pollTimer);
  pollTimer = setInterval(refreshStatus, POLL_INTERVAL_MS);
};

const requestQr = async () => {
  isLoading.value = true;
  try {
    const { data } = await WhatsmeowAPI.qr(props.inboxId);
    await renderQr(data.qr);
    status.value = 'qr_pending';
    startPolling();
  } catch (error) {
    useAlert(error.response?.data?.error || t('INBOX_MGMT.WHATSMEOW.ERROR'));
  } finally {
    isLoading.value = false;
  }
};

const requestPairingCode = async () => {
  isLoading.value = true;
  try {
    const { data } = await WhatsmeowAPI.pairPhone(
      props.inboxId,
      pairingPhone.value
    );
    pairingCode.value = data.code;
    startPolling();
  } catch (error) {
    useAlert(error.response?.data?.error || t('INBOX_MGMT.WHATSMEOW.ERROR'));
  } finally {
    isLoading.value = false;
  }
};

onMounted(async () => {
  await refreshStatus();
  if (status.value === 'logged_out') await requestQr();
  startPolling();
});

onBeforeUnmount(() => clearInterval(pollTimer));
</script>

<template>
  <div class="flex flex-col gap-4">
    <div class="flex items-center gap-2 text-body-main text-n-slate-12">
      <span
        class="size-2 rounded-full"
        :class="isConnected ? 'bg-n-teal-9' : 'bg-n-amber-9'"
      />
      <span>{{ statusLabel }}</span>
      <span v-if="pairedJid" class="text-n-slate-11">{{ pairedNumber }}</span>
    </div>

    <template v-if="!isConnected">
      <div class="flex gap-2">
        <NextButton
          sm
          :faded="mode !== 'qr'"
          :label="$t('INBOX_MGMT.WHATSMEOW.MODE_QR')"
          @click="mode = 'qr'"
        />
        <NextButton
          sm
          :faded="mode !== 'code'"
          :label="$t('INBOX_MGMT.WHATSMEOW.MODE_CODE')"
          @click="mode = 'code'"
        />
      </div>

      <div v-if="mode === 'qr'" class="flex flex-col items-start gap-3">
        <p class="text-body-main text-n-slate-11">
          {{ $t('INBOX_MGMT.WHATSMEOW.QR_INSTRUCTIONS') }}
        </p>
        <img
          v-if="qrDataUrl"
          :src="qrDataUrl"
          :alt="$t('INBOX_MGMT.WHATSMEOW.QR_ALT')"
          class="size-64 rounded-lg border border-n-weak bg-white p-2"
        />
        <NextButton
          sm
          :is-loading="isLoading"
          :label="$t('INBOX_MGMT.WHATSMEOW.GENERATE_QR')"
          @click="requestQr"
        />
      </div>

      <div v-else class="flex flex-col items-start gap-3">
        <p class="text-body-main text-n-slate-11">
          {{ $t('INBOX_MGMT.WHATSMEOW.CODE_INSTRUCTIONS') }}
        </p>
        <Input
          v-model="pairingPhone"
          class="w-full max-w-xs"
          :label="$t('INBOX_MGMT.WHATSMEOW.PHONE_LABEL')"
          placeholder="+5511999999999"
        />
        <NextButton
          sm
          :is-loading="isLoading"
          :label="$t('INBOX_MGMT.WHATSMEOW.GENERATE_CODE')"
          @click="requestPairingCode"
        />
        <p
          v-if="pairingCode"
          class="font-mono text-2xl tracking-widest text-n-slate-12"
        >
          {{ pairingCode }}
        </p>
      </div>
    </template>
  </div>
</template>
