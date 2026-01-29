<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/authStore'
import { login } from '@/api/auth'
import { ApiException } from '@/api/errors'
import AppInput from '@/components/ui/AppInput.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppAlert from '@/components/ui/AppAlert.vue'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()

const form = ref({
  login: '',
  password: '',
})

const loading = ref(false)
const error = ref('')
const fieldErrors = ref<Record<string, string>>({})

const isFormValid = computed(() => {
  return form.value.login.trim().length >= 3 && form.value.password.length >= 4
})

async function handleSubmit() {
  if (!isFormValid.value || loading.value) return

  loading.value = true
  error.value = ''
  fieldErrors.value = {}

  try {
    const response = await login({
      login: form.value.login.trim(),
      password: form.value.password,
    })

    authStore.setToken(response.token)

    const redirect = route.query.redirect as string
    router.push(redirect || '/characters')
  } catch (e) {
    if (ApiException.isValidation(e)) {
      error.value = e.message
      if (e.details) {
        for (const [field, messages] of Object.entries(e.details)) {
          fieldErrors.value[field] = messages[0] || ''
        }
      }
    } else if (ApiException.isAuth(e)) {
      error.value = 'Неверный логин или пароль'
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
  <div class="login-view">
    <div class="login-card">
      <h1 class="login-card__title">Вход</h1>

      <AppAlert v-if="error" type="error" class="login-card__alert">
        {{ error }}
      </AppAlert>

      <form class="login-card__form" @submit.prevent="handleSubmit">
        <AppInput
          v-model="form.login"
          label="Логин"
          placeholder="Введите логин"
          :error="fieldErrors.login"
          :disabled="loading"
          autocomplete="username"
        />

        <AppInput
          v-model="form.password"
          type="password"
          label="Пароль"
          placeholder="Введите пароль"
          :error="fieldErrors.password"
          :disabled="loading"
          autocomplete="current-password"
        />

        <AppButton
          type="submit"
          :loading="loading"
          :disabled="!isFormValid"
          class="login-card__submit"
        >
          Войти
        </AppButton>
      </form>

      <p class="login-card__footer">
        Нет аккаунта?
        <RouterLink to="/register">Зарегистрироваться</RouterLink>
      </p>
    </div>
  </div>
</template>

<style scoped lang="scss">
.login-view {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100%;
  padding: 1rem;
}

.login-card {
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
