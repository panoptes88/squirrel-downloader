<template>
  <a-space :size="2" class="task-actions">
    <a-button
      size="small"
      type="text"
      :aria-label="$t('查看任务 {id} 的日志', { id: task.id })"
      @click="$emit('log', task)"
    >
      {{ $t('日志') }}
    </a-button>

    <!-- 按状态给出唯一有意义的主操作 -->
    <a-button
      v-if="canCancel(task.status)"
      size="small"
      type="text"
      :aria-label="$t('取消任务 {id}', { id: task.id })"
      @click="$emit('cancel', task)"
    >
      {{ $t('取消') }}
    </a-button>
    <a-button
      v-else-if="canRetry(task.status)"
      size="small"
      type="text"
      class="retry-action"
      :aria-label="$t('重试任务 {id}', { id: task.id })"
      @click="$emit('retry', task)"
    >
      {{ $t('重试') }}
    </a-button>

    <a-dropdown :trigger="['click']">
      <a-button size="small" type="text" :aria-label="$t('任务 {id} 的更多操作', { id: task.id })">
        {{ $t('更多') }}
        <template #icon><down-outlined /></template>
      </a-button>
      <template #overlay>
        <a-menu @click="handleMenuClick">
          <a-menu-item v-if="showSaveProfile" key="save-profile">
            <save-outlined />
            <span style="margin-left: 8px;">{{ $t('保存为方案') }}</span>
          </a-menu-item>
          <a-menu-divider v-if="showSaveProfile" />
          <a-menu-item key="delete" danger>
            <delete-outlined />
            <span style="margin-left: 8px;">{{ $t('删除任务') }}</span>
          </a-menu-item>
        </a-menu>
      </template>
    </a-dropdown>
  </a-space>
</template>

<script setup>
import {
  DownOutlined,
  DeleteOutlined,
  SaveOutlined
} from '@ant-design/icons-vue'
import { canCancel, canRetry } from '../utils/task'

const props = defineProps({
  task: {
    type: Object,
    required: true
  },
  showSaveProfile: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits(['log', 'cancel', 'retry', 'save-profile', 'delete'])

// 下拉菜单项必须把 task 一起抛出：父组件的 saveAsProfile(task) 依赖它。
// 之前这里漏传，导致 task 为 undefined、读 task.id 抛错，
// 前端只表现为「方案保存失败」（后端其实从未被调用）。
function handleMenuClick({ key }) {
  if (key === 'save-profile') emit('save-profile', props.task)
  else if (key === 'delete') emit('delete', props.task)
}
</script>

<style scoped>
.task-actions {
  white-space: nowrap;
}

.retry-action {
  color: var(--brand);
}
</style>
