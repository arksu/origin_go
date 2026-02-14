<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/authStore'
import { register } from '@/api/auth'
import { login as apiLogin } from '@/api/auth'
import { ApiException } from '@/api/errors'
import AppInput from '@/components/ui/AppInput.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppAlert from '@/components/ui/AppAlert.vue'

const router = useRouter()
const authStore = useAuthStore()

const form = ref({
  login: '',
  password: '',
  confirmPassword: '',
})

const loading = ref(false)
const error = ref('')
const fieldErrors = ref<Record<string, string>>({})

const isFormValid = computed(() => {
  return (
    form.value.login.trim().length >= 3 &&
    form.value.password.length >= 4 &&
    form.value.password === form.value.confirmPassword
  )
})

const passwordMismatch = computed(() => {
  return (
    form.value.confirmPassword.length > 0 &&
    form.value.password !== form.value.confirmPassword
  )
})

async function handleSubmit() {
  if (!isFormValid.value || loading.value) return

  loading.value = true
  error.value = ''
  fieldErrors.value = {}

  try {
    await register({
      login: form.value.login.trim(),
      password: form.value.password,
    })

    const loginResponse = await apiLogin({
      login: form.value.login.trim(),
      password: form.value.password,
    })

    authStore.setToken(loginResponse.token)
    router.push('/characters')
  } catch (e) {
    if (ApiException.isValidation(e)) {
      error.value = e.message
      if (e.details) {
        for (const [field, messages] of Object.entries(e.details)) {
          fieldErrors.value[field] = messages[0] || ''
        }
      }
    } else if (ApiException.isNetwork(e)) {
      error.value = 'No connection to server'
    } else if (e instanceof ApiException) {
      error.value = e.message || 'Registration failed'
    } else {
      error.value = 'An error occurred. Please try again later.'
    }
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="padding-all">
    <div class="form-container">
      <div class="logo-container">
        <img src="/assets/img/origin_logo3.webp" alt="logo">
      </div>

      <div class="login-panel">
        <AppAlert v-if="error" type="error" class="login-panel__alert">
          {{ error }}
        </AppAlert>

        <form @submit.prevent="handleSubmit">
          <AppInput
            v-model="form.login"
            placeholder="Login"
            :error="fieldErrors.login"
            :disabled="loading"
            autocomplete="username"
            autofocus
          />

          <AppInput
            v-model="form.password"
            type="password"
            placeholder="Password"
            :error="fieldErrors.password"
            :disabled="loading"
            autocomplete="new-password"
          />

          <AppInput
            v-model="form.confirmPassword"
            type="password"
            placeholder="Confirm password"
            :error="passwordMismatch ? 'Passwords do not match' : ''"
            :disabled="loading"
            autocomplete="new-password"
          />

          <AppButton
            type="submit"
            :loading="loading"
            :disabled="!isFormValid"
            class="login-panel__submit"
          >
            signup
          </AppButton>
        </form>

        <div class="signup-link">
          Already a member?
          <RouterLink to="/login">Login</RouterLink>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.padding-all {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100vh;
}

.form-container {
  margin-top: -12%;
  width: 36%;
  max-width: 600px;
}

.logo-container {
  line-height: 55px;
  text-align: center;
  margin-bottom: 1rem;

  img {
    display: inline;
    max-width: 100%;
    max-height: 100%;
    vertical-align: middle;
    filter: drop-shadow(2px 11px 8px rgba(0, 0, 0, 0.8));
    transform: scale(1.2);
  }
}

.login-panel {
  border-radius: 18px;
  padding: 30px;
  text-align: center;
  background: rgba(16, 96, 109, 0.65);
  color: #d5eeed;
  margin: 0 auto;
  box-shadow: 0 0 10px rgba(0, 0, 0, 0.7);
  font-size: 19px;

  &__alert {
    margin-bottom: 1rem;
  }

  &__submit {
    width: 100%;
  }
}

.signup-link {
  font-size: 15px;
  color: #bebebe;
  padding-top: 20px;
  text-align: center;

  a {
    color: #7ed0fc;
    text-decoration: none;
  }
}

@media screen and (max-width: 1440px) {
  .form-container { width: 40%; }
}
@media screen and (max-width: 1280px) {
  .form-container { width: 46%; }
  .login-panel { padding: 25px; border-radius: 15px; }
}
@media screen and (max-width: 991px) {
  .form-container { width: 54%; }
}
@media screen and (max-width: 800px) {
  .form-container { width: 60%; }
}
@media screen and (max-width: 667px) {
  .form-container { width: 75%; }
}
@media screen and (max-width: 640px) {
  .login-panel { padding: 25px; border-radius: 10px; }
  .signup-link { font-size: 13px; }
}
@media screen and (max-width: 480px) {
  .form-container { width: 92%; }
  .login-panel { padding: 15px; border-radius: 10px; }
  .signup-link { font-size: 12px; }
}
@media screen and (max-width: 384px) {
  .form-container { width: 100%; }
}
@media screen and (max-height: 600px) {
  .form-container { margin-top: -2%; }
}
</style>
