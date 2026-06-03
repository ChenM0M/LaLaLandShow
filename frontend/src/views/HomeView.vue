<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { useAuthStore } from '../stores/auth'
import { getConfig } from '../api'
import type { PublicConfig } from '../api'
import gsap from 'gsap'
import ScrollTrigger from 'gsap/ScrollTrigger'

gsap.registerPlugin(ScrollTrigger)

// Track which act pages are near the viewport for GPU layer promotion
const visibleActs = ref<Set<string>>(new Set())
let actObserver: IntersectionObserver | null = null

const auth = useAuthStore()
const config = ref<PublicConfig | null>(null)
const countdownParts = ref({ d: '0', h: '00', m: '00', s: '00', open: false })

let countdownTimer: ReturnType<typeof setInterval> | null = null
let fadeObserver: IntersectionObserver | null = null
let mediaMatch: ReturnType<typeof gsap.matchMedia> | null = null

interface ActPage {
  key: string
  label: string
  title: string
  color: string
  quote: string
  desc: string
  song?: string
  image: string
}

const acts: ActPage[] = [
  {
    key: '1',
    label: '第一幕',
    title: '高架橋堵車',
    color: '#F6C048',
    quote: 'What is his problem? I should go.',
    desc: 'LA 高架橋，塞車，大太陽。一聲喇叭之後，所有人跳出車門開始唱歌——Mia 和 Seb 擦身而過。城市的躁動，金色的開場。',
    song: 'Another Day of Sun',
    image: '/assets/images/lalaland-act-freeway.jpg'
  },
  {
    key: '2',
    label: '第二幕',
    title: '宿舍派對',
    color: '#F24E8A',
    quote: "Mia, how'd the audition go?",
    desc: '試鏡又沒上。室友把她拽去派對——換衣服、戴墨鏡，把挫敗感丟在家裡。今晚說不定會不一樣。',
    song: 'Someone in the Crowd',
    image: '/assets/images/lalaland-act-party.jpg'
  },
  {
    key: '3',
    label: '第三幕',
    title: '海邊派對',
    color: '#2FC7D9',
    quote: 'I remember you. I can admit that.',
    desc: '海邊的派對，80 年代翻唱曲。Seb 在台上彈鍵盤，Mia 在人群裡認出他——那個在餐廳彈聖誕歌被開除的傢伙。',
    song: 'Sk8er Boi',
    image: '/assets/images/lalaland-act-pier.jpg'
  },
  {
    key: '4',
    label: '第四幕',
    title: '街頭漫步',
    color: '#6157D8',
    quote: 'What a waste of a lovely night.',
    desc: '深夜的 Griffith 公園，路燈把影子拉得很長。她差點走掉，他追上去——然後兩個人在星空下轉圈。',
    image: '/assets/images/lalaland-act-hilldance.jpg'
  },
  {
    key: '5',
    label: '第五幕',
    color: '#D96A36',
    title: '公寓夜話',
    quote: "We should call it Seb's.",
    desc: '公寓的鋼琴前，Seb 彈著一段旋律。Mia 說就叫 Seb\'s 吧——還沒開張的爵士俱樂部，名字先有了。',
    song: 'City of Stars',
    image: '/assets/images/lalaland-act-piano.jpg'
  },
  {
    key: '67',
    label: '第六 · 七幕',
    title: '衝突與試鏡',
    color: '#445E9C',
    quote: "I've dropped out for six years just for this fantasy.",
    desc: '拒信又來了。Seb 的巡演撞上 Mia 的獨角戲，電話那頭吵到沉默。最低谷的時候，Mia 站在試鏡間，唱了一首關於姑媽的歌。',
    song: 'The Fools Who Dream',
    image: '/assets/images/lalaland-act-audition.jpg'
  },
  {
    key: '8',
    label: '第八幕',
    title: '告別',
    color: '#F4B860',
    quote: "I'm always gonna love you.",
    desc: '巴黎來了一通電話。他要巡演，她要去法國——沒有誰對誰錯，只是方向不一樣了。',
    image: '/assets/images/lalaland-act-farewell.jpg'
  },
  {
    key: '9',
    label: '第九幕 · 終章',
    title: "五年後 · Seb's",
    color: '#233A8B',
    quote: "Welcome to Seb's.",
    desc: '五年後。Seb\'s 的招牌終於亮了。她走進來，他坐在鋼琴前——一個眼神，把沒能發生的那些年全部過了一遍。',
    song: 'Epilogue',
    image: '/assets/images/lalaland-act-jazzclub.jpg'
  },
]

const songs = acts
  .filter((act) => act.song)
  .map((act) => ({
    title: act.song as string,
    scene: `${act.label} · ${act.title}`,
    color: act.color,
  }))

const stagePhotos = [
  { src: '/assets/images/stage-photo-1.jpg' },
  { src: '/assets/images/stage-photo-2.jpg' },
  { src: '/assets/images/stage-photo-3.jpg' },
]



const ctaTarget = computed(() => (auth.token ? '/seats' : '/login'))
const venue = computed(() => config.value?.venue || '夢想劇場')

const nowTime = ref(Date.now())

const showDate = computed(() => {
  if (!config.value) return '2026 · 敬請期待'

  const now = nowTime.value
  const openAt = config.value.booking_open_at ? new Date(config.value.booking_open_at).getTime() : 0
  const lockAt = config.value.booking_lock_at ? new Date(config.value.booking_lock_at).getTime() : 0
  const startAt = config.value.show_start_at ? new Date(config.value.show_start_at).getTime() : 0

  const formatDate = (ts: number) => {
    const d = new Date(ts)
    return `${d.getFullYear()}.${d.getMonth() + 1}.${d.getDate()}`
  }

  if (openAt && now < openAt) return `${formatDate(openAt)} 預約開啟`
  if (lockAt && now < lockAt) return `${formatDate(lockAt)} 預約截止`
  if (startAt && now < startAt) return `${formatDate(startAt)} 演出開始`

  return config.value.show_date || '2026 · 敬請期待'
})

function updateCountdown() {
  nowTime.value = Date.now()
  if (!config.value?.booking_open_at) {
    countdownParts.value = { d: '0', h: '00', m: '00', s: '00', open: false }
    return
  }

  const diff = new Date(config.value.booking_open_at).getTime() - nowTime.value
  if (diff <= 0) {
    countdownParts.value = { d: '0', h: '00', m: '00', s: '00', open: true }
    return
  }

  const d = Math.floor(diff / 86400000)
  const h = Math.floor((diff % 86400000) / 3600000)
  const m = Math.floor((diff % 3600000) / 60000)
  const s = Math.floor((diff % 60000) / 1000)
  countdownParts.value = {
    d: String(d),
    h: String(h).padStart(2, '0'),
    m: String(m).padStart(2, '0'),
    s: String(s).padStart(2, '0'),
    open: false,
  }
}

function mountFadeObserver() {
  fadeObserver = new IntersectionObserver((entries) => {
    entries.forEach((entry) => {
      if (!entry.isIntersecting) return
      entry.target.classList.add('is-visible')
      fadeObserver?.unobserve(entry.target)
    })
  }, { threshold: 0.18 })

  document.querySelectorAll<HTMLElement>('.fade-up').forEach((el) => {
    fadeObserver?.observe(el)
  })
}

function mountActObserver() {
  // Only promote act-pages to GPU layers when they're within 1 viewport of being visible
  actObserver = new IntersectionObserver((entries) => {
    entries.forEach((entry) => {
      const key = (entry.target as HTMLElement).dataset.act || ''
      if (entry.isIntersecting) {
        visibleActs.value.add(key)
      } else {
        visibleActs.value.delete(key)
      }
    })
  }, { rootMargin: '100% 0px 100% 0px', threshold: 0 })

  document.querySelectorAll<HTMLElement>('.act-page').forEach((el) => {
    actObserver?.observe(el)
  })
}

function mountScrollMotion() {
  mediaMatch = gsap.matchMedia()

  // The horizontal accordion stacking effect
  mediaMatch.add('(min-width: 769px)', () => {
    const pages = gsap.utils.toArray<HTMLElement>('.act-page')
    const stackOffset = 40
    const maxStackOffset = 220
    const contents = pages.map((page) => page.querySelector('.act-page__content'))
    
    gsap.set(pages, { 
      xPercent: (i) => i === 0 ? 0 : 100,
      x: 0,
      width: (i) => i === 0 ? '100vw' : `calc(100vw - ${Math.min(i * stackOffset, maxStackOffset)}px)`,
      zIndex: (i) => i + 1,
      force3D: true,
    })
    gsap.set(contents, { autoAlpha: (i) => i === 0 ? 1 : 0 })

    const tl = gsap.timeline({
      scrollTrigger: {
        trigger: '#flip-book',
        start: 'top top',
        end: `+=${pages.length * 1000}`,
        pin: true,
        scrub: 0.6,
        anticipatePin: 1,
        fastScrollEnd: true,
        invalidateOnRefresh: true,
      }
    })

    pages.forEach((page, i) => {
      if (i === 0) return
      tl.to(pages[i - 1].querySelector('.act-page__content'), {
        autoAlpha: 0,
        ease: 'none',
        duration: 0.2,
      }, '+=0.08')
      tl.to(page, {
        xPercent: 0,
        x: 0,
        ease: 'none',
        duration: 1
      }, '<0.12')
      tl.to(page.querySelector('.act-page__content'), {
        autoAlpha: 1,
        ease: 'none',
        duration: 0.22,
      }, '<0.64')
    })

    tl.to({}, { duration: 1.2 })

    // Hero parallax
    gsap.to('.hero-image', {
      yPercent: 14,
      ease: 'none',
      scrollTrigger: {
        trigger: '.hero',
        start: 'top top',
        end: 'bottom top',
        scrub: true,
      },
    })
  })

  mediaMatch.add('(max-width: 768px)', () => {
    const pages = gsap.utils.toArray<HTMLElement>('.act-page')
    const stackOffset = 10
    const maxStackOffset = 72
    const contents = pages.map((page) => page.querySelector('.act-page__content'))
    gsap.set(pages, { 
      xPercent: (i) => i === 0 ? 0 : 100,
      x: 0,
      width: (i) => i === 0 ? '100vw' : `calc(100vw - ${Math.min(i * stackOffset, maxStackOffset)}px)`,
      zIndex: (i) => i + 1,
      force3D: true,
    })
    gsap.set(contents, { autoAlpha: (i) => i === 0 ? 1 : 0 })

    const tl = gsap.timeline({
      scrollTrigger: {
        trigger: '#flip-book',
        start: 'top top',
        end: `+=${pages.length * 980}`,
        pin: true,
        scrub: 0.6,
        anticipatePin: 1,
        fastScrollEnd: true,
        invalidateOnRefresh: true,
      }
    })

    pages.forEach((page, i) => {
      if (i === 0) return
      tl.to(pages[i - 1].querySelector('.act-page__content'), {
        autoAlpha: 0,
        ease: 'none',
        duration: 0.18,
      }, '+=0.18')
      tl.to(page, {
        xPercent: 0,
        x: 0,
        ease: 'none',
        duration: 1.1
      }, '<0.14')
      tl.to(page.querySelector('.act-page__content'), {
        autoAlpha: 1,
        ease: 'none',
        duration: 0.2,
      }, '<0.76')
    })

    tl.to({}, { duration: 1.6 })
  })
}

onMounted(async () => {
  try {
    config.value = await getConfig()
  } catch {
    config.value = null
  }

  updateCountdown()
  countdownTimer = setInterval(updateCountdown, 1000)

  await nextTick()
  mountFadeObserver()
  mountActObserver()
  mountScrollMotion()
})

onUnmounted(() => {
  if (countdownTimer) clearInterval(countdownTimer)
  fadeObserver?.disconnect()
  actObserver?.disconnect()
  mediaMatch?.revert()
  ScrollTrigger.getAll().forEach((trigger) => trigger.kill())
})
</script>

<template>
  <main class="home-wrapper">
    <div class="film-grain" aria-hidden="true"></div>

    <section id="hero" class="hero">
      <picture class="hero-picture">
        <source media="(max-width: 768px)" srcset="/assets/images/lalaland-clean-hero-mobile.jpg" />
        <img
          class="hero-image"
          src="/assets/images/lalaland-clean-hero.jpg"
          alt="La La Land 夜景共舞宣傳視覺"
        />
      </picture>
      <div class="hero-shade" aria-hidden="true"></div>

      <div class="hero-content fade-up">
        <p class="eyebrow hero-eyebrow">爱乐之城</p>
        <h1>La La Land</h1>
        <p class="hero-subtitle">一封寫給所有孤獨逐夢者的情書</p>
        <blockquote>當兩個追夢者在城市的燈火裡相遇，遺憾也會發出星光。</blockquote>
      </div>

      <router-link :to="ctaTarget" class="cta-button hero-cta fade-up">
        <span aria-hidden="true">★</span>
        預約觀演
        <small>選座入場</small>
      </router-link>

      <div class="hero-meta fade-up">
        <div class="venue-info">
          <img src="/assets/images/school-logo-cropped.png" alt="上海台商子女學校" class="venue-logo" />
          <span>{{ venue }}</span>
        </div>
        <span>{{ showDate }}</span>
      </div>
    </section>

    <!-- Seamless transitions between sections -->
    <div class="seamless-group">
      <section id="synopsis" class="synopsis fade-up">
        <div class="synopsis-inner">
          <p class="kicker">Story Book</p>
          <h2>電影有九幕，舞台也是。</h2>
          <p>
            從 LA 高架橋到爵士酒吧，九段故事、九個顏色。
            用身體、用音樂、用燈光，把 Mia 和 Seb 的軌跡重新走一次。
          </p>
        </div>

        <aside class="countdown">
          <p class="kicker">Curtain Raises</p>
          <div v-if="!countdownParts.open" class="time-grid">
            <div><strong>{{ countdownParts.d }}</strong><span>D</span></div>
            <div><strong>{{ countdownParts.h }}</strong><span>H</span></div>
            <div><strong>{{ countdownParts.m }}</strong><span>M</span></div>
            <div><strong>{{ countdownParts.s }}</strong><span>S</span></div>
          </div>
          <p v-else class="booking-open">預約通道已開啟</p>
        </aside>
      </section>

      <section id="flip-book" class="flip-book" aria-label="九幕翻頁書">
        <div class="scroll-cue" aria-hidden="true">
          <span>向下滑动</span>
          <i></i>
        </div>
        <article
          v-for="act in acts"
          :key="act.key"
          class="act-page"
          :class="{ 'act-page--active': visibleActs.has(act.key) }"
          :data-act="act.key"
          :style="{ '--act-color': act.color }"
        >
          <img
            class="act-page__bg"
            :src="act.image"
            :alt="`${act.label} ${act.title}`"
            loading="lazy"
            decoding="async"
          />
          <div class="act-page__content">
            <span class="act-label" :style="{ color: act.color, borderColor: act.color }">{{ act.label }}</span>
            <h2>{{ act.title }}</h2>
            <blockquote v-if="act.quote">{{ act.quote }}</blockquote>
            <p>{{ act.desc }}</p>
            <span v-if="act.song" class="song-chip" :style="{ borderColor: act.color }">{{ act.song }}</span>
          </div>
        </article>
      </section>

      <section id="songs" class="songs fade-up">
        <div class="section-heading">
          <p class="kicker">Songs</p>
          <h2>每首歌都是一幕的顏色。</h2>
        </div>
        <div class="song-list">
          <div
            v-for="(song, index) in songs"
            :key="song.title"
            class="song-row fade-up"
            :class="{ 'song-row--right': index % 2 === 1 }"
            :style="{ '--song-color': song.color }"
          >
            <span>{{ String(index + 1).padStart(2, '0') }}</span>
            <strong>{{ song.title }}</strong>
            <em>{{ song.scene }}</em>
          </div>
        </div>
      </section>
    </div>

    <section id="stage-photos" class="rehearsal fade-up">
      <div class="section-heading">
        <p class="kicker">Stage Photos</p>
        <h2>精彩场照</h2>
      </div>
      <div class="masonry">
        <figure
          v-for="(photo, index) in stagePhotos"
          :key="photo.src"
          class="rehearsal-frame fade-up"
          :class="`rehearsal-frame--${index + 1}`"
        >
          <img :src="photo.src" alt="Stage Photo" loading="lazy" />
        </figure>
      </div>
    </section>

    <section id="cast" class="cast fade-up">
      <div class="section-heading">
        <p class="kicker">Cast & Crew</p>
        <h2>台上台下，都是我們。</h2>
      </div>
      <div class="credits-container fade-up">
        <div class="credits-column">
          <h3 class="credits-title">🎭 Cast｜演員</h3>
          <dl class="credits-list">
            <dt>Mia</dt><dd>楊宓霓</dd>
            <dt>Sebastian</dt><dd>呂宜蓁</dd>
            <dt>鋼琴獨奏</dt><dd>王子亮</dd>
            <dt>舞蹈</dt><dd>呂依潔、孫和璐、陳伯韜、呂宜蓁、沈睿維、陳菱菲、陳采葳、張若妤、張睿祺、林芷雲、崔以諾、簡毓蓁、姜秉銓、姜柔竹、陳思帆</dd>
            <dt>舍友</dt><dd>陳菱菲、陳采葳、陳思帆</dd>
            <dt>派對樂隊主唱</dt><dd>呂俞霈</dd>
            <dt>派對觀眾</dt><dd>郭舒喬、呂依潔、孫和璐</dd>
            <dt>郵差</dt><dd>陳伯韜</dd>
            <dt>遞花・收花</dt><dd>張若妤</dd>
            <dt>遞酒・收酒</dt><dd>朱文詣</dd>
            <dt>製片人</dt><dd>楊洛亭</dd>
            <dt>鋼琴伴奏</dt><dd>陳思帆</dd>
            <dt>女主丈夫</dt><dd>朱文詣</dd>
            <dt>酒吧觀眾</dt><dd>郭舒喬、張翊婕、高巧芸、孫和璐</dd>
          </dl>
        </div>
        
        <div class="credits-column">
          <h3 class="credits-title">🎬 Production Team｜製作團隊</h3>
          <dl class="credits-list">
            <dt>導演</dt><dd>楊洛亭、楊宓霓</dd>
            <dt>舞蹈編排</dt><dd>陳菱菲、陳采葳</dd>
            <dt>道具</dt><dd>陳靖元、黃浩宇、張翊婕、高巧芸</dd>
            <dt>燈光</dt><dd>宣姿妤、黃詩涵</dd>
            <dt>ppt</dt><dd>李姍如</dd>
            <dt>網站</dt><dd>羅章鋭</dd>
            <dt>宣傳</dt><dd>陳思帆</dd>
            <dt>後勤</dt><dd>蒯乃安、陳靖元、黃盈偉、黃浩宇、申紹東、陳永軒、姜秉銓、高巧芸、張翊婕</dd>
          </dl>
          
          <h3 class="credits-title" style="margin-top: 48px;">🏫 指導單位</h3>
          <dl class="credits-list">
            <dt>指導老師</dt><dd>洪薏晴</dd>
            <dt>總召</dt><dd>楊洛亭、楊宓霓</dd>
          </dl>
        </div>
      </div>
      <div class="credits-thanks fade-up">
        謝謝每一個台上台下的人——<br>
        這齣戲是真的了。
      </div>
    </section>

    <footer id="cta-section" class="final-cta">
      <div class="stars" aria-hidden="true"></div>
      <div class="final-cta__inner fade-up">
        <p class="kicker">Final Call</p>
        <h2>還有一個位子，留給想來的人。</h2>
        <router-link :to="ctaTarget" class="cta-button cta-button--dark">
          <span aria-hidden="true">★</span>
          預約觀演
          <small>{{ auth.token ? '進入選座' : '登入後選座' }}</small>
        </router-link>
      </div>
      
    </footer>
  </main>
</template>

<style scoped>
.home-wrapper {
  --bg-deep: #070b24;
  --bg-night: #151445;
  --star: #efe8d3;
  --sunset: #8d91c7;
  --rose: #5f67a6;
  --cyan: #4f9ab1;
  --violet: #3f3c91;
  --wine: #32122d;
  --text-primary: #f7f2df;
  --text-secondary: rgba(247, 242, 223, 0.74);
  --paper: rgba(255, 233, 184, 0.07);
  --line: transparent; /* Removed visible lines as requested */
  --font-display: 'Libre Bodoni', 'Playfair Display', 'Noto Serif TC', serif;
  --font-act: 'Libre Bodoni', 'Bodoni 72 Smallcaps', 'Didot', 'Limelight', serif;
  --font-body: 'Lora', 'Noto Serif TC', serif;
  --font-ui: 'Nunito Sans', -apple-system, BlinkMacSystemFont, sans-serif;
  background:
    radial-gradient(circle at 18% 0%, rgba(239, 232, 211, 0.1), transparent 30rem),
    radial-gradient(circle at 82% 20%, rgba(95, 103, 166, 0.16), transparent 30rem),
    linear-gradient(180deg, #040827 0%, #11133f 46%, #090d2f 100%);
  color: var(--text-primary);
  font-family: var(--font-body);
  overflow-x: hidden;
}

.film-grain {
  position: fixed;
  inset: 0;
  z-index: 9999;
  pointer-events: none;
  opacity: 0.035;
  background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 180 180' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.85' numOctaves='3' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E");
  /* Prevent the grain layer from triggering recomposition on every scroll frame */
  will-change: auto;
  contain: strict;
  content-visibility: auto;
}

.hero {
  position: relative;
  min-height: 100vh;
  display: grid;
  place-items: start start;
  padding: clamp(4.25rem, 9vh, 6.5rem) clamp(1.25rem, 5vw, 4.5rem) 6rem;
  isolation: isolate;
  overflow: hidden;
  background: transparent;
}

.hero-picture,
.hero-shade {
  position: absolute;
}

.hero-picture {
  z-index: -3;
  inset: 0;
}

.hero-image {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
  object-position: center;
  filter: saturate(1.02) contrast(1.03) brightness(0.9);
  opacity: 1;
}

.hero-shade {
  z-index: -2;
  inset: 0;
  background:
    linear-gradient(180deg, rgba(4, 8, 39, 0.3) 0%, rgba(4, 8, 39, 0) 50%, #040827 100%),
    linear-gradient(90deg, rgba(4, 8, 39, 0.8) 0%, rgba(4, 8, 39, 0.4) 40%, transparent 70%, rgba(4, 8, 39, 0.2) 100%);
}

.hero-content {
  max-width: min(36rem, 52vw);
  text-align: left;
  text-shadow: 0 0.8rem 2.3rem rgba(0, 0, 0, 0.58);
}

.eyebrow,
.kicker {
  font-family: var(--font-ui);
  color: rgba(255, 193, 7, 0.86);
  font-size: 0.78rem;
  letter-spacing: 0.2em;
  text-transform: uppercase;
}

.hero h1 {
  margin: 0.4rem 0 0.55rem;
  font-family: 'Limelight', 'Libre Bodoni', 'Noto Serif TC', serif;
  font-size: clamp(4.3rem, 9.4vw, 8.7rem);
  font-weight: 400;
  line-height: 0.88;
  letter-spacing: 0;
  color: #f7f2df;
  text-shadow:
    0 0.12rem 0 rgba(10, 11, 38, 0.62),
    0 0.9rem 2.4rem rgba(0, 0, 0, 0.48);
}

.hero-eyebrow {
  color: rgba(255, 238, 205, 0.92);
  letter-spacing: 0.28em;
}

.hero-subtitle {
  max-width: 30rem;
  margin: 0;
  color: rgba(255, 238, 205, 0.88);
  font-family: var(--font-display);
  font-size: clamp(1rem, 2vw, 1.36rem);
  line-height: 1.45;
}

.hero blockquote {
  max-width: 31rem;
  margin: 1rem 0 0;
  color: rgba(255, 238, 205, 0.7);
  font-size: 0.98rem;
  line-height: 1.8;
}

.cta-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.75rem;
  min-height: 3.4rem;
  max-width: 28rem;
  margin-top: 1.55rem;
  padding: 0.85rem 1.4rem;
  border: 1px solid rgba(247, 242, 223, 0.46);
  background: rgba(3, 8, 37, 0.34);
  color: #f7f2df;
  font-family: var(--font-ui);
  font-weight: 700;
  letter-spacing: 0.08em;
  text-decoration: none;
  text-transform: uppercase;
  backdrop-filter: blur(18px);
  -webkit-backdrop-filter: blur(18px);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.06);
  transition: transform 220ms ease, background 220ms ease, border-color 220ms ease, color 220ms ease;
}

.cta-button small {
  color: inherit;
  font-size: 0.72rem;
  font-weight: 700;
  opacity: 0.72;
}

.cta-button:hover {
  transform: translateY(-2px);
  border-color: rgba(255, 255, 255, 0.92);
  background: rgba(247, 242, 223, 0.12);
  color: #fffaf0;
}

.cta-button--dark {
  color: #f7f2df;
  background: rgba(3, 8, 37, 0.58);
}

.hero-cta {
  position: absolute;
  left: clamp(1.25rem, 5vw, 4.5rem);
  bottom: clamp(5rem, 8vh, 6.4rem);
  margin-top: 0;
  box-shadow: 0 1rem 2.6rem rgba(0, 0, 0, 0.34);
}

.hero-meta {
  position: absolute;
  left: 2rem;
  right: 2rem;
  bottom: 1.4rem;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
  color: rgba(255, 238, 204, 0.72);
  font-family: var(--font-ui);
  font-size: 0.8rem;
  letter-spacing: 0.13em;
  text-transform: uppercase;
}

.venue-info {
  display: flex;
  align-items: center;
  gap: 0.6rem;
}

.venue-logo {
  height: 2.4rem;
  width: auto;
  object-fit: contain;
  filter: drop-shadow(0 2px 6px rgba(0,0,0,0.8));
}

.seamless-group {
  padding: 5rem 0 6rem;
}
.seamless-group > section {
  margin-bottom: 6rem;
}

.synopsis {
  width: min(1120px, calc(100% - 2rem));
  margin: 0 auto;
  display: grid;
  grid-template-columns: minmax(0, 1.35fr) minmax(18rem, 0.65fr);
  gap: 5rem;
  align-items: center;
}

.synopsis h2,
.section-heading h2,
.final-cta h2 {
  margin-top: 0.8rem;
  font-family: var(--font-display);
  font-size: 3rem;
  line-height: 1.15;
}

.synopsis p:not(.kicker) {
  margin-top: 1.4rem;
  color: var(--text-secondary);
  font-size: 1.05rem;
  line-height: 2;
}

.time-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 0.7rem;
  margin-top: 1.4rem;
}

.time-grid div {
  min-height: 5.5rem;
  display: grid;
  place-items: center;
  background: linear-gradient(160deg, rgba(255, 214, 107, 0.12), rgba(35, 215, 223, 0.06));
  border: 1px solid rgba(255, 243, 215, 0.12);
  border-radius: 4px;
}

.time-grid strong {
  display: block;
  font-family: var(--font-display);
  font-size: 2rem;
  line-height: 1;
}

.time-grid span {
  color: var(--text-secondary);
  font-family: var(--font-ui);
  font-size: 0.72rem;
}

.booking-open {
  margin-top: 1rem;
  color: var(--star);
}

/* Horizontal Stack Blinds Effect */
.flip-book {
  position: relative;
  width: 100vw;
  height: 100vh;
  overflow: hidden;
  /* Removed padding so cards fill the screen entirely */
}

.scroll-cue {
  position: absolute;
  left: 50%;
  bottom: clamp(5.2rem, 9vh, 6.4rem);
  z-index: 20;
  display: inline-flex;
  align-items: center;
  gap: 0.62rem;
  padding: 0.56rem 0.78rem;
  border: 1px solid rgba(255, 243, 215, 0.18);
  border-radius: 999px;
  background: rgba(7, 10, 34, 0.72);
  color: rgba(255, 243, 215, 0.76);
  font-family: var(--font-ui);
  font-size: 0.74rem;
  letter-spacing: 0.12em;
  transform: translateX(-50%);
  pointer-events: none;
  /* Removed backdrop-filter — expensive during scroll-scrub animation */
}

.scroll-cue i {
  width: 1.05rem;
  height: 1.7rem;
  border: 1px solid rgba(255, 243, 215, 0.46);
  border-radius: 999px;
  position: relative;
}

.scroll-cue i::after {
  content: '';
  position: absolute;
  left: 50%;
  top: 0.28rem;
  width: 0.25rem;
  height: 0.25rem;
  border-radius: 50%;
  background: #fff3d7;
  transform: translateX(-50%);
  animation: scroll-dot 1.35s ease-in-out infinite;
}

.act-page {
  --act-color: var(--star);
  position: absolute;
  top: 0;
  right: 0;
  left: auto;
  width: 100vw;
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: flex-start;
  padding: 8vh 9vw;
  background: #080b24;
  box-shadow: -10px 0 26px rgba(13, 8, 50, 0.32);
  backface-visibility: hidden;
  contain: layout paint style;
  transform: translateZ(0);
  /* Only promote to GPU layer when near viewport — managed by actObserver */
  will-change: auto;
  overflow: hidden;
}

/* Promote to GPU layer only when near viewport */
.act-page--active {
  will-change: transform;
}

.act-page__bg {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
  object-position: center;
  z-index: -1;
  /* Prevent image decode from blocking main thread */
  content-visibility: auto;
}

.act-page::before {
  content: '';
  position: absolute;
  inset: 0;
  background:
    linear-gradient(90deg, rgba(8, 10, 34, 0.88) 0%, rgba(8, 10, 34, 0.66) 34%, rgba(8, 10, 34, 0.14) 72%, rgba(8, 10, 34, 0.28) 100%),
    linear-gradient(180deg, rgba(8, 10, 34, 0.12) 0%, rgba(8, 10, 34, 0.1) 58%, rgba(8, 10, 34, 0.62) 100%);
  z-index: 0;
}

.act-page::after {
  content: '';
  position: absolute;
  top: 0;
  bottom: 0;
  left: 0;
  width: min(7vw, 72px);
  background: linear-gradient(90deg, rgba(255, 243, 215, 0.11), transparent);
  opacity: 0.54;
  pointer-events: none;
}

.act-page__content {
  position: relative;
  z-index: 1;
  max-width: 34rem;
  text-align: left;
  padding: 1.6rem 2rem 1.6rem 0;
  border-left: 4px solid var(--act-color);
  padding-left: 1.6rem;
  text-shadow: 0 0.8rem 2.6rem rgba(0, 0, 0, 0.54);
  contain: layout paint;
}

.act-label {
  display: inline-block;
  padding: 0.35rem 1rem;
  border: 1px solid;
  background: rgba(21, 16, 79, 0.38);
  font-family: var(--font-act);
  font-size: 0.8rem;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  margin-bottom: 1rem;
}

.act-page h2 {
  font-family: var(--font-display);
  font-size: clamp(3rem, 7vw, 6.6rem);
  line-height: 1.05;
  margin-bottom: 1rem;
  letter-spacing: 0;
  text-transform: none;
}

.act-page blockquote {
  color: var(--text-primary);
  font-style: italic;
  font-size: 1.1rem;
  line-height: 1.75;
  margin-bottom: 1rem;
}

.act-page p {
  color: var(--text-secondary);
  line-height: 1.95;
  margin-bottom: 1rem;
}

.song-chip {
  display: inline-flex;
  min-height: 2.3rem;
  align-items: center;
  border: 1px solid rgba(255, 243, 215, 0.2);
  background: rgba(21, 16, 79, 0.34);
  padding: 0.35rem 0.8rem;
  color: var(--text-primary);
  font-family: var(--font-ui);
  font-size: 0.82rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.songs, .rehearsal, .cast {
  width: min(1120px, calc(100% - 2rem));
  margin: 0 auto;
  padding: 4rem 0;
  /* Skip rendering of off-screen sections */
  content-visibility: auto;
  contain-intrinsic-size: auto 600px;
}

.section-heading {
  max-width: 48rem;
  margin-bottom: 2.6rem;
}

.song-list {
  display: grid;
  gap: 0.9rem;
}

.song-row {
  display: grid;
  grid-template-columns: 4rem minmax(12rem, 1fr) minmax(14rem, 0.9fr);
  gap: 1.25rem;
  align-items: center;
  min-height: 5rem;
  color: var(--text-secondary);
  transform: translateX(-1.5rem);
  padding: 0.75rem 1rem;
  background: linear-gradient(90deg, color-mix(in srgb, var(--song-color) 12%, transparent), transparent 72%);
  border-left: 3px solid var(--song-color);
}

.song-row--right {
  transform: translateX(1.5rem);
}

.song-row span {
  color: var(--song-color);
  font-family: var(--font-act);
}

.song-row strong {
  color: var(--text-primary);
  font-family: var(--font-display);
  font-size: 1.45rem;
}

.song-row em {
  font-style: normal;
  text-align: right;
}

.masonry {
  columns: 3 16rem;
  column-gap: 1rem;
}

.rehearsal-frame {
  position: relative;
  break-inside: avoid;
  margin: 0 0 1rem;
  overflow: hidden;
  background: #15104f;
  border-radius: 4px;
}

.rehearsal-frame--2 {
  margin-top: 2rem;
}

.rehearsal-frame img {
  display: block;
  width: 100%;
  aspect-ratio: 4 / 3;
  object-fit: cover;
  filter: saturate(0.76) contrast(1.12) sepia(0.08);
  transition: transform 700ms ease, filter 700ms ease;
}

.rehearsal-frame:hover img {
  transform: scale(1.05);
  filter: saturate(1.05) contrast(1);
}

.rehearsal-frame figcaption {
  position: absolute;
  left: 1rem;
  right: 1rem;
  bottom: 1rem;
  display: flex;
  justify-content: space-between;
  gap: 1rem;
  align-items: end;
  color: var(--text-primary);
  text-shadow: 0 0.8rem 2rem rgba(0, 0, 0, 0.85);
}

.rehearsal-frame span {
  color: rgba(245, 240, 232, 0.72);
  font-family: var(--font-ui);
  font-size: 0.72rem;
  text-transform: uppercase;
}

.credits-container {
  display: flex;
  flex-direction: column;
  gap: 60px;
  max-width: 1000px;
  margin: 0 auto;
}

@media (min-width: 768px) {
  .credits-container {
    flex-direction: row;
    justify-content: space-between;
    gap: 40px;
  }
}

.credits-column {
  flex: 1;
}

.credits-title {
  font-family: var(--font-display);
  font-size: 24px;
  color: var(--gold-light);
  margin-bottom: 24px;
  border-bottom: 1px solid var(--border);
  padding-bottom: 12px;
}

.credits-list {
  display: grid;
  grid-template-columns: 120px 1fr;
  row-gap: 16px;
  column-gap: 16px;
  align-items: baseline;
  text-align: left;
}

@media (max-width: 768px) {
  .credits-list {
    grid-template-columns: 100px 1fr;
    font-size: 14px;
  }
}

.credits-list dt {
  color: var(--muted);
  font-weight: 500;
  text-align: right;
}

.credits-list dd {
  color: var(--cream);
  margin: 0;
  line-height: 1.6;
  word-break: keep-all;
  overflow-wrap: normal;
  line-break: strict;
}

.credits-thanks {
  text-align: center;
  margin-top: 80px;
  font-size: 20px;
  line-height: 1.8;
  color: var(--purple-light);
  font-family: var(--font-display);
}

.final-cta {
  position: relative;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  padding: 10rem 1rem 9rem; /* Increased bottom padding to avoid dock */
  overflow: hidden;
  text-align: center;
  background:
    radial-gradient(circle at 50% 35%, rgba(247, 242, 223, 0.12), transparent 24rem),
    radial-gradient(circle at 72% 50%, rgba(95, 103, 166, 0.18), transparent 22rem);
}

.stars::before,
.stars::after {
  content: '';
  position: absolute;
  inset: 12% 8%;
  background-image:
    radial-gradient(circle, rgba(255, 214, 107, 0.9) 0 1px, transparent 1.5px),
    radial-gradient(circle, rgba(35, 215, 223, 0.7) 0 1px, transparent 1.5px);
  background-size: 8rem 7rem, 11rem 9rem;
  animation: star-drift 14s linear infinite;
  opacity: 0.45;
}

.stars::after {
  inset: 20% 4%;
  animation-duration: 20s;
  opacity: 0.28;
}

.fade-up {
  opacity: 0;
  transform: translateY(1.7rem);
  transition: opacity 1000ms cubic-bezier(0.16, 1, 0.3, 1), transform 1000ms cubic-bezier(0.16, 1, 0.3, 1);
}

.fade-up.is-visible {
  opacity: 1;
  transform: translateY(0);
}

@keyframes star-drift {
  from { transform: translateY(0); }
  to { transform: translateY(-4rem); }
}

@keyframes scroll-dot {
  0% { transform: translate(-50%, 0); opacity: 0; }
  25% { opacity: 1; }
  75% { opacity: 1; }
  100% { transform: translate(-50%, 0.7rem); opacity: 0; }
}

@media (max-width: 980px) {
  .hero h1 {
    font-size: clamp(3.6rem, 12vw, 6.2rem);
  }

  .synopsis {
    grid-template-columns: 1fr;
  }

  .song-row {
    grid-template-columns: 3rem 1fr;
  }

  .song-row em {
    grid-column: 2;
    text-align: left;
  }
}

@media (max-width: 768px) {
  .hero {
    min-height: 92svh;
    padding-top: 3.2rem;
  }

  .hero h1 {
    font-size: clamp(3rem, 15vw, 4.65rem);
  }

  .hero-subtitle {
    font-size: 1.08rem;
  }

  .hero-meta {
    position: absolute;
    left: 1rem;
    right: 1rem;
    bottom: 1rem;
    width: auto;
    margin: 0;
    flex-direction: column;
    gap: 0.45rem;
    text-align: center;
  }

  .synopsis h2,
  .section-heading h2,
  .final-cta h2 {
    font-size: 2.1rem;
  }

  .act-page__content {
    width: 100%;
    max-width: 100%;
    margin-right: 0;
    padding: 0 1.25rem 1.2rem;
    border-left: 0;
    border-bottom: 3px solid var(--act-color);
    text-align: left;
    box-sizing: border-box;
  }

  .act-page {
    align-items: flex-end;
    justify-content: flex-start;
    padding: 0 0 12vh;
    box-shadow: -5px 0 14px rgba(13, 8, 50, 0.24);
  }

  .act-page::before {
    background:
      linear-gradient(to top, rgba(9, 12, 36, 0.9) 0%, rgba(9, 12, 36, 0.62) 50%, rgba(9, 12, 36, 0.08) 100%);
  }

  .scroll-cue {
    top: calc(1rem + env(safe-area-inset-top));
    bottom: auto;
  }

  .time-grid {
    grid-template-columns: repeat(2, 1fr);
  }

  .cta-button {
    width: min(100%, 24rem);
  }

  .hero-cta {
    left: 2rem;
    right: 2rem;
    bottom: 6.2rem;
    width: auto;
    max-width: none;
    transform: none;
  }

  .hero-cta.is-visible {
    transform: none;
  }

  .hero-cta:hover {
    transform: translateY(-2px);
  }
}
</style>
