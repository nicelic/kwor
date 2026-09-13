<template>
  <div class="settings-language-manage">
    <!-- Card 1: 界面语言与偏好 -->
    <v-card rounded="xl" variant="outlined" class="mb-4 card-cyan">
      <v-card-title class="d-flex align-center justify-space-between py-3 px-4 flex-wrap ga-2">
        <div class="text-subtitle-1 font-weight-medium d-flex align-center ga-2">
          <v-icon size="small" color="primary">mdi-translate</v-icon>
          <span>{{ $t('setting.languageTitle') || '界面语言与显示偏好' }}</span>
        </div>
        <v-chip size="x-small" color="success" variant="outlined">
          {{ $t('setting.activeImmediatelyBadge') || '立即生效 / 自动同步' }}
        </v-chip>
      </v-card-title>
      <v-divider />
      <v-card-text class="pt-4">
        <!-- 语言卡片单选网格 -->
        <div class="text-body-2 text-medium-emphasis mb-3 font-weight-medium">
          {{ $t('setting.selectLanguageHint') || '请选择控制面板呈现的系统语言：' }}
        </div>
        <v-row class="mb-2">
          <v-col
            v-for="item in languageList"
            :key="item.value"
            cols="12"
            sm="6"
            md="4"
          >
            <v-card
              variant="outlined"
              class="language-card cursor-pointer pa-3"
              :class="{ 'language-card--selected': currentLocale === item.value }"
              :color="currentLocale === item.value ? 'primary' : undefined"
              @click="selectLocale(item.value)"
            >
              <div class="d-flex align-center justify-space-between">
                <div class="d-flex flex-column">
                  <span class="text-subtitle-2 font-weight-bold">{{ item.title }}</span>
                  <span class="text-caption text-medium-emphasis">{{ item.subtitle }} · {{ item.code }}</span>
                </div>
                <v-icon
                  v-if="currentLocale === item.value"
                  color="primary"
                  size="small"
                >
                  mdi-check-circle
                </v-icon>
                <v-icon
                  v-else
                  color="medium-emphasis"
                  size="small"
                >
                  mdi-circle-outline
                </v-icon>
              </div>
            </v-card>
          </v-col>
        </v-row>

        <!-- 紧凑备用下拉选择框 -->
        <v-row class="mt-2">
          <v-col cols="12" sm="6" md="4">
            <v-select
              v-model="currentLocale"
              :items="languageList"
              item-title="title"
              item-value="value"
              :label="$t('setting.language') || '当前语言'"
              density="comfortable"
              prepend-inner-icon="mdi-web"
              hide-details
              @update:model-value="selectLocale"
            />
          </v-col>
        </v-row>

        <!-- 操作按钮与辅助提示 -->
        <div class="d-flex flex-wrap align-center ga-3 mt-4 pt-2 border-t">
          <v-btn
            color="primary"
            variant="tonal"
            prepend-icon="mdi-content-save-cog-outline"
            :loading="savingDefault"
            @click="saveAsSystemDefault"
          >
            {{ $t('setting.setAsDefaultLanguage') || '设为系统默认语言' }}
          </v-btn>
          <v-btn
            variant="outlined"
            prepend-icon="mdi-compass-outline"
            @click="detectBrowserLanguage"
          >
            {{ $t('setting.followBrowserLanguage') || '跟随浏览器偏好' }}
          </v-btn>
        </div>

        <v-alert
          type="info"
          variant="tonal"
          density="compact"
          class="mt-4"
          icon="mdi-information-outline"
        >
          {{ $t('setting.languagePersistenceDesc') || '语言切换将即时刷新整站界面文本并保存到当前浏览器偏好中；点击“设为系统默认语言”可将当前配置写入服务器后端数据库，在新设备或新会话首次访问时生效。' }}
        </v-alert>
      </v-card-text>
    </v-card>

    <!-- Card 2: 国际化与本地化信息 -->
    <v-card rounded="xl" variant="outlined" class="card-green">
      <v-card-title class="py-3 px-4">
        <div class="text-subtitle-1 font-weight-medium d-flex align-center ga-2">
          <v-icon size="small" color="primary">mdi-earth</v-icon>
          <span>{{ $t('setting.i18nInfoTitle') || '国际化与本地化信息' }}</span>
        </div>
      </v-card-title>
      <v-divider />
      <v-card-text class="pt-4">
        <v-row>
          <v-col cols="12" sm="6" md="3">
            <div class="text-caption text-medium-emphasis">{{ $t('setting.currentLanguageCode') || '当前语言代码' }}</div>
            <div class="text-body-1 font-weight-medium mt-1">{{ currentLanguageMeta.code }}</div>
          </v-col>
          <v-col cols="12" sm="6" md="3">
            <div class="text-caption text-medium-emphasis">{{ $t('setting.textDirection') || '排版文字方向' }}</div>
            <div class="text-body-1 font-weight-medium mt-1">
              {{ currentLanguageMeta.isRTL ? '从右向左 (RTL)' : '从左向右 (LTR)' }}
            </div>
          </v-col>
          <v-col cols="12" sm="6" md="3">
            <div class="text-caption text-medium-emphasis">{{ $t('setting.charsetEncoding') || '字符集编码标准' }}</div>
            <div class="text-body-1 font-weight-medium mt-1">UTF-8 (Unicode)</div>
          </v-col>
          <v-col cols="12" sm="6" md="3">
            <div class="text-caption text-medium-emphasis">{{ $t('setting.serverDefaultLanguage') || '服务端持久化默认值' }}</div>
            <div class="text-body-1 font-weight-medium mt-1">{{ serverDefaultLanguageLabel }}</div>
          </v-col>
        </v-row>

        <div class="text-caption text-medium-emphasis mt-3">
          支持的语言体系：简体中文 (zh-Hans)、English (en-US)、繁體中文 (zh-Hant)、فارسی (fa-IR)、Tiếng Việt (vi-VN)、Русский (ru-RU)。全站数据与配置文件均遵循标准 UTF-8 编码，防止乱码。
        </div>
      </v-card-text>
    </v-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useLocale } from 'vuetify'
import { languages } from '@/locales'
import HttpUtils from '@/plugins/httputil'
import { push } from 'notivue'

type LanguageItem = {
  title: string
  value: string
  subtitle: string
  code: string
  isRTL: boolean
}

const languageList: LanguageItem[] = [
  { title: '简体中文', value: 'zhHans', subtitle: 'Simplified Chinese', code: 'zh-CN', isRTL: false },
  { title: 'English', value: 'en', subtitle: 'English (US)', code: 'en-US', isRTL: false },
  { title: '繁體中文', value: 'zhHant', subtitle: 'Traditional Chinese', code: 'zh-TW', isRTL: false },
  { title: 'فارسی', value: 'fa', subtitle: 'Persian', code: 'fa-IR', isRTL: true },
  { title: 'Tiếng Việt', value: 'vi', subtitle: 'Vietnamese', code: 'vi-VN', isRTL: false },
  { title: 'Русский', value: 'ru', subtitle: 'Russian', code: 'ru-RU', isRTL: false },
]

const locale = useLocale()
const currentLocale = ref<string>(localStorage.getItem('locale') ?? 'en')
const savingDefault = ref(false)
const serverDefaultLanguage = ref<string>('')

const currentLanguageMeta = computed(() => {
  return languageList.find(item => item.value === currentLocale.value) ?? languageList[0]
})

const serverDefaultLanguageLabel = computed(() => {
  if (!serverDefaultLanguage.value) return '读取中...'
  const match = languageList.find(item => item.value === serverDefaultLanguage.value)
  return match ? `${match.title} (${match.code})` : serverDefaultLanguage.value
})

const selectLocale = (val: string) => {
  if (!val) return
  currentLocale.value = val
  locale.current.value = val
  localStorage.setItem('locale', val)
}

const detectBrowserLanguage = () => {
  const navLang = navigator.language.toLowerCase()
  let matched = 'en'
  if (navLang.startsWith('zh-cn') || navLang.startsWith('zh-hans') || navLang === 'zh') {
    matched = 'zhHans'
  } else if (navLang.startsWith('zh')) {
    matched = 'zhHant'
  } else if (navLang.startsWith('fa')) {
    matched = 'fa'
  } else if (navLang.startsWith('vi')) {
    matched = 'vi'
  } else if (navLang.startsWith('ru')) {
    matched = 'ru'
  } else if (navLang.startsWith('en')) {
    matched = 'en'
  }

  selectLocale(matched)
  const item = languageList.find(l => l.value === matched)
  push.info(`已切换至浏览器语言：${item?.title || matched}`)
}

const loadServerDefaultLanguage = async () => {
  try {
    const res = await HttpUtils.get('api/settings-language', {}, { silentAuthCheck: true })
    if (res.success && res.obj?.defaultLanguage) {
      serverDefaultLanguage.value = res.obj.defaultLanguage
    }
  } catch {
    serverDefaultLanguage.value = 'zhHans'
  }
}

const saveAsSystemDefault = async () => {
  savingDefault.value = true
  try {
    const res = await HttpUtils.post('api/settings-language', { language: currentLocale.value })
    if (res.success) {
      serverDefaultLanguage.value = currentLocale.value
      push.success('已将当前语言设为系统默认语言')
    } else {
      push.error(`保存默认语言失败：${res.msg || '未知错误'}`)
    }
  } catch (err: any) {
    push.error(`保存默认语言异常：${err.message || err}`)
  } finally {
    savingDefault.value = false
  }
}

onMounted(() => {
  loadServerDefaultLanguage()
})
</script>

<style scoped>
.language-card {
  transition: all 0.2s ease-in-out;
  border-width: 1px;
}
.language-card:hover {
  border-color: rgba(var(--v-theme-primary), 0.6);
  transform: translateY(-1px);
}
.language-card--selected {
  border-width: 2px;
  background-color: rgba(var(--v-theme-primary), 0.08);
}
.cursor-pointer {
  cursor: pointer;
}
.border-t {
  border-top: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
}

.card-cyan {
  border-color: rgba(34, 211, 238, 0.45) !important;
  transition: border-color 0.2s ease;
}
.card-cyan:hover {
  border-color: rgba(34, 211, 238, 0.75) !important;
}
.card-green {
  border-color: rgba(74, 222, 128, 0.45) !important;
  transition: border-color 0.2s ease;
}
.card-green:hover {
  border-color: rgba(74, 222, 128, 0.75) !important;
}
</style>
