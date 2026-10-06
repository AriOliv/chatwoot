<script setup>
import { computed, onMounted, ref } from 'vue';
import { useStore } from 'vuex';
import { useI18n } from 'vue-i18n';
import { useAlert } from 'dashboard/composables';
import slackChannelClient from 'dashboard/api/channel/slackChannelClient';
import SettingsFieldSection from 'dashboard/components-next/Settings/SettingsFieldSection.vue';
import SettingsToggleSection from 'dashboard/components-next/Settings/SettingsToggleSection.vue';
import NextButton from 'dashboard/components-next/button/Button.vue';

const props = defineProps({
  inbox: { type: Object, required: true },
});

const store = useStore();
const { t } = useI18n();
const settings = props.inbox.settings || {};
const acceptDms = ref(settings.accept_dms !== false);
const mentionOnly = ref(settings.mention_only === true);
const monitoredIds = ref([...(settings.monitored_channel_ids || [])]);
const channels = ref([]);
const search = ref('');
const isLoading = ref(false);
const isSaving = ref(false);

const filteredChannels = computed(() => {
  const term = search.value.trim().toLowerCase();
  return term
    ? channels.value.filter(c => c.name.toLowerCase().includes(term))
    : channels.value;
});

const loadChannels = async () => {
  isLoading.value = true;
  try {
    const { data } = await slackChannelClient.listChannels(props.inbox.id);
    channels.value = data.channels;
  } catch (error) {
    useAlert(t('INBOX_MGMT.SLACK_CHANNEL.LOAD_ERROR'));
  } finally {
    isLoading.value = false;
  }
};

const toggleChannel = id => {
  monitoredIds.value = monitoredIds.value.includes(id)
    ? monitoredIds.value.filter(c => c !== id)
    : [...monitoredIds.value, id];
};

const save = async () => {
  isSaving.value = true;
  try {
    await store.dispatch('inboxes/updateInbox', {
      id: props.inbox.id,
      formData: false,
      channel: {
        settings: {
          accept_dms: acceptDms.value,
          mention_only: mentionOnly.value,
          monitored_channel_ids: monitoredIds.value,
        },
      },
    });
    useAlert(t('INBOX_MGMT.EDIT.API.SUCCESS_MESSAGE'));
  } catch (error) {
    useAlert(t('INBOX_MGMT.EDIT.API.ERROR_MESSAGE'));
  } finally {
    isSaving.value = false;
  }
};

onMounted(loadChannels);
</script>

<template>
  <div class="flex flex-col gap-6">
    <SettingsToggleSection
      v-model="acceptDms"
      :header="$t('INBOX_MGMT.SLACK_CHANNEL.ACCEPT_DMS')"
      :description="$t('INBOX_MGMT.SLACK_CHANNEL.ACCEPT_DMS_HELP')"
    />
    <SettingsToggleSection
      v-model="mentionOnly"
      :header="$t('INBOX_MGMT.SLACK_CHANNEL.MENTION_ONLY')"
      :description="$t('INBOX_MGMT.SLACK_CHANNEL.MENTION_ONLY_HELP')"
    />
    <SettingsFieldSection
      :label="$t('INBOX_MGMT.SLACK_CHANNEL.MONITORED_CHANNELS')"
      :help-text="$t('INBOX_MGMT.SLACK_CHANNEL.MONITORED_CHANNELS_HELP')"
    >
      <input
        v-model="search"
        type="text"
        class="mb-2"
        :placeholder="$t('INBOX_MGMT.SLACK_CHANNEL.SEARCH_PLACEHOLDER')"
      />
      <p v-if="isLoading" class="text-body-main text-n-slate-11">
        {{ $t('INBOX_MGMT.SLACK_CHANNEL.LOADING') }}
      </p>
      <ul
        v-else
        class="flex flex-col gap-1 overflow-y-auto max-h-72 border border-n-weak rounded-lg p-2"
      >
        <li
          v-for="slackChannel in filteredChannels"
          :key="slackChannel.id"
          class="flex items-center gap-2"
        >
          <input
            :id="`slack-channel-${slackChannel.id}`"
            type="checkbox"
            :checked="monitoredIds.includes(slackChannel.id)"
            @change="toggleChannel(slackChannel.id)"
          />
          <label
            :for="`slack-channel-${slackChannel.id}`"
            class="text-body-main text-n-slate-12"
          >
            {{ slackChannel.name }}
          </label>
          <span
            v-if="slackChannel.is_private"
            class="text-label-small text-n-slate-11"
          >
            {{ $t('INBOX_MGMT.SLACK_CHANNEL.PRIVATE_HINT') }}
          </span>
        </li>
      </ul>
    </SettingsFieldSection>
    <div>
      <NextButton
        :is-loading="isSaving"
        :label="$t('INBOX_MGMT.SLACK_CHANNEL.SAVE')"
        @click="save"
      />
    </div>
  </div>
</template>
