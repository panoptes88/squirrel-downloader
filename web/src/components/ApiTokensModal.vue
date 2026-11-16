<template>
  <a-modal
    :open="open"
    :title="$t('API 令牌')"
    :footer="null"
    :width="720"
    @cancel="$emit('update:open', false)"
  >
    <p class="apidoc-hint">
      {{ $t('令牌用于让 Agent、脚本或工作流工具调用本应用的接口，无需登录。令牌只在创建时显示一次，请立即复制保存。') }}
    </p>

    <!-- 新建 -->
    <div class="create-row">
      <a-input
        v-model:value="form.name"
        :placeholder="$t('令牌名称，例如：Hermes 自动化')"
        :maxlength="32"
        style="flex: 1 1 200px"
      />
      <a-select v-model:value="form.expiresIn" style="width: 130px">
        <a-select-option v-for="o in expiryOptions" :key="o.value" :value="o.value">
          {{ $t(o.label) }}
        </a-select-option>
      </a-select>
      <a-button
        type="primary"
        :loading="creating"
        :disabled="tokenLimitReached"
        @click="handleCreate"
      >
        {{ $t('创建令牌') }}
      </a-button>
    </div>

    <!-- 新建成功后展示明文 -->
    <p v-if="tokenLimitReached" class="limit-hint">
      {{ $t('最多只能创建 5 个有效令牌，请先撤销不用的令牌') }}
    </p>

    <!-- 新建成功后展示完整令牌（关闭弹窗后不再可见） -->
    <div v-if="createdToken" class="token-result">
      <div class="token-result-head">
        <check-circle-filled class="token-result-icon" />
        <span>{{ $t('令牌已创建，请立即复制') }}</span>
      </div>
      <p class="token-result-hint">
        {{ $t('关闭弹窗后将无法再次查看，只能重新创建。') }}
      </p>
      <div class="token-result-box">
        <code class="token-result-text" @click="selectTokenText">{{ createdToken }}</code>
        <a-button type="primary" size="small" @click="copyToken">
          {{ copied ? $t('已复制') : $t('复制') }}
        </a-button>
      </div>
      <p class="token-result-usage">{{ $t('使用方式：') }}</p>
      <pre class="token-usage">curl -H "Authorization: Bearer {{ createdToken }}" {{ baseUrlHint }}/api/tasks</pre>
    </div>

    <!-- 列表 -->
    <a-table
      :data-source="tokens"
      :columns="columns"
      :loading="loading"
      :pagination="false"
      row-key="id"
      size="small"
      style="margin-top: 16px"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'name'">
          <div class="token-name">{{ record.name }}</div>
          <div class="token-prefix">{{ record.prefix }}…</div>
        </template>
        <template v-else-if="column.key === 'expires'">
          <span v-if="!record.expires_at">{{ $t('永久有效') }}</span>
          <span v-else :class="{ expired: isExpired(record) }">
            {{ formatTime(record.expires_at) }}
            <template v-if="isExpired(record)">（{{ $t('已过期') }}）</template>
          </span>
        </template>
        <template v-else-if="column.key === 'lastUsed'">
          {{ record.last_used_at ? formatTime(record.last_used_at) : $t('从未使用') }}
        </template>
        <template v-else-if="column.key === 'action'">
          <a-button type="link" danger size="small" @click="handleRevoke(record)">
            {{ $t('撤销') }}
          </a-button>
        </template>
      </template>
      <template #emptyText>
        <div class="empty">{{ $t('还没有创建任何令牌') }}</div>
      </template>
    </a-table>
  </a-modal>
</template>

<script setup>
import { ref, reactive, computed, watch } from 'vue'
import { message, Modal } from 'ant-design-vue'
import { CheckCircleFilled } from '@ant-design/icons-vue'
import { get, post, del } from '../api'
import { copyToClipboard } from '../utils/clipboard'
import { t, apiError } from '../i18n'

const props = defineProps({
  open: { type: Boolean, default: false }
})
defineEmits(['update:open'])

const loading = ref(false)
const creating = ref(false)
const tokens = ref([])
const createdToken = ref('')
const copied = ref(false)

const form = reactive({
  name: '',
  expiresIn: '1m'
})

// 有效期档位与服务端 internal/handler/apitoken.go 的 apiTokenTTLs 一一对应
const expiryOptions = [
  { value: '1d', label: '1 天' },
  { value: '1w', label: '1 星期' },
  { value: '1m', label: '1 个月' },
  { value: '3m', label: '3 个月' },
  { value: '6m', label: '6 个月' },
  { value: '1y', label: '1 年' },
  { value: 'never', label: '永久' }
]

const columns = computed(() => [
  { title: t('名称'), key: 'name', width: 200 },
  { title: t('有效期'), key: 'expires', width: 200 },
  { title: t('最后使用'), key: 'lastUsed', width: 170 },
  { title: t('操作'), key: 'action', width: 90 }
])

const baseUrlHint = computed(() => window.location.origin)

// 与后端 internal/handler/apitoken.go 的 MaxAPITokens 保持一致
const MAX_TOKENS = 5
const tokenLimitReached = computed(() => tokens.value.length >= MAX_TOKENS)

function formatTime(v) {
  if (!v) return '-'
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) return v
  const pad = n => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function isExpired(record) {
  if (!record.expires_at) return false
  return new Date(record.expires_at).getTime() < Date.now()
}

async function loadTokens() {
  loading.value = true
  try {
    tokens.value = await get('/tokens')
  } catch (err) {
    message.error(apiError(err, '读取令牌失败'))
  } finally {
    loading.value = false
  }
}

async function handleCreate() {
  if (tokenLimitReached.value) {
    message.warning(t('最多只能创建 5 个有效令牌，请先撤销不用的令牌'))
    return
  }
  if (!form.name.trim()) {
    message.warning(t('请填写令牌名称'))
    return
  }
  creating.value = true
  try {
    const res = await post('/tokens', {
      name: form.name.trim(),
      expires_in: form.expiresIn
    })
    createdToken.value = res.token
    copied.value = false
    form.name = ''
    await loadTokens()
    message.success(t('令牌创建成功'))
  } catch (err) {
    message.error(apiError(err, '创建令牌失败'))
  } finally {
    creating.value = false
  }
}

async function copyToken() {
  // copyToClipboard 内部优先用 Clipboard API，HTTP 下回退到 execCommand
  const ok = await copyToClipboard(createdToken.value)
  if (ok) {
    copied.value = true
    setTimeout(() => { copied.value = false }, 2000)
  }
}

// 点击令牌文本全选，方便手动复制（HTTP 下 execCommand 被浏览器禁用时的兜底）
function selectTokenText(e) {
  const range = document.createRange()
  range.selectNodeContents(e.target)
  const sel = window.getSelection()
  sel.removeAllRanges()
  sel.addRange(range)
}

function handleRevoke(record) {
  Modal.confirm({
    title: t('撤销这个令牌？'),
    content: t('撤销后使用该令牌的调用方会立即失效，且无法恢复。'),
    okText: t('撤销'),
    okType: 'danger',
    cancelText: t('取消'),
    onOk: async () => {
      try {
        await del(`/tokens/${record.id}`)
        message.success(t('令牌已撤销'))
        if (createdToken.value) createdToken.value = ''
        await loadTokens()
      } catch (err) {
        message.error(apiError(err, '撤销令牌失败'))
      }
    }
  })
}

watch(() => props.open, val => {
  if (val) {
    createdToken.value = ''
    loadTokens()
  }
})
</script>

<style scoped>
.apidoc-hint {
  margin: 0 0 16px;
  font-size: 13px;
  line-height: 1.7;
  color: var(--text-sub);
}

.limit-hint {
  margin: 10px 0 0;
  font-size: 13px;
  color: #d4380d;
}

.create-row {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.token-hint {
  margin-bottom: 8px;
  font-size: 13px;
}

.token-result {
  margin: 16px 0;
  padding: 16px;
  border: 1px solid #b7eb8f;
  background: #f6ffed;
  border-radius: 8px;
}

.token-result-head {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 15px;
  font-weight: 600;
  color: #237804;
}

.token-result-icon {
  font-size: 18px;
}

.token-result-hint {
  margin: 6px 0 12px;
  font-size: 13px;
  color: #5b6b57;
}

.token-result-box {
  display: flex;
  align-items: stretch;
  gap: 8px;
}

/* 完整展示令牌：等宽字体 + 自动换行，不截断 */
.token-result-text {
  flex: 1;
  min-width: 0;
  padding: 10px 12px;
  background: #fff;
  border: 1px solid #d9d9d9;
  border-radius: 6px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 13px;
  line-height: 1.5;
  word-break: break-all;
  cursor: text;
  user-select: all;
}

.token-result-usage {
  margin: 14px 0 6px;
  font-size: 13px;
  color: #5b6b57;
}

.token-usage {
  margin: 0;
  padding: 10px 12px;
  background: #1e1e2e;
  color: #cdd6f4;
  border-radius: 6px;
  font-size: 12px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-all;
}

.token-name {
  font-weight: 500;
}

.token-prefix {
  font-size: 12px;
  color: var(--text-sub);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}

.expired {
  color: #d4380d;
}

.empty {
  padding: 24px;
  text-align: center;
  color: var(--text-sub);
}
</style>
