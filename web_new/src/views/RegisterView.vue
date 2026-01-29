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
      error.value = 'Нет соединения с сервером'
    } else {
      error.value = 'Произошла ошибка. Попробуйте позже.'
    }
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="register-view">
    <div class="register-card">
      <h1 class="register-card__title">Регистрация</h1>

      <AppAlert v-if="error" type="error" class="register-card__alert">
        {{ error }}
      </AppAlert>

      <form class="register-card__form" @submit.prevent="handleSubmit">
        <AppInput
          v-model="form.login"
          label="Логин"
          placeholder="Минимум 3 символа"
          :error="fieldErrors.login"
          :disabled="loading"
          autocomplete="username"
        />

        <AppInput
          v-model="form.password"
          type="password"
          label="Пароль"
          placeholder="Минимум 4 символа"
          :error="fieldErrors.password"
          :disabled="loading"
          autocomplete="new-password"
        />

        <AppInput
          v-model="form.confirmPassword"
          type="password"
          label="Подтверждение пароля"
          placeholder="Повторите пароль"
          :error="passwordMismatch ? 'Пароли не совпадают' : ''"
          :disabled="loading"
          autocomplete="new-password"
        />

        <AppButton
          type="submit"
          :loading="loading"
          :disabled="!isFormValid"
          class="register-card__submit"
        >
          Зарегистрироваться
        </AppButton>
      </form>

      <p class="register-card__footer">
        Уже есть аккаунт?
        <RouterLink to="/login">Войти</RouterLink>
      </p>
    </div>
  </div>
</template>

<style scoped lang="scss">
.register-view {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100%;
  padding: 1rem;
}

.register-card {
  width: 100%;
  max-width: 400px;
  padding: 2rem;
  background-color: #1f2937;
  border-radius: 12px;
  box-shadow: 0 4px 24px rgba(0, 0, 0, 0.3);

  &__title {
    margin-bottom: 1.5rem;
    font-size: 1.75rem;
    font-weight: 600;
    text-align: center;
    color: #e0e0e0;
  }

  &__alert {
    margin-bottom: 1rem;
  }

  &__form {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  &__submit {
    margin-top: 0.5rem;
    width: 100%;
  }

  &__footer {
    margin-top: 1.5rem;
    text-align: center;
    font-size: 0.875rem;
    color: #a0a0a0;

    a {
      color: #42b883;
      
      &:hover {
        text-decoration: underline;
      }
    }
  }
}
</style>
