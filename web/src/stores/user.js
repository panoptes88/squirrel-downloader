import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { get, post } from '../api'
import { buildEncryptedPayload } from '../utils/authcrypto'

export const useUserStore = defineStore('user', () => {
  const username = ref(localStorage.getItem('username') || '')
  const isLoggedIn = computed(() => !!username.value)

  async function checkLogin() {
    try {
      const res = await get('/user')
      // 只有当明确返回 username 时才认为已登录
      if (res && res.username) {
        username.value = res.username
        localStorage.setItem('username', res.username)
        return true
      }
      username.value = ''
      localStorage.removeItem('username')
      return false
    } catch (err) {
      // 401 未登录是正常情况，不做处理
      username.value = ''
      localStorage.removeItem('username')
      return false
    }
  }

  // 只走密文登录：局域网 HTTP 下密码不会以明文出现在请求体里。
  // 前端与 NAS 捆绑发布，不存在「前端新、服务端旧」的组合，所以不做明文回退。
  async function login(usernameVal, password) {
    const payload = await buildEncryptedPayload({ username: usernameVal, password })
    const res = await post('/auth/login-encrypted', payload)
    username.value = res.username
    localStorage.setItem('username', res.username)
    return res
  }

  function logout() {
    return post('/auth/logout').then(() => {
      username.value = ''
      localStorage.removeItem('username')
    })
  }

  return {
    username,
    isLoggedIn,
    checkLogin,
    login,
    logout
  }
})
