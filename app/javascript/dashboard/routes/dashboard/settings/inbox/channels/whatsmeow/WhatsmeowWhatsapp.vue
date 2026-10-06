<script setup>
import { computed, ref } from 'vue';
import { useStore } from 'vuex';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useVuelidate } from '@vuelidate/core';
import { required } from '@vuelidate/validators';
import { useAlert } from 'dashboard/composables';
import { isPhoneE164OrEmpty } from 'shared/helpers/Validators';
import NextButton from 'dashboard/components-next/button/Button.vue';
import Switch from 'dashboard/components-next/switch/Switch.vue';
import WhatsmeowPairing from './WhatsmeowPairing.vue';

const store = useStore();
const router = useRouter();
const { t } = useI18n();

const inboxName = ref('');
const phoneNumber = ref('');
const groupsEnabled = ref(true);
const importHistory = ref(true);
const nativeButtons = ref(true);
const createdInbox = ref(null);

const uiFlags = computed(() => store.getters['inboxes/getUIFlags']);
const v$ = useVuelidate(
  {
    inboxName: { required },
    phoneNumber: { required, isPhoneE164OrEmpty },
  },
  { inboxName, phoneNumber }
);

const createChannel = async () => {
  v$.value.$touch();
  if (v$.value.$invalid) return;

  try {
    createdInbox.value = await store.dispatch('inboxes/createChannel', {
      name: inboxName.value.trim(),
      channel: {
        type: 'whatsapp',
        provider: 'whatsmeow',
        phone_number: phoneNumber.value,
        provider_config: {
          groups_enabled: groupsEnabled.value,
          import_history: importHistory.value,
          interactive_mode: nativeButtons.value ? 'native' : 'text',
        },
      },
    });
  } catch (error) {
    useAlert(error.message || t('INBOX_MGMT.ADD.WHATSAPP.API.ERROR_MESSAGE'));
  }
};

const goToAgents = () => {
  router.replace({
    name: 'settings_inboxes_add_agents',
    params: { page: 'new', inbox_id: createdInbox.value.id },
  });
};
</script>

<template>
  <WhatsmeowPairing
    v-if="createdInbox"
    :inbox-id="createdInbox.id"
    :phone-number="phoneNumber"
    @connected="goToAgents"
  />
  <form
    v-else
    class="flex flex-wrap flex-col mx-0"
    @submit.prevent="createChannel"
  >
    <label :class="{ error: v$.inboxName.$error }">
      {{ $t('INBOX_MGMT.ADD.WHATSAPP.INBOX_NAME.LABEL') }}
      <input
        v-model="inboxName"
        type="text"
        :placeholder="$t('INBOX_MGMT.ADD.WHATSAPP.INBOX_NAME.PLACEHOLDER')"
        @blur="v$.inboxName.$touch"
      />
      <span v-if="v$.inboxName.$error" class="message">
        {{ $t('INBOX_MGMT.ADD.WHATSAPP.INBOX_NAME.ERROR') }}
      </span>
    </label>

    <label :class="{ error: v$.phoneNumber.$error }">
      {{ $t('INBOX_MGMT.ADD.WHATSAPP.PHONE_NUMBER.LABEL') }}
      <input
        v-model="phoneNumber"
        type="text"
        :placeholder="$t('INBOX_MGMT.ADD.WHATSAPP.PHONE_NUMBER.PLACEHOLDER')"
        @blur="v$.phoneNumber.$touch"
      />
      <span v-if="v$.phoneNumber.$error" class="message">
        {{ $t('INBOX_MGMT.ADD.WHATSAPP.PHONE_NUMBER.ERROR') }}
      </span>
    </label>

    <div class="flex flex-col gap-3 mb-6">
      <div class="flex items-center gap-2">
        <Switch v-model="groupsEnabled" />
        <span class="text-body-main text-n-slate-12">
          {{ $t('INBOX_MGMT.WHATSMEOW.GROUPS_ENABLED') }}
        </span>
      </div>
      <div class="flex items-center gap-2">
        <Switch v-model="importHistory" />
        <span class="text-body-main text-n-slate-12">
          {{ $t('INBOX_MGMT.WHATSMEOW.IMPORT_HISTORY') }}
        </span>
      </div>
      <div class="flex items-center gap-2">
        <Switch v-model="nativeButtons" />
        <span class="text-body-main text-n-slate-12">
          {{ $t('INBOX_MGMT.WHATSMEOW.NATIVE_BUTTONS') }}
        </span>
      </div>
    </div>

    <div class="w-full">
      <NextButton
        type="submit"
        :label="$t('INBOX_MGMT.WHATSMEOW.CREATE_AND_PAIR')"
        :is-loading="uiFlags.isCreating"
      />
    </div>
  </form>
</template>
