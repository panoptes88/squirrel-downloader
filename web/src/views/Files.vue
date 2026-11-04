<template>
  <div class="files-page">
    <PageHeader :title="$t('文件管理')" :subtitle="$t('浏览、播放与下载已完成的文件')">
      <a-select
        v-model:value="typeFilter"
        class="file-type-select"
        @change="selectedRowKeys = []"
      >
        <a-select-option value="">{{ $t('全部类型') }}</a-select-option>
        <a-select-option value="video">{{ $t('视频') }}</a-select-option>
        <a-select-option value="subtitle">{{ $t('字幕') }}</a-select-option>
        <a-select-option value="other">{{ $t('其他') }}</a-select-option>
      </a-select>
      <a-input
        v-model:value="searchKeyword"
        class="file-search-input"
        :placeholder="$t('搜索文件名')"
        allow-clear
      />
      <!-- 手机端不显示刷新：一行放不下会把按钮裁掉，用户下拉/重开页面即可 -->
      <a-button v-if="!isMobile" @click="fetchFiles">
        <template #icon><ReloadOutlined /></template>
        {{ $t('刷新') }}
      </a-button>
    </PageHeader>

    <a-card class="app-card files-card">
      <div class="files-summary">
        <span>{{ $t('共 {count} 个文件', { count: filteredFiles.length }) }}</span>
        <span class="summary-sep">·</span>
        <span>{{ $t('合计 {size}', { size: formatSize(totalSize) }) }}</span>
        <span v-if="filteredFiles.length !== files.length" class="summary-sep">
          {{ $t('（已从 {count} 个中筛选）', { count: files.length }) }}
        </span>
      </div>

      <transition name="fade">
        <div class="batch-bar" v-if="selectedRowKeys.length > 0">
          <span class="batch-info">{{ $t('已选择 {count} 个文件', { count: selectedRowKeys.length }) }}</span>
          <a-space :size="8">
            <a-button size="small" @click="selectedRowKeys = []">{{ $t('取消选择') }}</a-button>
            <a-popconfirm
              :title="$t('确定删除选中的文件？')"
              @confirm="batchDelete"
            >
              <a-button type="primary" danger size="small">{{ $t('批量删除') }}</a-button>
            </a-popconfirm>
          </a-space>
        </div>
      </transition>

      <!-- 移动端：表格在窄屏要横向滚动，「操作」列会被挤出屏幕，改用两行卡片 -->
      <div v-if="isMobile" class="file-cards">
        <a-empty v-if="filteredFiles.length === 0" :description="$t('没有符合条件的文件')" />
        <div
          v-for="record in pagedFiles"
          :key="record.name"
          class="file-card"
          @click="record.kind === 'video' ? playVideo(record) : copyToClipboard(record.name)"
        >
          <div class="file-card-main">
            <a-checkbox
              :checked="selectedRowKeys.includes(record.name)"
              :aria-label="$t('选择 {name}', { name: record.name })"
              @click.stop
              @change="e => toggleFile(record.name, e.target.checked)"
            />
            <img
              v-if="record.kind === 'video'"
              class="file-thumb"
              :src="thumbUrl(record)"
              alt=""
              loading="lazy"
              @error="$event.target.style.display = 'none'"
            />
            <FileOutlined v-else class="file-icon" aria-hidden="true" />
            <div class="file-card-text">
              <span class="file-card-name" :title="record.name">{{ record.name }}</span>
              <span class="file-card-meta">
                {{ record.modTime }}<span v-if="record.size"> · {{ formatSize(record.size) }}</span>
              </span>
            </div>
            <a-dropdown :trigger="['click']" placement="bottomRight">
              <a-button type="text" size="small" class="file-card-more" @click.stop>
                <template #icon><MoreOutlined /></template>
              </a-button>
              <template #overlay>
                <a-menu>
                  <a-menu-item @click="downloadFile(record.name)">{{ $t('下载') }}</a-menu-item>
                  <a-menu-item danger @click="confirmDeleteFile(record.name)">{{ $t('删除') }}</a-menu-item>
                </a-menu>
              </template>
            </a-dropdown>
          </div>
        </div>
        <a-pagination
          v-if="filteredFiles.length > pageSize"
          v-model:current="cardPage"
          :page-size="pageSize"
          :total="filteredFiles.length"
          size="small"
          simple
          class="file-cards-pagination"
        />
      </div>

      <a-table
        v-else
        :columns="columns"
        :data-source="filteredFiles"
        :pagination="paginationConfig"
        :loading="loading"
        :row-selection="rowSelection"
        :row-key="record => record.name"
        :scroll="{ x: 900 }"
        @change="handleTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'name'">
            <span class="file-cell">
              <span
                class="file-thumb-wrap"
                :class="{ 'is-video': record.kind === 'video' }"
                @click="record.kind === 'video' ? playVideo(record) : null"
              >
                <img
                  v-if="record.kind === 'video'"
                  class="file-thumb"
                  :src="thumbUrl(record)"
                  alt=""
                  loading="lazy"
                  @error="$event.target.style.display = 'none'"
                />
                <FileOutlined v-else class="file-icon" aria-hidden="true" />
                <span v-if="record.kind === 'video'" class="file-thumb-play" aria-hidden="true">
                  <PlayCircleOutlined />
                </span>
              </span>
              <a-tooltip :title="record.name">
                <span class="file-name copyable-text" @click="copyToClipboard(record.name)">{{ record.name }}</span>
              </a-tooltip>
            </span>
          </template>

          <template v-if="column.key === 'size'">
            {{ formatSize(record.size) }}
          </template>
          <template v-if="column.key === 'ext'">
            {{ typeLabel(record) }}
          </template>
          <template v-if="column.key === 'accessTime'">
            {{ record.accessTime || '-' }}
          </template>
          <template v-if="column.key === 'createTime'">
            {{ record.createTime || '-' }}
          </template>
          <template v-if="column.key === 'modTime'">
            {{ record.modTime }}
          </template>
          <template v-if="column.key === 'action'">
            <a-space :size="4">
              <a-button size="small" type="text" @click="downloadFile(record.name)">
                {{ $t('下载') }}
              </a-button>
              <a-popconfirm
                :title="$t('确定删除此文件？')"
                @confirm="deleteFile(record.name)"
              >
                <a-button size="small" type="text" danger>{{ $t('删除') }}</a-button>
              </a-popconfirm>
            </a-space>
          </template>
        </template>
      </a-table>
    </a-card>

    <!-- 视频播放弹窗 -->
    <a-modal
      v-model:open="videoModalVisible"
      :title="currentVideoName"
      :footer="null"
      @cancel="closeVideo"
      class="video-modal"
      :width="800"
    >
      <div class="video-wrapper">
        <div id="xgplayer-container" class="xgplayer-container"></div>
      </div>
    </a-modal>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, computed, nextTick, watch } from 'vue'
import { message, Modal } from 'ant-design-vue'
import { t, apiError, locale } from '../i18n'
import { ReloadOutlined, PlayCircleOutlined, FileOutlined, MoreOutlined } from '@ant-design/icons-vue'
import { get, del } from '../api'
import { copyToClipboard } from '../utils/clipboard'
import { useIsMobile } from '../composables/useIsMobile'
import Player from 'xgplayer'
import PageHeader from '../components/PageHeader.vue'

const loading = ref(false)
const files = ref([])
const searchKeyword = ref('')
const typeFilter = ref('')
const videoModalVisible = ref(false)
const currentVideoName = ref('')
const currentVideoUrl = ref('')
const selectedRowKeys = ref([])
const pageSize = ref(10)

// 窄屏精简工具栏：刷新按钮在手机上一行放不下
const { isMobile } = useIsMobile()

// 移动端卡片列表的分页（表格有自己的分页，两者互不影响）
const cardPage = ref(1)
const pagedFiles = computed(() => {
  const start = (cardPage.value - 1) * pageSize.value
  return filteredFiles.value.slice(start, start + pageSize.value)
})

// 卡片上的勾选框：与表格共用 selectedRowKeys，批量删除逻辑不变
function toggleFile(name, checked) {
  const next = new Set(selectedRowKeys.value)
  if (checked) next.add(name)
  else next.delete(name)
  selectedRowKeys.value = [...next]
}

// 切换筛选/搜索时回到第一页，避免停在越界的页
watch([searchKeyword, typeFilter], () => {
  cardPage.value = 1
})

// 文件变少（删除/筛选）时把越界页码收回来
watch(
  () => filteredFiles.value.length,
  len => {
    const maxPage = Math.max(1, Math.ceil(len / pageSize.value))
    if (cardPage.value > maxPage) cardPage.value = maxPage
  }
)

const subtitleExtensions = ['.srt', '.vtt', '.ass', '.ssa', '.sub']

function fileKind(name) {
  const dot = name.lastIndexOf('.')
  const ext = dot === -1 ? '' : name.substring(dot).toLowerCase()
  if (videoExtensions.includes(ext)) return 'video'
  if (subtitleExtensions.includes(ext)) return 'subtitle'
  return 'other'
}

// 按类型 + 文件名关键字过滤
const filteredFiles = computed(() => {
  const kw = searchKeyword.value.trim().toLowerCase()
  const kind = typeFilter.value
  return files.value.filter(f => {
    if (kind && fileKind(f.name) !== kind) return false
    if (kw && !f.name.toLowerCase().includes(kw)) return false
    return true
  })
})

// 当前筛选结果的总大小
const totalSize = computed(() =>
  filteredFiles.value.reduce((sum, f) => sum + (f.size || 0), 0)
)

// 西瓜播放器实例
let xgPlayer = null
watch(locale, value => {
  if (xgPlayer) xgPlayer.lang = value === 'en-US' ? 'en' : 'zh-cn'
})

const videoExtensions = ['.mp4', '.mkv', '.avi', '.mov', '.webm', '.flv', '.wmv', '.m4v', '.3gp']

const paginationConfig = computed(() => ({
  pageSize: pageSize.value,
  showSizeChanger: true,
  pageSizeOptions: ['10', '20', '50', '100', '200'],
  showTotal: (total) => t('共 {count} 个文件', { count: total })
}))

// 按中文习惯排序文件名（默认 localeCompare 对中文名的顺序不稳定）
const sortByName = (a, b) => a.name.localeCompare(b.name, 'zh-Hans-CN')

const columns = computed(() => [
  {
    title: t('文件名'),
    dataIndex: 'name',
    key: 'name',
    ellipsis: true,
    sorter: sortByName,
    // 320 而不是 150：单元格里还有缩略图和播放按钮，150 会把文件名压成几个字
    width: 340
  },
  {
    title: t('大小'),
    dataIndex: 'size',
    key: 'size',
    width: 90,
  },
  {
    title: t('类型'),
    dataIndex: 'ext',
    key: 'ext',
    width: 80,
  },
  {
    title: t('最近访问'),
    dataIndex: 'accessTime',
    key: 'accessTime',
    width: 150,
    sorter: (a, b) => a.size - b.size
  },
  {
    title: t('修改时间'),
    dataIndex: 'modTime',
    key: 'modTime',
    width: 150,
  },
  {
    title: t('创建日期'),
    dataIndex: 'createTime',
    key: 'createTime',
    width: 150,
    sorter: (a, b) => new Date(a.modTime) - new Date(b.modTime),
    responsive: ['lg']
  },
  { title: t('操作'), key: 'action', width: 110 }
])

const rowSelection = computed(() => ({
  selectedRowKeys: selectedRowKeys.value,
  onChange: (keys) => {
    selectedRowKeys.value = keys
  }
}))

// 后端返回的 ext 已是大写（MP4 / SRT），没有就从前缀推断
function typeLabel(record) {
  if (record.ext) return record.ext
  const dot = record.name.lastIndexOf('.')
  return dot === -1 ? '-' : record.name.substring(dot + 1).toUpperCase()
}

// 视频缩略图：后端用系统 ffmpeg 抽帧并缓存
function thumbUrl(record) {
  return `/api/files/thumbnail?name=${encodeURIComponent(record.name)}`
}

function formatSize(bytes) {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

function handleTableChange(paginationInfo) {
  pageSize.value = paginationInfo.pageSize
}

function isVideoFile(filename) {
  const ext = filename.substring(filename.lastIndexOf('.')).toLowerCase()
  return videoExtensions.includes(ext)
}

async function fetchFiles() {
  loading.value = true
  try {
    files.value = await get('/files')
    selectedRowKeys.value = []
  } catch (err) {
    message.error(t('获取文件列表失败'))
  } finally {
    loading.value = false
  }
}

function downloadFile(name) {
  // 不用 window.open：手机上新标签页会先弹出，下载开始后又自动关闭，
  // 表现为屏幕「跳一下」。改用 a[download] 触发同源下载，不切换页面。
  // 服务端本身也带 Content-Disposition: attachment，两边都保证是下载而不是跳转。
  const a = document.createElement('a')
  a.href = `/api/files/download?name=${encodeURIComponent(name)}`
  a.download = name
  a.rel = 'noopener'
  a.style.display = 'none'
  document.body.appendChild(a)
  a.click()
  a.remove()
}

async function playVideo(record) {
  currentVideoName.value = record.name
  currentVideoUrl.value = `/api/files/download?name=${encodeURIComponent(record.name)}`
  videoModalVisible.value = true

  // 修改页面标题为文件名（用于 QQ 浏览器嗅探）
  document.title = record.name

  // 等待 DOM 更新后初始化播放器
  await nextTick()
  initXgPlayer(currentVideoUrl.value, record.name)
  // 尝试加载同名字幕
  loadSubtitle(record.name)
}

// SRT 字幕转 VTT
function srtToVtt(srt) {
  return 'WEBVTT\n\n' + srt
    .replace(/\r\n/g, '\n')
    .replace(/\r/g, '\n')
    .replace(/(\d{2}:\d{2}:\d{2}),(\d{3})/g, '$1.$2')
}

// 加载同名字幕（.srt/.vtt），用 HTML5 track 原生渲染
async function loadSubtitle(videoName) {
  const baseName = videoName.replace(/\.[^.]+$/, '')
  // 匹配同名字幕：video.mp4 -> video.srt / video.en.srt / video.zh.srt 等
  const subFiles = files.value.filter(f =>
    f.name.startsWith(baseName + '.') && /\.(srt|vtt)$/i.test(f.name)
  )
  // 优先中/英文字幕，否则取第一个
  const subFile = subFiles.find(f => /\.(zh|en)\./i.test(f.name)) || subFiles[0]
  if (!subFile) return

  try {
    const subUrl = `/api/files/download?name=${encodeURIComponent(subFile.name)}`
    const res = await fetch(subUrl)
    let text = await res.text()
    if (/\.srt$/i.test(subFile.name)) {
      text = srtToVtt(text)
    }
    // 用 HTML5 <track> 加载字幕，浏览器原生渲染
    const blob = new Blob([text], { type: 'text/vtt' })
    const vttUrl = URL.createObjectURL(blob)
    const video = document.querySelector('#xgplayer-container video')
    if (!video) {
      console.warn('[字幕] 未找到 video 元素')
      return
    }
    // 清除旧 track
    video.querySelectorAll('track').forEach(t => t.remove())
    const track = document.createElement('track')
    track.kind = 'subtitles'
    track.label = t('字幕')
    track.srclang = 'zh'
    track.src = vttUrl
    track.default = true
    video.appendChild(track)
    // 字幕加载后开启显示
    track.addEventListener('load', () => {
      if (video.textTracks && video.textTracks.length > 0) {
        video.textTracks[0].mode = 'showing'
      }
    })
  } catch (err) {
    console.error('字幕加载失败:', err)
    message.warning(t('字幕加载失败，请查看控制台'))
  }
}

function initXgPlayer(url, title = '') {
  // 如果已存在播放器，先销毁
  if (xgPlayer) {
    xgPlayer.destroy()
    xgPlayer = null
  }

  const savedVolume = parseFloat(localStorage.getItem('xg_volume') || '1')
  xgPlayer = new Player({
    id: 'xgplayer-container',
    lang: locale.value === 'en-US' ? 'en' : 'zh-cn',
    url: url,
    playbackRate: [0.5, 0.75, 1, 1.25, 1.5, 2],
    defaultPlaybackRate: 1,
    fluid: true,
    volume: savedVolume,
    maxVolume: 1,
    ignores: ['quality'],
    closeVideoClick: false,
    title: title,
    error: () => {
      message.error(t('视频加载失败，请检查文件是否完整'))
    }
  })

  // 音量记忆：变化时保存
  xgPlayer.on('volumechange', () => {
    localStorage.setItem('xg_volume', String(xgPlayer.volume))
  })

  // 播放进度记忆：恢复上次播放位置
  const progressKey = `xg_progress_${title}`
  const savedTime = parseFloat(localStorage.getItem(progressKey) || '0')
  if (savedTime > 0) {
    xgPlayer.once('canplay', () => {
      xgPlayer.currentTime = savedTime
    })
  }
  xgPlayer.on('timeupdate', () => {
    localStorage.setItem(progressKey, String(xgPlayer.currentTime))
  })
  xgPlayer.on('ended', () => {
    localStorage.removeItem(progressKey)
  })
}

function closeVideo() {
  if (xgPlayer) {
    // 关闭前保存最后进度
    if (currentVideoName.value && xgPlayer.currentTime > 0) {
      localStorage.setItem(`xg_progress_${currentVideoName.value}`, String(xgPlayer.currentTime))
    }
    xgPlayer.destroy()
    xgPlayer = null
  }
  // 恢复页面标题
  document.title = t('松鼠下载器')
  currentVideoName.value = ''
  currentVideoUrl.value = ''
}

// 组件销毁时清理播放器
onUnmounted(() => {
  if (xgPlayer) {
    xgPlayer.destroy()
    xgPlayer = null
  }
})

// 手机端用居中的 Modal.confirm，而不是 popconfirm：
// antd 的 popconfirm 是贴在被点元素旁的浮层，在 390px 屏上会顶到屏幕边缘，
// 而且带 zoom-big-fast 缩放动画（宽度从 ~143px 长到 158px），看起来像"拉伸"。
// 任务页删任务也是用 Modal.confirm，保持一致。
function confirmDeleteFile(name) {
  Modal.confirm({
    title: t('确定删除此文件？'),
    content: t('「{name}」将被删除，此操作不可恢复。', { name }),
    okText: t('删除'),
    okType: 'danger',
    cancelText: t('取消'),
    onOk: () => deleteFile(name)
  })
}

async function deleteFile(name) {
  try {
    await del(`/files/${encodeURIComponent(name)}`)
    message.success(t('删除成功'))
    fetchFiles()
  } catch (err) {
    message.error(apiError(err, '删除失败'))
  }
}

async function batchDelete() {
  // 后端 ListFiles 只返回文件（目录已被跳过），选中的必然都是文件
  const names = selectedRowKeys.value
  if (names.length === 0) {
    return
  }

  try {
    // 单次批量请求，替代此前 N 次串行 DELETE
    const res = await del('/files', { names })
    const success = res?.deleted?.length || 0
    const failed = res?.failed?.length || 0

    if (success > 0) {
      message.success(t('成功删除 {count} 个文件', { count: success }))
    }
    if (failed > 0) {
      message.warning(t('删除失败 {count} 个文件', { count: failed }))
    }

    selectedRowKeys.value = []
    fetchFiles()
  } catch (err) {
    message.error(apiError(err, '批量删除失败'))
  }
}

onMounted(() => {
  fetchFiles()
})
</script>

<style scoped>
.files-page {
  display: flex;
  flex-direction: column;
}

.files-card :deep(.ant-card-body) {
  padding-top: 12px;
}

/* 移动端两行文件卡片 */
/* 工具栏里的类型筛选与搜索框：窄屏下允许搜索框收缩，避免英文文案把一行撑爆 */
.file-type-select {
  width: 120px;
  flex: 0 0 auto;
}

.file-search-input {
  width: 200px;
  flex: 1 1 160px;
  min-width: 0;
}

.file-cards {
  display: flex;
  flex-direction: column;
}

.file-card {
  padding: 10px 4px;
  border-radius: var(--radius-sm);
  cursor: pointer;
}

.file-card:active {
  background: var(--border-soft);
}

/* 单行：勾选 / 缩略图 / 文件名+信息 / 更多 */
.file-card-main {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

/* 缩略图外层：悬停时浮出播放按钮 */
.file-thumb-wrap {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: none;
  line-height: 0;
}

.file-thumb-wrap.is-video {
  cursor: pointer;
}

.file-thumb-play {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 3px;
  background: rgba(0, 0, 0, 0.45);
  color: #fff;
  font-size: 16px;
  opacity: 0;
  transition: opacity 0.15s;
}

.file-thumb-wrap.is-video:hover .file-thumb-play {
  opacity: 1;
}

/* 视频缩略图：桌面表格与手机列表共用 */
.file-thumb {
  width: 56px;
  height: 32px;
  flex: none;
  object-fit: cover;
  border-radius: 3px;
  background: var(--border-soft);
}

.file-card-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1;
  min-width: 0;
}

/* 文件名最多两行，超出省略（与绿联文件管理器一致） */
.file-card-name {
  font-size: 15px;
  line-height: 1.35;
  color: var(--text);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  word-break: break-all;
}

.file-card-meta {
  font-size: 12px;
  color: var(--text-sub);
}

.file-card-more {
  flex: none;
  color: var(--text-sub);
}

.file-cards-pagination {
  align-self: center;
  margin-top: 4px;
}

.files-summary {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  margin-bottom: 12px;
  font-size: 13px;
  color: var(--text-2);
}

.files-summary b {
  color: var(--text-1);
  font-variant-numeric: tabular-nums;
}

.summary-sep {
  color: var(--text-3);
}

.batch-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 14px;
  margin-bottom: 12px;
  border-radius: var(--radius-sm);
  background: rgba(99, 102, 241, 0.08);
  border: 1px solid rgba(99, 102, 241, 0.2);
}

.batch-info {
  font-size: 13px;
  color: var(--text-1);
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

.file-cell {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 100%;
  min-width: 0;
}

.play-button {
  flex: none;
  color: var(--brand);
  font-size: 17px;
}

.play-button:hover {
  color: var(--brand-strong) !important;
  background: rgba(99, 102, 241, 0.08) !important;
}

.file-icon {
  flex: none;
  color: var(--text-3);
  font-size: 16px;
}

.file-name {
  color: var(--text-1);
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.copyable-text {
  cursor: pointer;
  display: inline-block;
  max-width: 100%;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  vertical-align: bottom;
}

.copyable-text:hover {
  color: var(--brand);
}

.video-wrapper {
  position: relative;
}

.xgplayer-container {
  width: 100%;
  max-height: 450px;
}

.video-modal :deep(.ant-modal-body) {
  padding: 8px;
}

@media (max-width: 576px) {
  .files-page :deep(.ant-table-cell) {
    padding: 8px !important;
  }
}
</style>
