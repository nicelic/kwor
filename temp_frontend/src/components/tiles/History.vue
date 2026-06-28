<template>
  <Line v-if="loaded" :data="data" :options="<any>options" />
</template>

<script lang="ts">
import { ref } from 'vue'
import { Line } from 'vue-chartjs'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Filler,
} from 'chart.js'
import { HumanReadable } from '@/plugins/utils'
ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Filler
)
ChartJS.defaults.font.family = 'Vazirmatn'
export default {
  components: {
    Line
  },
  props: ['tilesData','type'],
  data() {
    return {
      loaded: false,
      labels: new Array(20).fill(''),
      oldValues: <any>{net: {}, dio: {}},
      options1: {
        animation: false,
        responsive: true,
        maintainAspectRatio: false,
        interaction: {
          intersect: false,
          mode: 'index',
        },
        plugins: {
          tooltip: {
            enabled: false
          },
          legend: {
              display: false,
          }
        },
        scales: {
          y: {
            min: 0,
            max: 100,
            grid: {
              color: '#777777',
            },
            beginAtZero: true,
            ticks: {
                beginAtZero: true,
                steps: 10,
                stepValue: 5,
                max: 100
            }
          }
        }
      },
      optionsNet: {
        animation: false,
        responsive: true,
        maintainAspectRatio: false,
        interaction: {
          intersect: false,
          mode: 'index',
        },
        plugins: {
          tooltip: {
            enabled: false
          },
          legend: {
              display: false,
          }
        },
        scales: {
          y: {
            grid: {
              color: '#777777',
            },
            beginAtZero: true,
            ticks: {
              callback: (label:any, index: number) => { return parseInt(label).toString() },
              count: 10
            }
          }
        }
      },
      data: ref(<any>{})
    }
  },
  computed: {
    options() {
      switch (this.$props.type){
        case "h-net":
          this.optionsNet.scales.y.ticks.callback = (label:any, index: number) => {
            return label == 0 ? "0" : HumanReadable.sizeFormat(label,0)
          }
          return this.optionsNet
        case "hp-net":
          this.optionsNet.scales.y.ticks.callback = (label:any, index: number) => {
            return label == 0 ? "0" : HumanReadable.packetFormat(label,0)
          }
          return this.optionsNet
        case "h-dio":
          this.optionsNet.scales.y.ticks.callback = (label:any, index: number) => {
            return label == 0 ? "0" : HumanReadable.sizeFormat(label,0)
          }
          return this.optionsNet
      }
      return this.options1
    }
  },
  methods: {
    toFiniteNumber(value: unknown) {
      if (typeof value === 'number') {
        return Number.isFinite(value) ? value : 0
      }
      if (typeof value === 'string') {
        const parsed = Number(value)
        return Number.isFinite(parsed) ? parsed : 0
      }
      return 0
    },
    clampPercent(value: unknown) {
      const numberValue = this.toFiniteNumber(value)
      if (numberValue < 0) return 0
      if (numberValue > 100) return 100
      return numberValue
    },
    getPerSecondDelta(current: unknown, previous: unknown) {
      const currentValue = this.toFiniteNumber(current)
      const previousValue = this.toFiniteNumber(previous)
      if (currentValue < previousValue) return 0
      return Math.max(0, (currentValue - previousValue) / 2)
    },
    updateData1(value1: number) {
      const newData = <number[]>[]
      if (this.data.datasets){
        newData.push(...this.data.datasets[0].data,value1)
      }
      if (newData.length>20) newData.shift()
      this.data = {
        labels: this.labels,
        datasets: [
          {
            label: '',
            backgroundColor: 'rgba(255, 165, 0, 0.2)',
            borderColor: 'rgba(255, 165, 0,0.8)',
            fill: true,
            data: newData
          }
        ],
      }
      this.loaded = true
    },
    updateData2(value1: number, value2:number) {
      const newData1 = <number[]>[]
      const newData2 = <number[]>[]
      if (this.data.datasets){
        newData1.push(...this.data.datasets[0].data,value1)
        newData2.push(...this.data.datasets[1].data,value2)
      }
      if (newData1.length>20) newData1.shift()
      if (newData2.length>20) newData2.shift()
      this.data = {
        labels: this.labels,
        datasets: [
          {
            label: '',
            backgroundColor: 'rgba(255, 165, 0, 0.2)',
            borderColor: 'rgba(255, 165, 0,0.8)',
            fill: true,
            data: newData1
          },
          {
            label: '',
            backgroundColor: 'rgba(0, 128, 0, 0.1)',
            borderColor: 'rgba(0, 128, 0,0.8)',
            fill: true,
            data: newData2
          }
        ],
      }
      this.loaded = true
    }
  },
  watch: {
    tilesData(v:any) {
      if (!v || typeof v !== 'object') return
      switch (this.$props.type) {
        case 'h-cpu':
          this.updateData1(this.clampPercent(v.cpu))
          break
        case 'h-mem':
          if (!v.mem) break
          if (this.toFiniteNumber(v.mem.total) <= 0) {
            this.updateData1(0)
            break
          }
          this.updateData1(this.clampPercent(this.toFiniteNumber(v.mem.current) * 100 / this.toFiniteNumber(v.mem.total)))
          break
        case 'h-net':
          if (v.net && Object.keys(this.oldValues.net).length > 0) {
            const downSpeed = this.getPerSecondDelta(v.net.recv, this.oldValues.net.recv)
            const upSpeed = this.getPerSecondDelta(v.net.sent, this.oldValues.net.sent)
            this.updateData2(upSpeed,downSpeed)
          }
          this.oldValues.net = v.net ?? {}
          break
        case 'hp-net':
          if (v.net && Object.keys(this.oldValues.net).length > 0) {
            const downSpeed = this.getPerSecondDelta(v.net.precv, this.oldValues.net.precv)
            const upSpeed = this.getPerSecondDelta(v.net.psent, this.oldValues.net.psent)
            this.updateData2(upSpeed,downSpeed)
          }
          this.oldValues.net = v.net ?? {}
          break
        case 'h-dio':
          if (v.dio && Object.keys(this.oldValues.dio).length > 0) {
            const downSpeed = this.getPerSecondDelta(v.dio.read, this.oldValues.dio.read)
            const upSpeed = this.getPerSecondDelta(v.dio.write, this.oldValues.dio.write)
            this.updateData2(upSpeed,downSpeed)
          }
          this.oldValues.dio = v.dio ?? {}
          break
      }
    }
  }
}
</script>
