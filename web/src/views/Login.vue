<script setup lang="ts">
import { ref } from 'vue'
import { useAuthStore } from '../stores/auth'
import { useRoute, useRouter } from 'vue-router'
import { systemService } from '../services/system'
import type { PasswordCredentialStatus } from '../types/credentials'
import WorkspacePreviewCard from '../components/WorkspacePreviewCard.vue'
import { LOGIN_PREVIEW_DEMO } from '../utils/workspacePreview'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()
const form = ref({ username: '', password: '' })
const loading = ref(false)
const passwordChangeOpen = ref(false)
const changingPassword = ref(false)
const passwordChangeError = ref('')
const newPasswordInput = ref<{ focus: () => void } | null>(null)
const passwordChangeForm = ref({ old_password: '', new_password: '', confirm_password: '' })

async function handleLogin() {
  const { ElMessage } = await import('element-plus')
  if (!form.value.username || !form.value.password) {
    ElMessage.warning('请输入用户名和密码')
    return
  }

  loading.value = true
  const result = await auth.login(form.value.username, form.value.password)
  loading.value = false
  if (!result.ok) {
    ElMessage.error('登录失败，请检查凭证')
    return
  }

  if (await startPasswordRemediation(result.credential)) {
    return
  }
  await completeLogin('欢迎回来')
}

async function startPasswordRemediation(status: PasswordCredentialStatus): Promise<boolean> {
  if (!status.change_required) return false
  const { ElMessageBox } = await import('element-plus')
  if (status.management === 'environment') {
    const variable = status.environment_variable || 'PROXY_WEB_PASSWORD'
    await ElMessageBox.alert(
      `当前登录密码强度不足，并由环境变量 ${variable} 管理。控制台不会覆盖环境变量，请在部署环境中修改后重启 HiDeck。`,
      '请更换弱密码',
      { confirmButtonText: '我知道了', showClose: false, closeOnClickModal: false, closeOnPressEscape: false, type: 'warning' }
    )
    return false
  }
  passwordChangeForm.value = {
    old_password: form.value.password,
    new_password: '',
    confirm_password: ''
  }
  form.value.password = ''
  passwordChangeError.value = ''
  passwordChangeOpen.value = true
  return true
}

async function submitPasswordChange() {
  passwordChangeError.value = validatePasswordChange()
  if (passwordChangeError.value) return

  changingPassword.value = true
  const result = await systemService.changePassword(passwordChangeForm.value)
  changingPassword.value = false
  if (!result.ok) {
    passwordChangeError.value = result.error.message || '密码更新失败'
    return
  }
  auth.applyToken(result.data.token)
  await completeLogin('密码已更新')
}

function validatePasswordChange(): string {
  if (!passwordChangeForm.value.old_password || !passwordChangeForm.value.new_password) {
    return '请填写当前密码和新密码'
  }
  if (passwordChangeForm.value.new_password !== passwordChangeForm.value.confirm_password) {
    return '两次输入的新密码不一致'
  }
  return ''
}

async function completeLogin(message: string) {
  const { ElMessage } = await import('element-plus')
  passwordChangeOpen.value = false
  passwordChangeForm.value = { old_password: '', new_password: '', confirm_password: '' }
  passwordChangeError.value = ''
  form.value.password = ''
  ElMessage.success(message)
  await redirectAfterLogin()
}

async function redirectAfterLogin() {
  const queryRedirect = typeof route.query.redirect === 'string' ? route.query.redirect : ''
  let redirect = queryRedirect ? decodeURIComponent(queryRedirect) : ''
  if (!redirect) {
    try {
      redirect = sessionStorage.getItem('post_login_redirect') || ''
    } catch {
      // Storage can be unavailable in hardened browser contexts.
    }
  }
  if (redirect) {
    try {
      sessionStorage.removeItem('post_login_redirect')
    } catch {
      // Storage can be unavailable in hardened browser contexts.
    }
    await router.push(redirect)
    return
  }
  await router.push('/')
}
</script>

<template>
  <main class="login-landing">
    <header class="login-brand">
      <span class="login-brand-mark">H</span>
      <strong>HiDeck</strong>
    </header>

    <section class="login-hero">
      <h1>设备在线，直接进工作区。</h1>
      <p class="login-sub">通信模组控制台 · 管理射频、电话和 eSIM</p>

      <form class="login-form" @submit.prevent="handleLogin">
        <label for="login-username">账号</label>
        <input
          id="login-username"
          v-model.trim="form.username"
          class="login-input"
          type="text"
          name="username"
          autocomplete="username"
          placeholder="admin"
        />

        <label for="login-password">密码</label>
        <input
          id="login-password"
          v-model="form.password"
          class="login-input"
          type="password"
          name="password"
          autocomplete="current-password"
          placeholder=""
        />

        <button type="submit" class="login-submit" :disabled="loading">
          {{ loading ? '正在验证' : '登录' }}
        </button>
      </form>

      <p class="login-footnote">弱密码登录后必须修改</p>
    </section>

    <section class="login-preview-wrap" aria-label="工作区预览">
      <WorkspacePreviewCard :model="LOGIN_PREVIEW_DEMO" />
    </section>

    <el-dialog
      v-model="passwordChangeOpen"
      title="修改登录密码"
      width="min(440px, calc(100vw - 32px))"
      :show-close="false"
      :close-on-click-modal="false"
      :close-on-press-escape="false"
      @opened="newPasswordInput?.focus()"
    >
      <form class="password-change-form" @submit.prevent="submitPasswordChange">
        <p class="password-change-notice">当前密码仍是初始明文凭证或强度不足。建议立即修改。</p>

        <div class="space-y-1">
          <label for="login-current-password">当前密码</label>
          <el-input
            id="login-current-password"
            v-model="passwordChangeForm.old_password"
            type="password"
            show-password
            autocomplete="current-password"
          />
        </div>
        <div class="space-y-1">
          <label for="login-new-password">新密码</label>
          <el-input
            id="login-new-password"
            ref="newPasswordInput"
            v-model="passwordChangeForm.new_password"
            type="password"
            show-password
            autocomplete="new-password"
            placeholder="至少 8 位，建议 12 位以上"
          />
        </div>
        <div class="space-y-1">
          <label for="login-confirm-password">确认新密码</label>
          <el-input
            id="login-confirm-password"
            v-model="passwordChangeForm.confirm_password"
            type="password"
            show-password
            autocomplete="new-password"
          />
        </div>

        <p v-if="passwordChangeError" class="password-change-error" role="alert">
          {{ passwordChangeError }}
        </p>
        <div class="password-change-actions">
          <el-button native-type="button" :disabled="changingPassword" @click="completeLogin('欢迎回来')">
            稍后处理
          </el-button>
          <el-button type="primary" native-type="submit" :loading="changingPassword">
            更新密码并进入
          </el-button>
        </div>
      </form>
    </el-dialog>
  </main>
</template>

<style scoped>
.login-landing {
  position: relative;
  min-height: 100%;
  padding: 28px 28px 48px;
  overflow: hidden;
  background: var(--ui-bg);
  color: var(--ui-text);
}

.login-landing::before,
.login-landing::after {
  position: absolute;
  content: "";
  pointer-events: none;
  z-index: 0;
}

.login-landing::before {
  left: -8%;
  bottom: 4%;
  width: 42%;
  height: 38%;
  background: radial-gradient(circle, color-mix(in srgb, #E8B4C8 36%, transparent), transparent 72%);
}

.login-landing::after {
  right: -6%;
  bottom: 0;
  width: 40%;
  height: 36%;
  background: radial-gradient(circle, color-mix(in srgb, #C4B5E8 32%, transparent), transparent 74%);
}

.login-brand,
.login-hero,
.login-preview-wrap {
  position: relative;
  z-index: 1;
}

.login-brand {
  display: flex;
  align-items: center;
  gap: 10px;
  color: var(--ui-text);
}

.login-brand-mark {
  width: 34px;
  height: 34px;
  display: grid;
  place-items: center;
  border-radius: 10px;
  background: var(--ui-accent);
  color: #fff;
  font-size: 16px;
  font-weight: 700;
}

.login-brand strong {
  font-size: 20px;
  font-weight: 700;
}

.login-hero {
  width: min(420px, 100%);
  margin: 56px auto 0;
  text-align: center;
}

.login-hero h1 {
  margin: 0;
  color: var(--ui-text);
  font-size: 32px;
  font-weight: 700;
  line-height: 1.25;
}

.login-sub {
  margin: 12px 0 0;
  color: var(--ui-muted);
  font-size: 14px;
  line-height: 1.5;
}

.login-form {
  margin-top: 28px;
  display: grid;
  gap: 8px;
  text-align: left;
}

.login-form label {
  margin-top: 8px;
  color: var(--ui-text);
  font-size: 13px;
  font-weight: 650;
}

.login-input {
  height: 50px;
  padding: 0 20px;
  border: 1px solid var(--ui-border);
  border-radius: var(--ui-radius-pill);
  background: var(--ui-surface);
  color: var(--ui-text);
  outline: 0;
}

.login-input:focus {
  border-color: var(--ui-accent);
  box-shadow: var(--ui-focus);
}

.login-submit {
  height: 50px;
  margin-top: 16px;
  border: 0;
  border-radius: var(--ui-radius-pill);
  background: var(--ui-primary-solid);
  color: #fff;
  font-size: 16px;
  font-weight: 700;
  cursor: pointer;
}

.login-submit:hover:not(:disabled) {
  background: var(--ui-primary-hover);
}

.login-submit:disabled {
  opacity: .58;
  cursor: not-allowed;
}

.login-footnote {
  margin: 14px 0 0;
  color: var(--ui-muted);
  font-size: 12px;
  text-align: center;
}

.login-preview-wrap {
  width: min(760px, 100%);
  margin: 48px auto 0;
}

.password-change-form {
  display: grid;
  gap: 14px;
}

.password-change-notice {
  margin: 0 0 2px;
  color: var(--ui-text-muted);
  font-size: 13px;
  line-height: 1.6;
}

.password-change-error {
  margin: 0;
  color: var(--ui-danger);
  font-size: 13px;
  line-height: 1.5;
}

.password-change-actions {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1.4fr);
  gap: 10px;
  margin-top: 4px;
}

.password-change-actions :deep(.el-button) {
  width: 100%;
  height: 44px;
  margin: 0;
  border-radius: 8px;
}

.password-change-actions :deep(.el-button:not(.el-button--primary)) {
  border: 1px solid var(--ui-border);
  background: var(--ui-surface);
  color: var(--ui-text);
}

@media (max-width: 640px) {
  .login-landing {
    padding: 20px 16px 36px;
  }

  .login-hero {
    margin-top: 40px;
  }

  .login-hero h1 {
    font-size: 26px;
  }

  .login-preview-wrap {
    margin-top: 32px;
  }
}
</style>
