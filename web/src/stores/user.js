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

  // 优先走密文登录：局域网 HTTP 下密码不会以明文出现在请求体里。
  // 服务端未提供加密接口时（旧版本）回退到明文，保证升级过程不中断。
  async function login(usernameVal, password) {
    let res
    try {
      const payload = await buildEncryptedPayload({ username: usernameVal, password })
      res = await post('/auth/login-encrypted', payload)
    } catch (err) {
      if (!isMissingEncryptionEndpoint(err)) throw err
      res = await post('/auth/login', { username: usernameVal, password })
    }
    username.value = res.username
    localStorage.setItem('username', res.username)
    return res
  }

  // 404 / 405 表示服务端还没有密文接口，可以安全回退到明文登录。
  function isMissingEncryptionEndpoint(err) {
    const status = err?.response?.status
    return status === 404 || status === 405
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
