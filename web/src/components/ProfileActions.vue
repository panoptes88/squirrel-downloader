<template>
  <a-space :size="2" class="profile-actions">
    <a-button
      size="small"
      type="text"
      :aria-label="$t('加载方案 {name}', { name: profile.name })"
      @click="$emit('load', profile)"
    >
      {{ $t('加载') }}
    </a-button>
    <a-button
      size="small"
      type="text"
      :aria-label="$t('查看方案 {name} 的详情', { name: profile.name })"
      @click="$emit('detail', profile)"
    >
      {{ $t('详情') }}
    </a-button>

    <a-dropdown :trigger="['click']">
      <a-button size="small" type="text" :aria-label="$t('方案 {name} 的更多操作', { name: profile.name })">
        {{ $t('更多') }}
        <template #icon><down-outlined /></template>
      </a-button>
      <template #overlay>
        <a-menu @click="handleMenuClick">
          <a-menu-item key="rename">
            <edit-outlined />
            <span style="margin-left: 8px;">{{ $t('重命名') }}</span>
          </a-menu-item>
          <a-menu-divider />
          <a-menu-item key="delete" danger>
            <delete-outlined />
            <span style="margin-left: 8px;">{{ $t('删除方案') }}</span>
          </a-menu-item>
        </a-menu>
      </template>
    </a-dropdown>
  </a-space>
</template>

<script setup>
import { DownOutlined, DeleteOutlined, EditOutlined } from '@ant-design/icons-vue'

const props = defineProps({
  profile: {
    type: Object,
    required: true
  }
})

const emit = defineEmits(['load', 'detail', 'rename', 'delete'])

// 与 TaskActions 一致：下拉项要把 profile 一起抛出
function handleMenuClick({ key }) {
  if (key === 'rename') emit('rename', props.profile)
  else if (key === 'delete') emit('delete', props.profile)
}
</script>

<style scoped>
.profile-actions {
  white-space: nowrap;
}
</style>
