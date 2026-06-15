<template>
  <v-text-field
    id="expiry"
    :label="displayLabel"
    v-model="dateFormatted"
    prepend-inner-icon="mdi-calendar"
    readonly
    hide-details
  ></v-text-field>
  <DatePicker
    v-model="Input"
    @input="Input=$event"
    :locale="pickerLocale"
    :format="resolvedPickerFormat"
    element="expiry"
    :compact-time="pickerType === 'datetime'"
    :type="pickerType">
      <template v-slot:next-month>
        <v-icon icon="mdi-chevron-right" />
      </template>
      <template v-slot:prev-month>
        <v-icon icon="mdi-chevron-left" />
      </template>
      <template #submit-btn="{ submit, canSubmit  }">
        <v-btn
          :disabled="!canSubmit"
          @click="submit"
        >{{ $t('submit') }}</v-btn>
      </template>
      <template #cancel-btn="{ vm }">
        <v-btn
          @click="reset(vm)"
        >{{ $t('reset') }}</v-btn>
      </template>
      <template #now-btn="{ goToday }">
        <v-btn
          @click="goToday"
        >{{ $t('now') }}</v-btn>
      </template>
    </DatePicker>
</template>

<script lang="ts">
import DatePicker from 'vue3-persian-datetime-picker'
import { i18n } from '@/locales'
import 'moment/locale/vi'
import 'moment/locale/zh-cn'
import 'moment/locale/zh-tw'

export default {
  props: {
    expiry: {
      type: [Number, String],
      default: 0,
    },
    labelText: {
      type: String,
      default: '',
    },
    zeroText: {
      type: String,
      default: '',
    },
    pickerType: {
      type: String,
      default: 'datetime',
    },
    pickerFormat: {
      type: String,
      default: '',
    },
    submitMode: {
      type: String,
      default: 'exact',
    },
  },
  emits: ['submit'],
  data() {
    return {
      menu: false,
      input: new Date(),
    }
  },
  components: { DatePicker },
  computed: {
    displayLabel() {
      const custom = (this.labelText ?? '').trim()
      return custom.length > 0 ? custom : i18n.global.t('date.expiry')
    },
    displayLocale() {
      const l = i18n.global.locale.value
      switch (l) {
        case "zhHans":
          return "zh-cn"
        case "zhHant":
          return "zh-tw"
        default:
          return l
      }
    },
    pickerLocale() {
      const l = String(i18n.global.locale.value ?? '').trim().toLowerCase()
      return l.startsWith('fa') ? 'fa' : 'en'
    },
    resolvedPickerFormat() {
      const custom = String(this.pickerFormat ?? '').trim()
      if (custom.length > 0) {
        return custom
      }
      switch (this.pickerType) {
        case 'time':
          return 'HH:mm'
        case 'year':
          return 'YYYY'
        case 'month':
          return 'MM'
        case 'date':
          return 'YYYY/MM/DD'
        default:
          return 'YYYY/MM/DD HH:mm'
      }
    },
    dateFormatted() {
      if (this.displayEpoch == 0) {
        const customZeroText = (this.zeroText ?? '').trim()
        return customZeroText.length > 0 ? customZeroText : i18n.global.t('unlimited')
      }
      const date = new Date(this.displayEpoch * 1000)
      if (this.pickerType === 'date') {
        return date.toLocaleDateString(this.displayLocale)
      }
      return date.toLocaleString(this.displayLocale)
    },
    expDate() {
      const raw = Number(this.expiry ?? 0)
      if (!Number.isFinite(raw) || raw <= 0) {
        return 0
      }
      return Math.floor(raw)
    },
    displayEpoch() {
      if (this.expDate == 0) {
        return 0
      }
      if (this.submitMode !== 'day-end') {
        return this.expDate
      }
      const date = new Date(this.expDate * 1000)
      if (date.getHours() === 0 && date.getMinutes() === 0 && date.getSeconds() === 0) {
        return Math.max(0, this.expDate - 1)
      }
      return this.expDate
    },
    Input: {
      get() { return this.displayEpoch == 0 ? new Date() : new Date(this.displayEpoch * 1000) },
      set(v: unknown) {
        const parsed = this.coerceToDate(v)
        if (parsed == null) {
          return
        }
        this.input = parsed
        this.submit()
      }
    }
  },
  methods: {
    updateInput(v:Date) {
      this.input = v
    },
    setNow() {
      this.input = new Date()
    },
    toDateFromEpoch(raw: number): Date | null {
      if (!Number.isFinite(raw)) {
        return null
      }
      const abs = Math.abs(raw)
      const millis = abs > 0 && abs < 1e11 ? raw * 1000 : raw
      const date = new Date(millis)
      return Number.isFinite(date.getTime()) ? date : null
    },
    parseYMD(value: string): Date | null {
      const match = value.match(/^(\d{4})[\/.-](\d{1,2})[\/.-](\d{1,2})(?:\s.*)?$/)
      if (!match) {
        return null
      }

      const year = Number(match[1])
      const month = Number(match[2])
      const day = Number(match[3])
      if (!Number.isInteger(year) || !Number.isInteger(month) || !Number.isInteger(day)) {
        return null
      }
      if (month < 1 || month > 12 || day < 1 || day > 31) {
        return null
      }

      const date = new Date(year, month - 1, day, 0, 0, 0, 0)
      if (
        date.getFullYear() !== year ||
        date.getMonth() !== month - 1 ||
        date.getDate() !== day
      ) {
        return null
      }
      return date
    },
    coerceToDate(value: unknown): Date | null {
      if (value instanceof Date) {
        return Number.isFinite(value.getTime()) ? new Date(value.getTime()) : null
      }

      if (typeof value === 'number') {
        return this.toDateFromEpoch(value)
      }

      if (typeof value !== 'string') {
        return null
      }

      const trimmed = value.trim()
      if (trimmed.length === 0) {
        return null
      }

      if (/^-?\d+(?:\.\d+)?$/.test(trimmed)) {
        return this.toDateFromEpoch(Number(trimmed))
      }

      const byYMD = this.parseYMD(trimmed)
      if (byYMD != null) {
        return byYMD
      }

      const timestamp = Date.parse(trimmed)
      if (!Number.isFinite(timestamp)) {
        return null
      }

      const date = new Date(timestamp)
      return Number.isFinite(date.getTime()) ? date : null
    },
    submit() {
      const parsed = this.coerceToDate(this.input)
      if (parsed == null) {
        return
      }

      const next = new Date(parsed.getTime())
      if (this.submitMode === 'day-end') {
        next.setHours(0, 0, 0, 0)
        next.setDate(next.getDate() + 1)
      }
      const epoch = Math.floor(next.getTime() / 1000)
      if (!Number.isFinite(epoch) || epoch <= 0) {
        return
      }
      this.$emit('submit', epoch)
    },
    reset(vm:any) {
      this.$emit('submit',0)
      this.input = new Date()
      vm.visible = false
    }
  },
  watch: {
    menu(v) {
      if (v) {
        this.input = this.displayEpoch == 0 ? new Date() : new Date(this.displayEpoch * 1000)
      }
    }
  }
};
</script>

<style>
.vpd-addon-list,
.vpd-addon-list-item {
  background-color: rgb(var(--v-theme-background)) !important;
  border-color: rgb(var(--v-theme-background)) !important;
}
.vpd-content {
  background-color: rgb(var(--v-theme-background)) !important;
}
.vpd-addon-list-item.vpd-selected,
.vpd-addon-list-item:hover {
  background-color: rgb(var(--v-theme-primary)) !important;
}
.vpd-close-addon {
  color: rgb(var(--v-theme-on-surface)) !important;
  background-color: transparent;
}
.vpd-controls {
  overflow-x: hidden;
}
.vpd-month-label {
  width: auto;
}
.vpd-actions button:hover {
  background-color: transparent;
}
.vpd-wrapper[data-type=datetime].vpd-compact-time .vpd-time {
  border-top: 0;
}
.vpd-time .vpd-time-h .vpd-counter-item,
.vpd-time .vpd-time-m .vpd-counter-item {
  vertical-align: top;
}
</style>
