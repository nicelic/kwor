<script lang="ts" setup>
import { HumanReadable } from '@/plugins/utils'
import { computed } from 'vue'

const props = defineProps({
  tilesData: <any>{},
  type: String
})

const toFiniteNumber = (value: unknown): number => {
  if (typeof value === 'number') {
    return Number.isFinite(value) ? value : 0
  }
  if (typeof value === 'string') {
    const parsed = Number(value)
    return Number.isFinite(parsed) ? parsed : 0
  }
  return 0
}

const clampPercent = (value: number): number => {
  if (!Number.isFinite(value)) return 0
  if (value < 0) return 0
  if (value > 100) return 100
  return value
}

const data = computed(() => {
  const d = props.tilesData
  switch (props.type) {
    case 'g-cpu':
      return cpuGaugeData(d.cpu)
    case 'g-mem':
      return gaugeData(d.mem)
    case 'g-dsk':
      return gaugeData(d.dsk)
    case 'g-swp':
      return gaugeData(d.swp)
  }
  return { percent: 0, text: '-'}
})

const cpuGaugeData = (value: unknown): any => {
  const percent = clampPercent(Math.ceil(toFiniteNumber(value)))
  return {
    percent,
    text: `${percent}%`,
  }
}

const gaugeData = (d:any) :any => {
  if (!d) return { percent: 0, text: '-' }
  const current = toFiniteNumber(d.current)
  const total = toFiniteNumber(d.total)
  if (total <= 0) {
    return { percent: 0, text: '-' }
  }
  const curr = HumanReadable.sizeFormat(current, 0).split(' ')
  const totalText = HumanReadable.sizeFormat(total, 0).split(' ')
  if (curr[1] === totalText[1]) curr[1] = ''
  return {
    percent: clampPercent(Math.ceil(current * 100 / total)),
    text: curr[0] + "<sup>" + (curr[1] ?? ' ') + "</sup>/" + totalText[0] + "<sup>" + (totalText[1] ?? '') + "</sup>"
  }
}

const cssTransformRotateValue = computed(() => {
  const percentageAsFraction = clampPercent(data.value.percent) / 100
  const halfPercentage = percentageAsFraction / 2

  return `${halfPercentage}turn`
})

const gaugeColor = computed(() => {
  const percent = clampPercent(data.value.percent)
  if (percent > 90) return 'error'
  if (percent > 70) return 'warning'
  return 'info'
})
</script>

<template>
  <div class="gauge__outer">
    <div class="gauge__inner">
      <div
        class="gauge__fill" 
        :style="{ 
          transform: `rotate(${cssTransformRotateValue})`,
          background: `rgb(var(--v-theme-${gaugeColor}))`
          }">
      </div>
      <div class="gauge__cover"><span dir="ltr" v-html="data.text"></span></div>
    </div>
  </div>
</template>

<style scoped>
.gauge__outer {
  width: 100%;
  max-width: 250px;
}

.gauge__inner {
  width: 100%;
  height: 0;
  padding-bottom: 50%;
  background: rgb(var(--v-theme-surface));
  position: relative;
  border-top-left-radius: 100% 200%;
  border-top-right-radius: 100% 200%;
  overflow: hidden;
}

.gauge__fill {
  position: absolute;
  top: 100%;
  left: 0;
  width: inherit;
  height: 100%;
  background: rgb(var(--v-theme-primary));
  transform-origin: center top;
  transform: rotate(0turn);
  transition: transform 0.2s ease-out;
}

.gauge__cover {
  width: 75%;
  height: 150%;
  background: rgb(var(--v-theme-background));
  position: absolute;
  top: 25%;
  left: 50%;
  transform: translateX(-50%);
  border-radius: 50%;

  /* Text */
  display: flex;
  align-items: center;
  justify-content: center;
  padding-bottom: 25%;
  box-sizing: border-box;
  font-family: 'Lexend', sans-serif;
  font-weight: bold;
  font-size: 32px;
}

sup {
  font-size: 16px;
}
</style>
