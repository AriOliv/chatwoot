<script setup>
import { ref, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import slackChannelClient from 'dashboard/api/channel/slackChannelClient';
import Button from 'dashboard/components-next/button/Button.vue';
import { useAlert } from 'dashboard/composables';

const { t } = useI18n();
const errorMessage = ref('');
const isRequestingAuthorization = ref(false);

onMounted(() => {
  const urlParams = new URLSearchParams(window.location.search);
  errorMessage.value = urlParams.get('error_message') || '';
  window.history.replaceState({}, document.title, window.location.pathname);
});

const requestAuthorization = async () => {
  isRequestingAuthorization.value = true;
  try {
    const {
      data: { url },
    } = await slackChannelClient.generateAuthorization();
    window.location.href = url;
  } catch (error) {
    isRequestingAuthorization.value = false;
    useAlert(t('INBOX_MGMT.ADD.SLACK_CHANNEL.ERROR_MESSAGE'));
  }
};
</script>

<template>
  <div class="h-full p-6 w-full max-w-full flex-shrink-0 flex-grow-0">
    <div
      class="flex flex-col items-center justify-center w-full px-8 py-10 text-center rounded-2xl outline outline-1 outline-n-weak"
    >
      <div class="flex flex-col items-center w-full max-w-2xl">
        <h6 class="text-2xl font-medium">
          {{ $t('INBOX_MGMT.ADD.SLACK_CHANNEL.TITLE') }}
        </h6>
        <p class="max-w-xl py-6 text-sm text-n-slate-11">
          {{ $t('INBOX_MGMT.ADD.SLACK_CHANNEL.HELP') }}
        </p>
        <p v-if="errorMessage" class="pb-4 text-sm text-n-ruby-11">
          {{ errorMessage }}
        </p>
        <Button
          lg
          icon="i-ri-slack-line"
          :is-loading="isRequestingAuthorization"
          :label="$t('INBOX_MGMT.ADD.SLACK_CHANNEL.CONNECT')"
          @click="requestAuthorization"
        />
      </div>
    </div>
  </div>
</template>
