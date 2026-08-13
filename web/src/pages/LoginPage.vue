<template>
  <q-layout view="hHh lpR fFf">
    <q-page-container>
      <q-page class="flex flex-center column q-pa-xl">
        <div class="dasm-shell" style="max-width: 460px; width: 100%">
          <div class="dasm-shell__content text-center">
            <div class="dasm-caps">dasm-burner</div>
            <h1 class="dasm-title">Sign in</h1>
            <p class="dasm-subtitle q-mb-lg">
              Use the dasmlab Keycloak realm. Access requires the
              <code>dasm-burner</code> client role <code>admin</code>.
            </p>
            <q-btn
              color="primary"
              unelevated
              size="lg"
              icon="login"
              label="Sign in with Keycloak"
              :loading="busy"
              @click="doLogin"
            />
            <p class="text-caption text-grey-7 q-mt-lg">
              NOT FOR USE ON ANY CLUSTER THAT IS IMPORTANT
            </p>
          </div>
        </div>
      </q-page>
    </q-page-container>
  </q-layout>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuth } from 'src/services/auth'

const auth = useAuth()
const route = useRoute()
const router = useRouter()
const busy = ref(false)

function doLogin() {
  busy.value = true
  auth.login()
}

onMounted(async () => {
  await auth.init()
  if (!auth.authEnabled.value || auth.isAuthenticated.value) {
    router.replace(typeof route.query.returnTo === 'string' ? route.query.returnTo : '/')
  }
})
</script>
