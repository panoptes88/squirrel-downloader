#!/usr/bin/env node
/**
 * 侧边栏对齐回归检查。
 *
 * 为什么需要它：收起态菜单图标的居中依赖 antd 的内边距推算，
 * 而我们在 theme.js 里自定义了 Menu.itemMarginInline。两者一旦有一方变化
 * （antd 升级、改主题 token、改 collapsedWidth），图标就会悄悄偏掉几像素——
 * 肉眼很难发现，但截图放大后很明显。这个脚本用真实浏览器量 DOM，直接给出结论。
 *
 * 断言：收起态下，顶部 logo 与每个菜单图标的水平中心，都应与侧边栏内容区中心一致。
 *
 * 用法：
 *   1) 先起一个容器（或本地 dev server）：
 *        docker run -d --name sq-ui -p 21810:8080 \
 *          -e ADMIN_PASSWORD=UiCheck12345 -e ALLOW_INSECURE=true \
 *          panoptes88/squirrel-downloader:0.1.0
 *   2) 运行（需要能 require 到 playwright）：
 *        npm i --no-save playwright
 *        BASE_URL=http://127.0.0.1:21810 APP_PASSWORD=UiCheck12345 \
 *          node scripts/check-sidebar-alignment.js
 *
 * 退出码：0 = 全部对齐；1 = 有偏差（会打印具体像素数）。
 */

const { chromium } = require('playwright')

const BASE = process.env.BASE_URL || 'http://127.0.0.1:21810'
const PASS = process.env.APP_PASSWORD || 'admin123'
const TOLERANCE = 1.0 // px

async function login(page) {
  await page.goto(BASE, { waitUntil: 'networkidle' })
  await page.waitForTimeout(1200)
  if (await page.locator('input[type=password]').count()) {
    await page.locator('input').first().fill('admin')
    await page.locator('input[type=password]').first().fill(PASS)
    await page.locator('input[type=password]').first().press('Enter')
    await page.waitForTimeout(3000)
  }
  await page.waitForSelector('.pc-sider .logo-img', { timeout: 15000 })
}

async function measure(page) {
  return page.evaluate(() => {
    const sider = document.querySelector('.pc-sider')
    const img = document.querySelector('.pc-sider .logo-img')
    if (!sider || !img) return null
    const cx = el => {
      const b = el.getBoundingClientRect()
      return (b.left + b.right) / 2
    }
    // 用内容区中心作基准：右边框 1px 不该参与居中计算
    const contentCenter = sider.getBoundingClientRect().left + sider.clientWidth / 2
    const icons = [...document.querySelectorAll('.pc-sider .ant-menu-item')].map(it => {
      const ic = it.querySelector('.ant-menu-item-icon, .anticon')
      return { label: (it.innerText || '').trim(), cx: ic ? cx(ic) : null }
    })
    return { contentCenter, logoCx: cx(img), icons }
  })
}

;(async () => {
  const browser = await chromium.launch()
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } })
  await login(page)

  let failed = 0

  // 只有收起态才有「居中」这回事：展开态是左对齐布局（图标 + 文字），
  // 对展开态只做信息性输出，不断言居中。
  for (const [state, collapsed] of [['展开态', false], ['收起态', true]]) {
    if (collapsed) {
      await page.locator('.pc-trigger').click()
      await page.waitForTimeout(1500)
    }
    const m = await measure(page)
    if (!m) { console.error(`❌ ${state}: 找不到侧边栏元素`); failed++; continue }

    const rows = [{ label: 'logo', cx: m.logoCx }, ...m.icons.filter(i => i.cx !== null)]
    console.log(`\n== ${state} ==  内容区中心 = ${m.contentCenter.toFixed(2)}`)
    for (const r of rows) {
      const d = r.cx - m.contentCenter
      const ok = Math.abs(d) < TOLERANCE
      if (collapsed && !ok) failed++
      console.log(
        `   ${r.label.padEnd(12)} 中心=${r.cx.toFixed(2).padStart(7)}  偏离=${d >= 0 ? '+' : ''}${d.toFixed(2)}px  ` +
        (collapsed ? (ok ? '✅' : '❌') : '(展开态左对齐，仅供参考)')
      )
    }
  }

  await browser.close()
  console.log(failed === 0 ? '\n✅ 收起态侧边栏对齐检查通过' : `\n❌ 有 ${failed} 处未对齐`)
  process.exit(failed === 0 ? 0 : 1)
})().catch(e => { console.error('FAILED:', e.message); process.exit(1) })
