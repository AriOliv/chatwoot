<script setup>
import { ref } from 'vue';
import { useStore } from 'vuex';
import { useI18n } from 'vue-i18n';
import { useAlert } from 'dashboard/composables';
import WhatsmeowAPI from 'dashboard/api/channel/whatsmeowClient';
import SettingsFieldSection from 'dashboard/components-next/Settings/SettingsFieldSection.vue';
import SettingsToggleSection from 'dashboard/components-next/Settings/SettingsToggleSection.vue';
import NextButton from 'dashboard/components-next/button/Button.vue';
import WhatsmeowPairing from './WhatsmeowPairing.vue';

const props = defineProps({
  inbox: { type: Object, required: true },
});

const store = useStore();
const { t } = useI18n();
const config = props.inbox.provider_config || {};
const groupsEnabled = ref(config.groups_enabled !== false);
const importHistory = ref(config.import_history !== false);
const nativeButtons = ref(config.interactive_mode !== 'text');
const pairingKey = ref(0);
const isBusy = ref(false);

const saveConfig = async () => {
  try {
    await store.dispatch('inboxes/updateInbox', {
      id: props.inbox.id,
      formData: false,
      channel: {
        provider_config: {
          ...props.inbox.provider_config,
          groups_enabled: groupsEnabled.value,
          import_history: importHistory.value,
          interactive_mode: nativeButtons.value ? 'native' : 'text',
        },
      },
    });
    useAlert(t('INBOX_MGMT.EDIT.API.SUCCESS_MESSAGE'));
  } catch (error) {
    useAlert(t('INBOX_MGMT.EDIT.API.ERROR_MESSAGE'));
  }
};

const runAction = async action => {
  isBusy.value = true;
  try {
    await action(props.inbox.id);
    pairingKey.value += 1;
  } catch (error) {
    useAlert(error.response?.data?.error || t('INBOX_MGMT.WHATSMEOW.ERROR'));
  } finally {
    isBusy.value = false;
  }
};
</script>

<template>
  <div class="flex flex-col gap-6">
    <SettingsFieldSection
      :label="$t('INBOX_MGMT.WHATSMEOW.CONNECTION_TITLE')"
      :help-text="$t('INBOX_MGMT.WHATSMEOW.CONNECTION_HELP')"
    >
      <WhatsmeowPairing
        :key="pairingKey"
        :inbox-id="inbox.id"
        :phone-number="inbox.phone_number"
      />
      <div class="flex gap-2 mt-4">
        <NextButton
          sm
          faded
          :is-loading="isBusy"
          :label="$t('INBOX_MGMT.WHATSMEOW.RECONNECT')"
          @click="runAction(WhatsmeowAPI.reconnect.bind(WhatsmeowAPI))"
        />
        <NextButton
          sm
          faded
          ruby
          :is-loading="isBusy"
          :label="$t('INBOX_MGMT.WHATSMEOW.LOGOUT')"
          @click="runAction(WhatsmeowAPI.logout.bind(WhatsmeowAPI))"
        />
      </div>
    </SettingsFieldSection>

    <SettingsToggleSection
      v-model="groupsEnabled"
      :header="$t('INBOX_MGMT.WHATSMEOW.GROUPS_ENABLED')"
      :description="$t('INBOX_MGMT.WHATSMEOW.GROUPS_ENABLED_HELP')"
      @update:model-value="saveConfig"
    />
    <SettingsToggleSection
      v-model="importHistory"
      :header="$t('INBOX_MGMT.WHATSMEOW.IMPORT_HISTORY')"
      :description="$t('INBOX_MGMT.WHATSMEOW.IMPORT_HISTORY_HELP')"
      @update:model-value="saveConfig"
    />
    <SettingsToggleSection
      v-model="nativeButtons"
      :header="$t('INBOX_MGMT.WHATSMEOW.NATIVE_BUTTONS')"
      :description="$t('INBOX_MGMT.WHATSMEOW.NATIVE_BUTTONS_HELP')"
      @update:model-value="saveConfig"
    />
  </div>
</template>
