<template>
  <v-dialog v-model="dialogVisible" transition="dialog-bottom-transition" width="800" max-width="90vw" max-height="90vh">
    <v-card class="rounded-lg" :loading="loading">
      <v-card-title>
        <v-row>
          <v-col cols="auto">
            {{ $t("stats.graphTitle") }}
          </v-col>
          <v-spacer></v-spacer>
          <v-col cols="auto">
            <v-tooltip location="top" :text="$t('actions.close')">
              <template #activator="{ props: tooltipProps }">
                <v-btn v-bind="tooltipProps" icon="mdi-close" density="compact" variant="text" @click="closeDialog" />
              </template>
            </v-tooltip>
          </v-col>
        </v-row>
      </v-card-title>
      <v-divider></v-divider>
      <v-card-text style="padding: 0 16px; max-height: calc(90vh - 92px); overflow-y: auto">
        <div style="text-align: center; margin: 5px">
          {{ $t("objects." + resource) + " : " + tag }}
        </div>
        <v-radio-group
          v-model="limit"
          @change="changePeriod"
          density="compact"
          :loading="loading"
          inline
          hide-details
        >
          <v-radio
            v-for="p in periods"
            :key="p.value"
            :label="p.title"
            :value="p.value"
          ></v-radio>
        </v-radio-group>
        <v-container id="container" style="height: 40vh">
          <v-skeleton-loader
            class="mx-auto border"
            width="95%"
            type="image"
            v-if="loading"
          ></v-skeleton-loader>
          <template v-else>
            <v-alert
              :text="$t('noData')"
              type="warning"
              variant="outlined"
              v-if="alert"
            ></v-alert>
            <Line v-if="loaded" :data="usage" :options="<any>options" />
          </template>
        </v-container>
      </v-card-text>
    </v-card>
  </v-dialog>
</template>

<script lang="ts">
import { i18n } from "@/locales";
import HttpUtils from "@/plugins/httputil";
import { HumanReadable } from "@/plugins/utils";
import { formatPanelDateTime } from "@/plugins/panelTime";
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler,
} from "chart.js";
import { ref } from "vue";
import { Line } from "vue-chartjs";
ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler,
);
ChartJS.defaults.font.family = "Vazirmatn";
export default {
  components: {
    Line,
  },
  props: ["modelValue", "visible", "resource", "tag", "namespace"],
  emits: ["close", "update:modelValue"],
  data() {
    return {
      loading: false,
      loaded: false,
      alert: false,
      intervalId: <any>0,
      pollingEnabled: false,
      loadSeq: 0,
      loadController: null as AbortController | null,
      limit: 1,
      periods: [
        { value: 1, title: i18n.global.n(1) + i18n.global.t("date.h") },
        { value: 6, title: i18n.global.n(6) + i18n.global.t("date.h") },
        { value: 12, title: i18n.global.n(12) + i18n.global.t("date.h") },
        { value: 24, title: i18n.global.n(1) + i18n.global.t("date.d") },
        { value: 48, title: i18n.global.n(2) + i18n.global.t("date.d") },
        { value: 240, title: i18n.global.n(10) + i18n.global.t("date.d") },
        { value: 480, title: i18n.global.n(20) + i18n.global.t("date.d") },
        { value: 720, title: i18n.global.n(30) + i18n.global.t("date.d") },
        { value: 1440, title: i18n.global.n(60) + i18n.global.t("date.d") },
        { value: 2160, title: i18n.global.n(90) + i18n.global.t("date.d") },
      ],
      options: {
        responsive: true,
        maintainAspectRatio: false,
        interaction: {
          intersect: false,
          mode: "index",
        },
        elements: {
          point: { pointStyle: "crossRot" },
        },
        plugins: {
          tooltip: {
            callbacks: {
              label: (ctx: any) => {
                const value = Number(ctx.raw ?? 0);
                return `${ctx.dataset?.label ?? ""}: ${HumanReadable.sizeFormat(Number.isFinite(value) ? value : 0)}`;
              },
              footer: (items: any[]) => {
                return HumanReadable.sizeFormat(
                  items.reduce((acc, c) => {
                    const value = Number(c.raw ?? 0);
                    return acc + (Number.isFinite(value) ? value : 0);
                  }, 0),
                );
              },
            },
          },
        },
        scales: {
          y: {
            grid: {
              color: "#777777",
            },
            beginAtZero: true,
            ticks: {
              callback: function (label: any, index: number) {
                return label == 0 ? 0 : HumanReadable.sizeFormat(label, 0);
              },
              count: 10,
            },
          },
        },
      },
      usage: ref(<any>{}),
    };
  },
  computed: {
    dialogVisible: {
      get(): boolean {
        return this.$props.modelValue ?? this.$props.visible ?? false;
      },
      set(value: boolean) {
        this.$emit("update:modelValue", value);
        if (!value) this.$emit("close");
      },
    },
  },
  methods: {
    closeDialog() {
      this.dialogVisible = false;
    },
    cancelPendingLoad() {
      this.loadSeq += 1;
      this.loadController?.abort();
      this.loadController = null;
    },
    isCurrentLoad(controller: AbortController, requestSeq: number, resource: string, tag: string, namespace: string) {
      return !controller.signal.aborted &&
        this.loadController === controller &&
        this.loadSeq === requestSeq &&
        this.dialogVisible === true &&
        this.$props.resource === resource &&
        this.$props.tag === tag &&
        (this.$props.namespace ?? "default") === namespace;
    },
    stopPolling() {
      if (this.intervalId && this.intervalId != 0) {
        clearTimeout(this.intervalId);
        this.intervalId = 0;
      }
      this.pollingEnabled = false;
    },
    schedulePolling() {
      if (!this.pollingEnabled || !this.dialogVisible) return;
      if (typeof document !== "undefined" && document.visibilityState !== "visible") return;
      if (this.intervalId && this.intervalId != 0) clearTimeout(this.intervalId);
      this.intervalId = setTimeout(() => {
        this.intervalId = 0;
        if (this.loading) {
          this.schedulePolling();
          return;
        }
        void this.loadData();
      }, this.pollingIntervalMs());
    },
    startPolling() {
      this.stopPolling();
      if (!this.dialogVisible) {
        return;
      }
      if (typeof document !== "undefined" && document.visibilityState !== "visible") {
        return;
      }
      this.pollingEnabled = true;
      if (!this.loading) this.schedulePolling();
    },
    pollingIntervalMs() {
      if (this.limit <= 6) return 10000;
      if (this.limit <= 24) return 30000;
      if (this.limit <= 240) return 60000;
      return 300000;
    },
    changePeriod() {
      this.loadData(true);
      this.startPolling();
    },
    handleVisibilityChange() {
      if (document.visibilityState === "visible") {
        if (this.dialogVisible) {
          this.loadData(true);
          this.startPolling();
        }
        return;
      }
      this.stopPolling();
      this.cancelPendingLoad();
    },
    async loadData(replacePending = false) {
      if (this.loading) {
        if (!replacePending) return;
        this.cancelPendingLoad();
      }
      const controller = new AbortController();
      const requestSeq = ++this.loadSeq;
      const resource = String(this.$props.resource ?? "");
      const tag = String(this.$props.tag ?? "");
      const namespace = this.$props.namespace ?? "default";
      this.loadController = controller;
      this.loading = true;
      try {
        const data = await HttpUtils.get("api/stats", {
          resource,
          tag,
          limit: this.limit,
          namespace,
        }, { signal: controller.signal });
        if (!this.isCurrentLoad(controller, requestSeq, resource, tag, namespace)) return;
        if (data.success && Array.isArray(data.obj)) {
          const obj = data.obj
            .filter((item: unknown): item is Record<string, unknown> => item !== null && typeof item === 'object' && !Array.isArray(item))
            .map((item) => {
              const traffic = Number(item.traffic ?? 0);
              return {
                dateTime: Number(item.dateTime),
                direction: Boolean(item.direction),
                traffic: Number.isFinite(traffic) ? traffic : 0,
              };
            })
            .filter((item) => Number.isFinite(item.dateTime))
            .sort((a, b) => a.dateTime - b.dateTime);
          const l = String(i18n.global.locale) == "fa" ? "fa-IR" : "en-US";
          const labels = <string[]>[];
          const uplinkData = <(number | null)[]>[];
          const downlinkData = <(number | null)[]>[];
          const grouped = new Map<
            number,
            { up: number | null; down: number | null }
          >();
          for (const item of obj) {
            const bucket = Number(item.dateTime) * 1000;
            if (!grouped.has(bucket)) {
              grouped.set(bucket, { up: null, down: null });
            }
            const point = grouped.get(bucket)!;
            if (item.direction) {
              point.up = (point.up ?? 0) + item.traffic;
            } else {
              point.down = (point.down ?? 0) + item.traffic;
            }
          }
          const buckets = Array.from(grouped.keys()).sort((a, b) => a - b);
          for (const bucket of buckets) {
            const point = grouped.get(bucket)!;
            labels.push(this.genLable(bucket, l));
            uplinkData.push(point.up);
            downlinkData.push(point.down);
          }
          this.usage = {
            labels: labels,
            datasets: [
              {
                label: i18n.global.t("stats.upload"),
                backgroundColor: "rgba(255, 165, 0, 0.4)",
                borderColor: "rgba(255, 165, 0)",
                fill: true,
                data: uplinkData,
              },
              {
                label: i18n.global.t("stats.download"),
                backgroundColor: "rgba(0, 128, 0, 0.2)",
                borderColor: "rgba(0, 128, 0)",
                fill: true,
                data: downlinkData,
              },
            ],
          };
          this.loaded = labels.length > 0;
          this.alert = labels.length === 0;
        } else {
          this.alert = true;
          this.loaded = false;
        }
      } catch {
        if (this.isCurrentLoad(controller, requestSeq, resource, tag, namespace)) {
          this.alert = true;
          this.loaded = false;
        }
      } finally {
        if (this.loadController === controller) {
          this.loadController = null;
          this.loading = false;
          this.schedulePolling();
        }
      }
    },
    genLable(step: number, locale: string) {
      return formatPanelDateTime(step, locale, {
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        hour12: false,
      });
    },
  },
  watch: {
    dialogVisible(v) {
      if (v) {
        this.limit = 1;
        this.loadData(true);
        this.startPolling();
      } else {
        this.cancelPendingLoad();
        this.loaded = false;
        this.alert = false;
        this.usage.labels = [];
        if (this.usage.datasets) {
          this.usage.datasets[0].data = [];
          this.usage.datasets[1].data = [];
        }
        this.stopPolling();
      }
    },
    tag() {
      if (!this.dialogVisible) return;
      this.loadData(true);
      this.startPolling();
    },
    resource() {
      if (!this.dialogVisible) return;
      this.loadData(true);
      this.startPolling();
    },
    namespace() {
      if (!this.dialogVisible) return;
      this.loadData(true);
      this.startPolling();
    },
  },
  mounted() {
    if (typeof document !== "undefined") {
      document.addEventListener("visibilitychange", this.handleVisibilityChange);
    }
  },
  beforeUnmount() {
    this.stopPolling();
    this.cancelPendingLoad();
    if (typeof document !== "undefined") {
      document.removeEventListener("visibilitychange", this.handleVisibilityChange);
    }
  },
};
</script>
