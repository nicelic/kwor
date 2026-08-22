<template>
  <v-dialog transition="dialog-bottom-transition" max-width="800" width="calc(100vw - 24px)">
    <v-card>
      <v-card-title>
        {{ title }}
      </v-card-title>
      <v-divider></v-divider>
      <v-card-text class="editor-card-text">
        <div class="code-editor" :class="{ 'code-editor--large': isLargeDocument }">
          <div v-if="showLineNumbers" ref="lineNumbersRef" class="line-numbers">
            <span v-for="n in lineNumberCount" :key="n">{{ n }}</span>
          </div>
          <v-textarea
            ref="textareaRef"
            v-model="content"
            @scroll="syncScroll"
            hide-details
            variant="outlined"
            bg-color="background"
            :style="{ 'font-family': 'monospace' }"
            rows="20"
            no-resize
          ></v-textarea>
        </div>
      </v-card-text>
      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn
          color="primary"
          variant="outlined"
          @click="closeModal"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="tonal"
          @click="saveChanges"
        >
          {{ $t('actions.save') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts">
import { useTheme } from 'vuetify'

const EDITOR_LARGE_DOCUMENT_BYTES = 256 * 1024
const EDITOR_MAX_LINE_NUMBER_ROWS = 2000

function countDocumentLines(value: string, maximum: number): number {
  let count = 1
  for (let index = 0; index < value.length; index += 1) {
    if (value.charCodeAt(index) !== 10) continue
    count += 1
    if (count > maximum) return count
  }
  return count
}

export default {
  props: ['visible', 'data', 'title'],
  emits: ['close', 'save'],
  data() {
    return {
      content: this.$props.data,
      theme: useTheme()
    }
  },
  computed: {
    isLargeDocument() {
      return String(this.content ?? '').length > EDITOR_LARGE_DOCUMENT_BYTES
    },
    lineNumberCount() {
      if (this.isLargeDocument) return 0
      return countDocumentLines(String(this.content ?? ''), EDITOR_MAX_LINE_NUMBER_ROWS + 1)
    },
    showLineNumbers() {
      return this.lineNumberCount > 0 && this.lineNumberCount <= EDITOR_MAX_LINE_NUMBER_ROWS
    }
  },
  methods: {
    syncScroll() {
      const textareaComponent = this.$refs.textareaRef as { $el?: HTMLElement } | undefined
      const textarea = textareaComponent?.$el?.querySelector('textarea')
      const lineNumbers = this.$refs.lineNumbersRef as HTMLElement | undefined
      if (lineNumbers && textarea) {
        lineNumbers.scrollTop = textarea.scrollTop
      }
    },
    closeModal() {
      this.$emit('close')
    },
    saveChanges() {
      this.$emit('save', this.content)
    }
  },
  watch: {
    visible(v) {
      if (v) {
        this.content = this.$props.data
      }
    }
  }
}
</script>

<style scoped>
.code-editor {
  direction: ltr !important;
  display: flex;
  border: 1px solid v-bind('theme.current.colors["outline"]');
  border-radius: 4px;
  overflow: hidden;
  font-size: 14px; /* Consistent font size */
}

.editor-card-text {
  padding: 0 16px;
}

.line-numbers {
  width: 40px;
  background: v-bind('theme.current.colors["surface"]');
  text-align: right;
  padding: 12px 8px 12px 4px; /* Match textarea padding */
  line-height: 1.5; /* Match textarea line height */
  overflow-y: hidden; /* Prevent independent scrolling */
  user-select: none;
  display: flex;
  flex-direction: column;
}

.line-numbers span {
  display: block;
  line-height: 1.5; /* Match textarea line height */
  height: 1.5em; /* Ensure consistent height per line */
  font-family: monospace; /* Match textarea font */
}

/* Override Vuetify textarea styles for alignment */
:deep(.v-textarea .v-field__input) {
  padding: 12px 8px !important; /* Match line-numbers padding */
  line-height: 1.5 !important; /* Match line-numbers line height */
  font-family: monospace !important;
  white-space: pre;
  mask-image: inherit;
  font-size: 14px !important; /* Match font size */
}

:deep(.v-textarea textarea) {
  max-height: min(60vh, 560px) !important;
  overflow: auto !important;
}

/* Ensure textarea and line numbers align */
:deep(.v-textarea textarea) {
  margin-top: 0 !important; /* Remove any default margin */
  padding-top: 0 !important; /* Remove any default padding */
}

.code-editor--large :deep(.v-textarea .v-field__input) {
  white-space: pre-wrap;
}
</style>
