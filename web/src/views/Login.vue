<template>
  <div class="login-page">
    <div class="login-backdrop" aria-hidden="true" />
    <div class="login-main">
      <div class="login-brand">
        <img class="brand-tile" :src="logo" :alt="$t('松鼠下载器')" />
        <div class="brand-name">{{ $t('松鼠下载器') }}</div>
        <div class="brand-sub">Squirrel Downloader</div>
      </div>

      <div class="login-language">
        <a-button size="small" @click="appStore.setLanguage(appStore.language === 'zh-CN' ? 'en-US' : 'zh-CN')">
          {{ appStore.language === 'zh-CN' ? 'English' : '中文' }}
        </a-button>
      </div>
      <a-config-provider :theme="cardTheme" :locale="antdLocale">
        <div class="login-card">
          <div class="login-card-title">{{ $t('登录') }}</div>
          <a-form
            :model="formState"
            :rules="rules"
            layout="vertical"
            @finish="handleSubmit"
          >
            <a-form-item :label="$t('用户名')" name="username">
              <a-input
                v-model:value="formState.username"
                :placeholder="$t('请输入用户名')"
                size="large"
              >
                <template #prefix>
                  <UserOutlined class="field-icon" />
                </template>
              </a-input>
            </a-form-item>
            <a-form-item :label="$t('密码')" name="password">
              <a-input-password
                v-model:value="formState.password"
                :placeholder="$t('请输入密码')"
                size="large"
                @press-enter="handleSubmit"
              >
                <template #prefix>
                  <LockOutlined class="field-icon" />
                </template>
              </a-input-password>
            </a-form-item>
            <a-form-item style="margin-bottom: 12px">
              <a-checkbox v-model:checked="agreed">
                {{ $t('我已阅读并同意') }}
                <a href="/privacy.html" target="_blank" rel="noopener" @click.stop>
                  {{ $t('《隐私政策》') }}
                </a>
              </a-checkbox>
            </a-form-item>
            <a-form-item style="margin-bottom: 0">
              <a-button
                type="primary"
                html-type="submit"
                :loading="loading"
                block
                size="large"
                class="login-button"
              >
                {{ $t('登录') }}
              </a-button>
            </a-form-item>
          </a-form>
        </div>
      </a-config-provider>
          <div class="login-footer">
            <a href="/privacy.html" target="_blank" rel="noopener">{{ $t('隐私政策') }}</a>
            <span class="sep">·</span>
            <a href="https://github.com/panoptes88/squirrel-downloader" target="_blank" rel="noopener">GitHub</a>
          </div>
    </div>
  </div>

    <!-- 首次访问：隐私政策提示，须明确同意后才能登录 -->
    <a-modal
      v-model:open="showConsent"
      :closable="false"
      :mask-closable="false"
      :keyboard="false"
      :width="520"
      centered
    >
      <template #title>{{ $t('隐私政策与数据说明') }}</template>
      <div class="consent-body">
        <p>{{ $t('松鼠下载器运行在你自己设备上，不包含任何云端服务。') }}</p>
        <ul>
          <li>{{ $t('下载链接、任务记录与视频文件只保存在你的 NAS 上，不会上传到任何第三方。') }}</li>
          <li>{{ $t('登录密码以 bcrypt 哈希形式存储，应用不保存明文密码。') }}</li>
          <li>{{ $t('应用不收集设备标识、位置、通讯录等个人信息，也不含统计与广告 SDK。') }}</li>
          <li>{{ $t('你可以随时在「设置」中删除任务与文件，卸载应用即删除全部数据。') }}</li>
        </ul>
        <p>
          {{ $t('完整条款请阅读') }}
          <a href="/privacy.html" target="_blank" rel="noopener">{{ $t('《隐私政策》') }}</a>
        </p>
      </div>
      <template #footer>
        <a-button @click="declineConsent">{{ $t('不同意') }}</a-button>
        <a-button type="primary" @click="acceptConsent">{{ $t('同意并继续') }}</a-button>
      </template>
    </a-modal>
</template>

<script setup>
import { ref, reactive, computed } from 'vue'
import { useRoute } from 'vue-router'
import { message } from 'ant-design-vue'
import zhCN from 'ant-design-vue/es/locale/zh_CN'
import enUS from 'ant-design-vue/es/locale/en_US'
import { t, apiError } from '../i18n'
import { useAppStore } from '../stores/app'
import { UserOutlined, LockOutlined } from '@ant-design/icons-vue'
import logo from '../assets/logo.png'
import { useUserStore } from '../stores/user'

const route = useRoute()
const userStore = useUserStore()
const appStore = useAppStore()
const antdLocale = computed(() => appStore.language === 'en-US' ? enUS : zhCN)

const loading = ref(false)
const formState = reactive({
  username: '',
  password: ''
})

// ---------- 隐私政策同意 ----------
// 合规要求：首次使用须明确告知并征得同意。用 localStorage 记住，
// 清理浏览器数据或卸载重装后会重新出现。
const CONSENT_KEY = 'privacy_consent_v1'
const showConsent = ref(false)
const agreed = ref(false)

try {
  agreed.value = localStorage.getItem(CONSENT_KEY) === '1'
} catch {
  agreed.value = false
}
showConsent.value = !agreed.value

function acceptConsent() {
  agreed.value = true
  showConsent.value = false
  try {
    localStorage.setItem(CONSENT_KEY, '1')
  } catch {
    // 隐私模式下 localStorage 可能不可写，本次会话内仍然有效
  }
}

function declineConsent() {
  showConsent.value = false
  agreed.value = false
  message.warning(t('未同意隐私政策，无法继续使用'))
}

// 登录卡片固定使用亮色主题，与深色背景形成对比
const cardTheme = {
  token: {
    colorPrimary: '#6366f1',
    colorInfo: '#6366f1',
    colorBgContainer: '#ffffff',
    colorBgElevated: '#ffffff',
    colorText: '#1c2333',
    colorTextSecondary: '#5f6b85',
    colorBorder: '#dfe3ec',
    borderRadius: 10,
    borderRadiusLG: 12
  }
}

const rules = computed(() => ({
  username: [{ required: true, message: t('请输入用户名') }],
  password: [{ required: true, message: t('请输入密码') }]
}))

// 只接受站内相对路径，避免 ?redirect=//evil.com 之类的开放重定向
function safeRedirect(value) {
  if (typeof value !== 'string') return '/'
  if (!value.startsWith('/') || value.startsWith('//')) return '/'
  return value
}

async function handleSubmit() {
  if (!agreed.value) {
    message.warning(t('请先阅读并同意《隐私政策》'))
    showConsent.value = true
    return
  }
  loading.value = true
  try {
    await userStore.login(formState.username, formState.password)
    message.success(t('登录成功'))
    // 登录后回到被拦截的页面（会话过期跳转时会带上 redirect）
    const redirect = safeRedirect(route.query.redirect)
    // 使用 window.location 强制跳转，而不是 router.push
    window.location.href = redirect
  } catch (err) {
    message.error(apiError(err, '登录失败'))
  } finally {
    loading.value = false
  }
}
</script>

.login-footer {
  margin-top: 18px;
  text-align: center;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.55);
}

.login-footer a {
  color: rgba(255, 255, 255, 0.7);
  text-decoration: none;
}

.login-footer a:hover {
  color: #fff;
  text-decoration: underline;
}

.login-footer .sep {
  margin: 0 8px;
  opacity: 0.5;
}

.consent-body p {
  color: #4a5164;
  margin: 0 0 10px;
  line-height: 1.7;
}

.consent-body ul {
  margin: 0 0 12px;
  padding-left: 20px;
  color: #4a5164;
  line-height: 1.7;
}

.consent-body li {
  margin: 4px 0;
}

<style scoped>
.login-page {
  position: relative;
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 100vh;
  overflow: hidden;
  background: #0b1020;
}

/* 渐变光斑 + 网格背景 */
.login-backdrop {
  position: absolute;
  inset: 0;
  background:
    radial-gradient(640px 420px at 18% 12%, rgba(99, 102, 241, 0.32), transparent 62%),
    radial-gradient(560px 420px at 84% 78%, rgba(139, 92, 246, 0.26), transparent 62%),
    radial-gradient(420px 320px at 70% 15%, rgba(56, 189, 248, 0.14), transparent 65%);
}

.login-backdrop::after {
  content: '';
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(rgba(255, 255, 255, 0.035) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 255, 255, 0.035) 1px, transparent 1px);
  background-size: 44px 44px;
  mask-image: radial-gradient(ellipse 70% 60% at 50% 45%, #000 30%, transparent 75%);
  -webkit-mask-image: radial-gradient(ellipse 70% 60% at 50% 45%, #000 30%, transparent 75%);
}

.login-main {
  position: relative;
  z-index: 1;
  width: 100%;
  max-width: 380px;
  padding: 24px;
  display: flex;
  flex-direction: column;
  align-items: center;
}

.login-brand {
  text-align: center;
  margin-bottom: 28px;
}

.brand-tile {
  width: 64px;
  height: 64px;
  margin: 0 auto 14px;
  border-radius: var(--radius-lg);
  display: block;
  object-fit: contain;
  box-shadow: 0 8px 24px rgba(99, 102, 241, 0.45);
}

.brand-name {
  font-size: 22px;
  font-weight: 700;
  color: #fff;
  letter-spacing: 0.3px;
}

.brand-sub {
  margin-top: 4px;
  font-size: 13px;
  color: rgba(255, 255, 255, 0.65);
}

.login-card {
  width: 100%;
  padding: 28px 26px;
  border-radius: var(--radius-lg);
  background: #fff;
  box-shadow: 0 24px 64px rgba(2, 6, 23, 0.55);
}

.login-language {
  width: 100%;
  text-align: right;
  margin-bottom: 10px;
}

.login-card-title {
  margin-bottom: 20px;
  font-size: 17px;
  font-weight: 700;
  color: #1c2333;
}

.field-icon {
  color: #98a2b8;
}

.login-button {
  font-weight: 600;
  background: linear-gradient(135deg, #6366f1 0%, #7c6cf8 100%);
  border: none;
  box-shadow: 0 6px 16px rgba(99, 102, 241, 0.35);
}

.login-button:hover {
  opacity: 0.92;
}

</style>
