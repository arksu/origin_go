<script setup lang="ts">
import { computed } from 'vue'
import { gameCalendarSync } from '@/network/GameCalendarSync'
import daysky from '@/assets/img/daytime/daysky.png'
import nightsky from '@/assets/img/daytime/nightsky.png'
import dayscape from '@/assets/img/daytime/dayscape.png'
import nightscape from '@/assets/img/daytime/nightscape.png'
import sun from '@/assets/img/daytime/sun.png'

const calendar = computed(() => gameCalendarSync.getCalendar())
const timeText = computed(() => calendar.value
  ? `${String(calendar.value.hour).padStart(2, '0')}:${String(calendar.value.minute).padStart(2, '0')}`
  : '--:--')
const timeLabel = computed(() => calendar.value ? `Game time: ${timeText.value}` : 'Game time unavailable')

const sky = computed(() => {
  if (!calendar.value) return null
  const phase = calendar.value.dayPhase
  // The original artwork uses a normalized dawn-to-evening decorative profile.
  const progress = Math.min(1, Math.max(0, (phase - 5 / 24) / (16 / 24)))
  const angle = progress * (Math.PI + 0.6) - 0.2
  const nightOpacity = phase < 5 / 24 || phase >= 22 / 24 ? 1
    : phase < 6 / 24 ? 6 - phase * 24
      : phase <= 21 / 24 ? 0 : phase * 24 - 21
  return {
    nightOpacity,
    sunStyle: {
      left: `${60 - Math.cos(angle) * 23}px`,
      top: `${26 - Math.sin(angle) * 23}px`,
    },
  }
})
</script>

<template>
  <div class="daytime" role="img" :aria-label="timeLabel">
    <template v-if="sky">
      <img class="daytime__layer" :src="daysky" alt="" aria-hidden="true" draggable="false" width="134" height="71">
      <img class="daytime__layer" :src="nightsky" :style="{ opacity: sky.nightOpacity }" alt="" aria-hidden="true" draggable="false" width="134" height="71">
      <img class="daytime__layer daytime__sun" :src="sun" :style="sky.sunStyle" alt="" aria-hidden="true" draggable="false" width="17" height="17">
      <img class="daytime__layer" :src="dayscape" alt="" aria-hidden="true" draggable="false" width="134" height="71">
      <img class="daytime__layer" :src="nightscape" :style="{ opacity: sky.nightOpacity }" alt="" aria-hidden="true" draggable="false" width="134" height="71">
    </template>
    <span class="daytime__time" aria-hidden="true">{{ timeText }}</span>
  </div>
</template>

<style scoped>
.daytime {
  position: relative;
  flex: none;
  width: 134px;
  height: 80px;
  pointer-events: none;
}

.daytime__layer {
  position: absolute;
  top: 0;
  left: 0;
  pointer-events: none;
}

.daytime__time {
  position: absolute;
  top: 66px;
  left: 0;
  width: 100%;
  color: #3db67f;
  font-size: 12px;
  line-height: 14px;
  text-align: center;
  text-shadow: 1px 0 1px #000, 0 1px 1px #000, -1px 0 1px #000, 0 -1px 1px #000;
}
</style>
